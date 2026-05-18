/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package notebook implements the mutating webhook for Notebook connection secrets.
package notebook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	authv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/opendatahub-io/workbenches-operator/internal/metadata"
)

var (
	errNoContainers     = errors.New("no containers found in notebook spec")
	errInvalidContainer = errors.New("invalid container format")
)

// NotebookWebhook implements a mutating webhook that injects connection secrets
// into Notebook resources based on the opendatahub.io/connections annotation.
type NotebookWebhook struct {
	Client    client.Client
	APIReader client.Reader
	Decoder   admission.Decoder
	Name      string
}

//+kubebuilder:webhook:path=/platform-connection-notebook,mutating=true,failurePolicy=fail,groups=kubeflow.org,resources=notebooks,verbs=create;update,versions=v1,name=connection-notebook.opendatahub.io,sideEffects=None,admissionReviewVersions=v1

// SetupWithManager registers the notebook webhook with the manager.
func (w *NotebookWebhook) SetupWithManager(mgr ctrl.Manager) error {
	hookServer := mgr.GetWebhookServer()
	hookServer.Register("/platform-connection-notebook", &webhook.Admission{
		Handler: w,
	})

	return nil
}

// Handle processes admission requests for Notebook create/update operations.
func (w *NotebookWebhook) Handle(ctx context.Context, req admission.Request) admission.Response {
	l := logf.FromContext(ctx).WithName(w.Name)

	notebook := &unstructured.Unstructured{}
	if err := w.Decoder.Decode(req, notebook); err != nil {
		l.Error(err, "failed to decode notebook")

		return admission.Errored(http.StatusBadRequest, err)
	}

	annotations := notebook.GetAnnotations()
	if annotations == nil {
		return admission.Allowed("no annotations found")
	}

	connectionsStr, ok := annotations[metadata.ConnectionAnnotation]
	if !ok || connectionsStr == "" {
		return admission.Allowed("no connections annotation found")
	}

	connectionRefs := parseConnectionRefs(connectionsStr)
	if len(connectionRefs) == 0 {
		return admission.Allowed("no connection references found")
	}

	namespace := notebook.GetNamespace()

	if resp := w.validateConnectionSecrets(ctx, req, connectionRefs, namespace); resp != nil {
		return *resp
	}

	if err := w.injectConnections(notebook, connectionRefs, namespace); err != nil {
		l.Error(err, "failed to inject connections")

		return admission.Errored(http.StatusInternalServerError, err)
	}

	w.handleStaleConnections(req, notebook, connectionRefs, namespace)

	marshaled, err := json.Marshal(notebook)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return admission.PatchResponseFromRaw(req.Object.Raw, marshaled)
}

func (w *NotebookWebhook) validateConnectionSecrets(
	ctx context.Context,
	req admission.Request,
	refs []string,
	namespace string,
) *admission.Response {
	userInfo := req.UserInfo

	for _, ref := range refs {
		secretName, secretNS := parseSecretRef(ref, namespace)

		if secretNS != namespace {
			resp := admission.Denied(fmt.Sprintf(
				"cross-namespace secret reference %q is not allowed; secrets must be in the same namespace as the Notebook",
				ref,
			))

			return &resp
		}

		secret := &corev1.Secret{}

		err := w.APIReader.Get(ctx, types.NamespacedName{Name: secretName, Namespace: secretNS}, secret)
		if err != nil {
			resp := admission.Denied(fmt.Sprintf("secret %q not found in namespace %q: %v", secretName, secretNS, err))

			return &resp
		}

		allowed, err := w.checkSecretAccess(ctx, userInfo.Username, userInfo.Groups, secretName, namespace)
		if err != nil {
			resp := admission.Errored(http.StatusInternalServerError, fmt.Errorf("failed to check access to secret %q: %w", secretName, err))

			return &resp
		}

		if !allowed {
			resp := admission.Denied(fmt.Sprintf(
				"user %q does not have permission to access secret %q in namespace %q",
				userInfo.Username, secretName, namespace,
			))

			return &resp
		}
	}

	return nil
}

func (w *NotebookWebhook) handleStaleConnections(
	req admission.Request,
	notebook *unstructured.Unstructured,
	connectionRefs []string,
	namespace string,
) {
	if req.OldObject.Raw == nil {
		return
	}

	oldNotebook := &unstructured.Unstructured{}

	if err := json.Unmarshal(req.OldObject.Raw, oldNotebook); err != nil {
		return
	}

	oldAnnotations := oldNotebook.GetAnnotations()
	if oldAnnotations == nil {
		return
	}

	oldConns, ok := oldAnnotations[metadata.ConnectionAnnotation]
	if !ok {
		return
	}

	oldRefs := parseConnectionRefs(oldConns)
	w.removeStaleConnections(notebook, oldRefs, connectionRefs, namespace)
}

func (w *NotebookWebhook) checkSecretAccess(ctx context.Context, username string, groups []string, secretName, namespace string) (bool, error) {
	sar := &authv1.SubjectAccessReview{
		Spec: authv1.SubjectAccessReviewSpec{
			User:   username,
			Groups: groups,
			ResourceAttributes: &authv1.ResourceAttributes{
				Namespace: namespace,
				Verb:      "get",
				Group:     "",
				Resource:  "secrets",
				Name:      secretName,
			},
		},
	}

	err := w.Client.Create(ctx, sar)
	if err != nil {
		return false, err
	}

	return sar.Status.Allowed, nil
}

func (w *NotebookWebhook) injectConnections(notebook *unstructured.Unstructured, refs []string, namespace string) error {
	containers, found, err := unstructured.NestedSlice(notebook.Object, "spec", "template", "spec", "containers")
	if err != nil || !found || len(containers) == 0 {
		return errNoContainers
	}

	container, ok := containers[0].(map[string]interface{})
	if !ok {
		return errInvalidContainer
	}

	var envFrom []interface{}

	if existing, found, _ := unstructured.NestedSlice(container, "envFrom"); found {
		envFrom = existing
	}

	for _, ref := range refs {
		secretName, _ := parseSecretRef(ref, namespace)

		if !hasEnvFromSecret(envFrom, secretName) {
			envFrom = append(envFrom, map[string]interface{}{
				"secretRef": map[string]interface{}{
					"name": secretName,
				},
			})
		}
	}

	container["envFrom"] = envFrom
	containers[0] = container

	return unstructured.SetNestedSlice(notebook.Object, containers, "spec", "template", "spec", "containers")
}

func (w *NotebookWebhook) removeStaleConnections(notebook *unstructured.Unstructured, oldRefs, newRefs []string, namespace string) {
	newSet := make(map[string]bool, len(newRefs))
	for _, ref := range newRefs {
		name, _ := parseSecretRef(ref, namespace)
		newSet[name] = true
	}

	containers, found, err := unstructured.NestedSlice(notebook.Object, "spec", "template", "spec", "containers")
	if err != nil || !found || len(containers) == 0 {
		return
	}

	container, ok := containers[0].(map[string]interface{})
	if !ok {
		return
	}

	envFrom, found, _ := unstructured.NestedSlice(container, "envFrom")
	if !found {
		return
	}

	filtered := filterStaleEnvFrom(envFrom, oldRefs, newSet, namespace)

	container["envFrom"] = filtered
	containers[0] = container

	_ = unstructured.SetNestedSlice(notebook.Object, containers, "spec", "template", "spec", "containers")
}

func filterStaleEnvFrom(envFrom []interface{}, oldRefs []string, newSet map[string]bool, namespace string) []interface{} {
	var filtered []interface{}

	for _, ef := range envFrom {
		efMap, ok := ef.(map[string]interface{})
		if !ok {
			filtered = append(filtered, ef)

			continue
		}

		secretRef, ok := efMap["secretRef"].(map[string]interface{})
		if !ok {
			filtered = append(filtered, ef)

			continue
		}

		name, _ := secretRef["name"].(string)
		isOldConnection := isInOldRefs(name, oldRefs, namespace)

		if !isOldConnection || newSet[name] {
			filtered = append(filtered, ef)
		}
	}

	return filtered
}

func isInOldRefs(name string, oldRefs []string, namespace string) bool {
	for _, oldRef := range oldRefs {
		oldName, _ := parseSecretRef(oldRef, namespace)
		if oldName == name {
			return true
		}
	}

	return false
}

func parseConnectionRefs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	refs := make([]string, 0, len(parts))

	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			refs = append(refs, p)
		}
	}

	return refs
}

func parseSecretRef(ref, defaultNS string) (name, namespace string) {
	parts := strings.SplitN(ref, "/", 2) //nolint:mnd
	if len(parts) == 2 {                 //nolint:mnd
		return parts[1], parts[0]
	}

	return ref, defaultNS
}

func hasEnvFromSecret(envFrom []interface{}, secretName string) bool {
	for _, ef := range envFrom {
		efMap, ok := ef.(map[string]interface{})
		if !ok {
			continue
		}

		secretRef, ok := efMap["secretRef"].(map[string]interface{})
		if !ok {
			continue
		}

		if name, _ := secretRef["name"].(string); name == secretName {
			return true
		}
	}

	return false
}
