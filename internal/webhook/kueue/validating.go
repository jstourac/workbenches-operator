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

// Package kueue implements the validating webhook for Kueue label validation on Notebooks.
// NOTE: This webhook is currently disabled. To re-enable, restore the kubebuilder:webhook
// markers and uncomment the SetupWithManager body.
package kueue

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/opendatahub-io/workbenches-operator/internal/gvk"
)

const (
	// KueueQueueNameLabel is the Kueue label that must be present on workloads.
	KueueQueueNameLabel = "kueue.x-k8s.io/queue-name"

	// KueueManagedLabelKey indicates a namespace is managed by Kueue.
	KueueManagedLabelKey = "kueue.x-k8s.io/managed"
)

var (
	errDecoderNil        = errors.New("webhook decoder not initialized")
	errMissingQueueLabel = fmt.Errorf("missing required label %q", KueueQueueNameLabel)
	errEmptyQueueLabel   = fmt.Errorf("label %q is set but empty", KueueQueueNameLabel)
)

// Webhook markers disabled — to re-enable, add the + prefix back:
// webhook:path=/validate-kueue,mutating=false,failurePolicy=fail,sideEffects=None,groups=kubeflow.org,resources=notebooks,verbs=create;update,versions=v1,name=kubeflow-kueuelabels-validator.opendatahub.io,admissionReviewVersions=v1

// Validator implements a validating admission webhook for Kueue label enforcement on Notebooks.
type Validator struct {
	Client  client.Reader
	Decoder admission.Decoder
	Name    string
}

// SetupWithManager registers the webhook. Currently disabled (no-op).
func (v *Validator) SetupWithManager(_ ctrl.Manager) error {
	return nil
}

// Handle processes admission requests for Notebook create/update, validating
// the presence of the Kueue queue-name label when the namespace is Kueue-managed.
func (v *Validator) Handle(ctx context.Context, req admission.Request) admission.Response {
	l := logf.FromContext(ctx).WithName(v.Name)

	if v.Decoder == nil {
		l.Error(nil, "Decoder is nil")

		return admission.Errored(http.StatusInternalServerError, errDecoderNil)
	}

	if req.Kind.Group != gvk.Notebook.Group || req.Kind.Kind != gvk.Notebook.Kind {
		return admission.Allowed(fmt.Sprintf("not a Notebook: %s/%s", req.Kind.Group, req.Kind.Kind))
	}

	obj := &unstructured.Unstructured{}
	if err := v.Decoder.Decode(req, obj); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if !obj.GetDeletionTimestamp().IsZero() {
		return admission.Allowed("object marked for deletion")
	}

	switch req.Operation {
	case admissionv1.Create, admissionv1.Update:
		return v.validateLabels(ctx, req.Namespace, obj)
	case admissionv1.Delete, admissionv1.Connect:
		return admission.Allowed(fmt.Sprintf("operation %s allowed", req.Operation))
	default:
		return admission.Allowed(fmt.Sprintf("operation %s allowed", req.Operation))
	}
}

func (v *Validator) validateLabels(ctx context.Context, namespace string, obj *unstructured.Unstructured) admission.Response {
	l := logf.FromContext(ctx)

	kueueManaged, err := v.isNamespaceManagedByKueue(ctx, namespace)
	if err != nil {
		l.Error(err, "failed to check namespace Kueue labels")

		return admission.Errored(http.StatusInternalServerError, err)
	}

	if !kueueManaged {
		return admission.Allowed(fmt.Sprintf("namespace %q is not Kueue-managed", namespace))
	}

	labels := obj.GetLabels()
	if labels == nil {
		return admission.Denied(errMissingQueueLabel.Error())
	}

	queueName, ok := labels[KueueQueueNameLabel]
	if !ok {
		return admission.Denied(errMissingQueueLabel.Error())
	}

	if queueName == "" {
		return admission.Denied(errEmptyQueueLabel.Error())
	}

	return admission.Allowed("Kueue label validation passed")
}

func (v *Validator) isNamespaceManagedByKueue(ctx context.Context, namespace string) (bool, error) {
	ns := &metav1.PartialObjectMetadata{}
	ns.SetGroupVersionKind(gvk.Namespace)

	if err := v.Client.Get(ctx, types.NamespacedName{Name: namespace}, ns); err != nil {
		return false, err
	}

	labels := ns.GetLabels()
	if labels == nil {
		return false, nil
	}

	return labels[KueueManagedLabelKey] == "true", nil
}
