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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"

	componentsv1alpha1 "github.com/opendatahub-io/workbenches-operator/api/v1alpha1"
	"github.com/opendatahub-io/workbenches-operator/internal/controller"
)

var _ = Describe("Workbenches Controller", func() {
	Context("When reconciling a Workbenches resource", func() {
		It("Should set Ready condition to True for a managed workbenches", func() {
			workbenches := &componentsv1alpha1.Workbenches{
				ObjectMeta: metav1.ObjectMeta{
					Name: componentsv1alpha1.WorkbenchesInstanceName,
				},
				Spec: componentsv1alpha1.WorkbenchesSpec{
					ManagementState:    "Managed",
					WorkbenchNamespace: "test-notebooks",
				},
			}
			Expect(k8sClient.Create(ctx, workbenches)).To(Succeed())

			DeferCleanup(func() {
				Expect(k8sClient.Delete(ctx, workbenches)).To(Succeed())
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

			updated := &componentsv1alpha1.Workbenches{}

			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: componentsv1alpha1.WorkbenchesInstanceName,
			}, updated)).To(Succeed())

			Expect(updated.Status.Phase).To(Equal("Ready"))
			Expect(updated.Status.ObservedGeneration).To(Equal(updated.Generation))
			Expect(updated.Status.WorkbenchNamespace).To(Equal("test-notebooks"))

			readyCond := meta.FindStatusCondition(updated.Status.Conditions, "Ready")
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCond.Reason).To(Equal("ReconcileSuccess"))
		})

		It("Should set Ready condition to False when management state is Removed", func() {
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
				Expect(k8sClient.Delete(ctx, workbenches)).To(Succeed())
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
		})

		It("Should return no error when the resource does not exist", func() {
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
})
