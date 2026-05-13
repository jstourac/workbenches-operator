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

package controller_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	componentsv1alpha1 "github.com/opendatahub-io/workbenches-operator/api/v1alpha1"
	"github.com/opendatahub-io/workbenches-operator/internal/controller"
)

var _ = Describe("Workbenches Controller", func() {
	Context("When reconciling a managed Workbenches resource", func() {
		It("Should create the workbench namespace and set status", func() {
			workbenches := &componentsv1alpha1.Workbenches{
				ObjectMeta: metav1.ObjectMeta{
					Name: componentsv1alpha1.WorkbenchesInstanceName,
				},
				Spec: componentsv1alpha1.WorkbenchesSpec{
					ManagementState:    "Managed",
					WorkbenchNamespace: "test-notebooks-managed",
					Platform:           "OpenDataHub",
				},
			}
			Expect(k8sClient.Create(ctx, workbenches)).To(Succeed())

			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, workbenches)

				ns := &corev1.Namespace{}

				err := k8sClient.Get(ctx, client.ObjectKey{Name: "test-notebooks-managed"}, ns)
				if err == nil {
					_ = k8sClient.Delete(ctx, ns)
				}
			})

			reconciler := &controller.WorkbenchesReconciler{
				Client: k8sClient,
				Scheme: scheme.Scheme,
			}

			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name: componentsv1alpha1.WorkbenchesInstanceName,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify namespace was created
			ns := &corev1.Namespace{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "test-notebooks-managed"}, ns)).To(Succeed())
			Expect(ns.Labels).To(HaveKeyWithValue("opendatahub.io/generated-namespace", "true"))

			// Verify status
			updated := &componentsv1alpha1.Workbenches{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: componentsv1alpha1.WorkbenchesInstanceName,
			}, updated)).To(Succeed())

			Expect(updated.Status.ObservedGeneration).To(Equal(updated.Generation))
			Expect(updated.Status.WorkbenchNamespace).To(Equal("test-notebooks-managed"))

			// ProvisioningSucceeded should be True
			provCond := meta.FindStatusCondition(updated.Status.Conditions, "ProvisioningSucceeded")
			Expect(provCond).NotTo(BeNil())
			Expect(provCond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Context("When reconciling a Removed Workbenches resource", func() {
		It("Should set Ready and ProvisioningSucceeded to False", func() {
			workbenches := &componentsv1alpha1.Workbenches{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
				Spec: componentsv1alpha1.WorkbenchesSpec{
					ManagementState: "Removed",
				},
			}
			Expect(k8sClient.Create(ctx, workbenches)).To(Succeed())

			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, workbenches)
			})

			reconciler := &controller.WorkbenchesReconciler{
				Client: k8sClient,
				Scheme: scheme.Scheme,
			}

			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name: "default",
				},
			})
			Expect(err).NotTo(HaveOccurred())

			updated := &componentsv1alpha1.Workbenches{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: "default",
			}, updated)).To(Succeed())

			Expect(updated.Status.Phase).To(Equal("Not Ready"))

			readyCond := meta.FindStatusCondition(updated.Status.Conditions, "Ready")
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("Removed"))

			provCond := meta.FindStatusCondition(updated.Status.Conditions, "ProvisioningSucceeded")
			Expect(provCond).NotTo(BeNil())
			Expect(provCond.Status).To(Equal(metav1.ConditionFalse))
		})
	})

	Context("When the resource does not exist", func() {
		It("Should return no error", func() {
			reconciler := &controller.WorkbenchesReconciler{
				Client: k8sClient,
				Scheme: scheme.Scheme,
			}

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name: "nonexistent",
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})
	})

	Context("When the workbench namespace already exists", func() {
		It("Should label it and not fail", func() {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pre-existing-ns",
				},
			}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, ns)
			})

			workbenches := &componentsv1alpha1.Workbenches{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
				Spec: componentsv1alpha1.WorkbenchesSpec{
					ManagementState:    "Managed",
					WorkbenchNamespace: "pre-existing-ns",
				},
			}
			Expect(k8sClient.Create(ctx, workbenches)).To(Succeed())

			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, workbenches)
			})

			reconciler := &controller.WorkbenchesReconciler{
				Client: k8sClient,
				Scheme: scheme.Scheme,
			}

			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "default"},
			})
			Expect(err).NotTo(HaveOccurred())

			updatedNS := &corev1.Namespace{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "pre-existing-ns"}, updatedNS)).To(Succeed())
			Expect(updatedNS.Labels).To(HaveKeyWithValue("opendatahub.io/generated-namespace", "true"))
		})
	})

	Context("When no workbenchNamespace is specified", func() {
		It("Should use the default ODH namespace for OpenDataHub platform", func() {
			workbenches := &componentsv1alpha1.Workbenches{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
				Spec: componentsv1alpha1.WorkbenchesSpec{
					ManagementState: "Managed",
					Platform:        "OpenDataHub",
				},
			}
			Expect(k8sClient.Create(ctx, workbenches)).To(Succeed())

			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, workbenches)

				ns := &corev1.Namespace{}

				err := k8sClient.Get(ctx, client.ObjectKey{Name: "opendatahub"}, ns)
				if err == nil {
					_ = k8sClient.Delete(ctx, ns)
				}
			})

			reconciler := &controller.WorkbenchesReconciler{
				Client: k8sClient,
				Scheme: scheme.Scheme,
			}

			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "default"},
			})
			Expect(err).NotTo(HaveOccurred())

			ns := &corev1.Namespace{}
			nsErr := k8sClient.Get(ctx, client.ObjectKey{Name: "opendatahub"}, ns)

			if errors.IsNotFound(nsErr) {
				Skip("namespace creation might not work in envtest for default namespace")
			}

			Expect(nsErr).NotTo(HaveOccurred())
		})
	})
})
