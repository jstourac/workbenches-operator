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

// Package hardwareprofile implements the mutating webhook for HardwareProfile injection into Notebooks.
package hardwareprofile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/opendatahub-io/workbenches-operator/internal/gvk"
	"github.com/opendatahub-io/workbenches-operator/internal/metadata"
)

var (
	errDecoderNil   = errors.New("webhook decoder not initialized")
	errNoContainers = errors.New("no containers found in notebook spec")
)

// Injector implements a mutating admission webhook for hardware profile injection
// into Notebook resources. This module-scoped version only handles Notebooks
// (KServe owns InferenceService/LLMInferenceService).
type Injector struct {
	Client  client.Reader
	Decoder admission.Decoder
	Name    string
}

//+kubebuilder:webhook:path=/mutate-hardware-profile,mutating=true,failurePolicy=fail,groups=kubeflow.org,resources=notebooks,verbs=create;update,versions=v1,name=hardwareprofile-notebook-injector.opendatahub.io,sideEffects=None,admissionReviewVersions=v1

// SetupWithManager registers the webhook with the manager.
func (i *Injector) SetupWithManager(mgr ctrl.Manager) error {
	hookServer := mgr.GetWebhookServer()
	hookServer.Register("/mutate-hardware-profile", &webhook.Admission{
		Handler: i,
	})

	return nil
}

// Handle processes admission requests for Notebook create/update operations.
func (i *Injector) Handle(ctx context.Context, req admission.Request) admission.Response {
	l := logf.FromContext(ctx).WithName(i.Name)

	if i.Decoder == nil {
		l.Error(nil, "Decoder is nil")

		return admission.Errored(http.StatusInternalServerError, errDecoderNil)
	}

	if req.Kind.Group != gvk.Notebook.Group || req.Kind.Kind != gvk.Notebook.Kind {
		return admission.Allowed(fmt.Sprintf("not a Notebook resource: %s/%s", req.Kind.Group, req.Kind.Kind))
	}

	notebook := &unstructured.Unstructured{}
	if err := i.Decoder.Decode(req, notebook); err != nil {
		l.Error(err, "failed to decode notebook")

		return admission.Errored(http.StatusBadRequest, err)
	}

	if !notebook.GetDeletionTimestamp().IsZero() {
		return admission.Allowed("object marked for deletion")
	}

	annotations := notebook.GetAnnotations()
	if annotations == nil {
		return admission.Allowed("no annotations found")
	}

	hwpName, ok := annotations[metadata.HardwareProfileNameAnnotation]
	if !ok || hwpName == "" {
		return admission.Allowed("no hardware profile annotation found")
	}

	hwpNamespace := annotations[metadata.HardwareProfileNamespaceAnnotation]
	if hwpNamespace == "" {
		hwpNamespace = notebook.GetNamespace()
	}

	hwp := &unstructured.Unstructured{}
	hwp.SetGroupVersionKind(gvk.HardwareProfile)

	getErr := i.Client.Get(ctx, types.NamespacedName{Name: hwpName, Namespace: hwpNamespace}, hwp)
	if getErr != nil {
		if k8serr.IsNotFound(getErr) {
			l.Info("HardwareProfile not found, skipping injection", "name", hwpName, "namespace", hwpNamespace)

			return admission.Allowed(fmt.Sprintf("HardwareProfile %s/%s not found", hwpNamespace, hwpName))
		}

		return admission.Errored(http.StatusInternalServerError, fmt.Errorf("failed to get HardwareProfile: %w", getErr))
	}

	if annotations[metadata.HardwareProfileNamespaceAnnotation] == "" {
		annotations[metadata.HardwareProfileNamespaceAnnotation] = hwpNamespace
		notebook.SetAnnotations(annotations)
	}

	applyErr := applyHardwareProfileToNotebook(hwp, notebook)
	if applyErr != nil {
		l.Error(applyErr, "failed to apply hardware profile", "profile", hwpName)

		return admission.Errored(http.StatusInternalServerError, applyErr)
	}

	marshaled, err := json.Marshal(notebook)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return admission.PatchResponseFromRaw(req.Object.Raw, marshaled)
}

func applyHardwareProfileToNotebook(hwp, notebook *unstructured.Unstructured) error {
	nodeSelector, found, err := unstructured.NestedStringMap(hwp.Object, "spec", "scheduling", "node", "nodeSelector")
	if err == nil && found && len(nodeSelector) > 0 {
		nodeSelIface := make(map[string]interface{}, len(nodeSelector))
		for k, v := range nodeSelector {
			nodeSelIface[k] = v
		}

		setErr := unstructured.SetNestedField(notebook.Object, nodeSelIface, "spec", "template", "spec", "nodeSelector")
		if setErr != nil {
			return fmt.Errorf("failed to set nodeSelector: %w", setErr)
		}
	}

	tolerations, found, err := unstructured.NestedSlice(hwp.Object, "spec", "scheduling", "node", "tolerations")
	if err == nil && found && len(tolerations) > 0 {
		setErr := unstructured.SetNestedSlice(notebook.Object, tolerations, "spec", "template", "spec", "tolerations")
		if setErr != nil {
			return fmt.Errorf("failed to set tolerations: %w", setErr)
		}
	}

	identifiers, found, idErr := unstructured.NestedSlice(hwp.Object, "spec", "identifiers")
	if idErr != nil {
		return fmt.Errorf("failed to get identifiers: %w", idErr)
	}

	if !found || len(identifiers) == 0 {
		return nil
	}

	return applyResourcesToNotebookContainer(identifiers, notebook)
}

func applyResourcesToNotebookContainer(identifiers []interface{}, notebook *unstructured.Unstructured) error {
	containers, found, err := unstructured.NestedSlice(notebook.Object, "spec", "template", "spec", "containers")
	if err != nil || !found || len(containers) == 0 {
		return errNoContainers
	}

	notebookName := notebook.GetName()
	containerIdx := findMainContainer(containers, notebookName)

	if containerIdx == -1 {
		return fmt.Errorf("%w: expected container name %q", errNoContainers, notebookName)
	}

	container, _ := containers[containerIdx].(map[string]interface{})

	requests, limits := buildResourceLists(identifiers)

	if len(requests) > 0 || len(limits) > 0 {
		container["resources"] = buildResourcesMap(requests, limits)
	}

	containers[containerIdx] = container

	return unstructured.SetNestedSlice(notebook.Object, containers, "spec", "template", "spec", "containers")
}

func findMainContainer(containers []interface{}, notebookName string) int {
	for idx, c := range containers {
		cMap, ok := c.(map[string]interface{})
		if !ok {
			continue
		}

		name, _, _ := unstructured.NestedString(cMap, "name")
		if name == notebookName {
			return idx
		}
	}

	return -1
}

func buildResourceLists(identifiers []interface{}) (corev1.ResourceList, corev1.ResourceList) {
	requests := make(corev1.ResourceList)
	limits := make(corev1.ResourceList)

	for _, id := range identifiers {
		idMap, ok := id.(map[string]interface{})
		if !ok {
			continue
		}

		identifier, _, _ := unstructured.NestedString(idMap, "identifier")
		if identifier == "" {
			continue
		}

		resourceName := corev1.ResourceName(identifier)
		minCount, _, _ := unstructured.NestedString(idMap, "minCount")
		maxCount, _, _ := unstructured.NestedString(idMap, "maxCount")
		defaultCount, _, _ := unstructured.NestedString(idMap, "defaultCount")

		if minCount != "" {
			requests[resourceName] = resource.MustParse(minCount)
		} else if defaultCount != "" {
			requests[resourceName] = resource.MustParse(defaultCount)
		}

		if maxCount != "" {
			limits[resourceName] = resource.MustParse(maxCount)
		} else if defaultCount != "" {
			limits[resourceName] = resource.MustParse(defaultCount)
		}
	}

	return requests, limits
}

func buildResourcesMap(requests, limits corev1.ResourceList) map[string]interface{} {
	resources := map[string]interface{}{}

	if len(requests) > 0 {
		reqMap := make(map[string]interface{}, len(requests))
		for k, v := range requests {
			reqMap[string(k)] = v.String()
		}

		resources["requests"] = reqMap
	}

	if len(limits) > 0 {
		limMap := make(map[string]interface{}, len(limits))
		for k, v := range limits {
			limMap[string(k)] = v.String()
		}

		resources["limits"] = limMap
	}

	return resources
}
