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

// Package controller contains the Workbenches reconciler.
package controller

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	componentsv1alpha1 "github.com/opendatahub-io/workbenches-operator/api/v1alpha1"
)

const (
	conditionTypeReady = "Ready"
	phaseReady         = "Ready"
	phaseNotReady      = "Not Ready"
)

// WorkbenchesReconciler reconciles a Workbenches object.
type WorkbenchesReconciler struct {
	client.Client

	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=components.platform.opendatahub.io,resources=workbenches,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=components.platform.opendatahub.io,resources=workbenches/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=components.platform.opendatahub.io,resources=workbenches/finalizers,verbs=update

// Reconcile handles the reconciliation loop for Workbenches resources.
func (r *WorkbenchesReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	workbenches := &componentsv1alpha1.Workbenches{}

	err := r.Get(ctx, req.NamespacedName, workbenches)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	l.Info("reconciling Workbenches", "name", workbenches.Name, "managementState", workbenches.Spec.ManagementState)

	if workbenches.Spec.ManagementState == "Removed" {
		l.Info("workbenches management state is Removed, skipping reconciliation")

		meta.SetStatusCondition(&workbenches.Status.Conditions, metav1.Condition{
			Type:               conditionTypeReady,
			Status:             metav1.ConditionFalse,
			Reason:             "Removed",
			Message:            "Workbenches component has been removed",
			ObservedGeneration: workbenches.Generation,
		})
		workbenches.Status.Phase = phaseNotReady
		workbenches.Status.ObservedGeneration = workbenches.Generation

		err = r.Status().Update(ctx, workbenches)

		return ctrl.Result{}, err
	}

	// TODO(phase2): Implement full reconciliation action chain.
	// For now, set Ready=True as a stub.
	meta.SetStatusCondition(&workbenches.Status.Conditions, metav1.Condition{
		Type:               conditionTypeReady,
		Status:             metav1.ConditionTrue,
		Reason:             "ReconcileSuccess",
		Message:            "Workbenches component is ready",
		ObservedGeneration: workbenches.Generation,
	})
	workbenches.Status.Phase = phaseReady
	workbenches.Status.ObservedGeneration = workbenches.Generation
	workbenches.Status.WorkbenchNamespace = workbenches.Spec.WorkbenchNamespace

	err = r.Status().Update(ctx, workbenches)
	if err != nil {
		l.Error(err, "failed to update Workbenches status")

		return ctrl.Result{RequeueAfter: 10 * time.Second}, err
	}

	l.Info("reconciliation complete", "phase", workbenches.Status.Phase)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *WorkbenchesReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&componentsv1alpha1.Workbenches{}).
		Named("workbenches").
		Complete(r)
}
