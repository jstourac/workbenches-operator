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

package e2e

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	componentsv1alpha1 "github.com/opendatahub-io/workbenches-operator/api/v1alpha1"
)

const (
	timeout  = 2 * time.Minute
	interval = 5 * time.Second
)

var _ = Describe("Workbenches E2E", Ordered, func() {
	Context("Component lifecycle", func() {
		It("Should deploy the operator successfully", func() {
			deploy := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      "workbenches-operator-controller-manager",
					Namespace: operatorNS,
				}, deploy)
			}, timeout, interval).Should(Succeed())

			Expect(deploy.Status.ReadyReplicas).To(BeNumerically(">=", 1))
		})

		It("Should create a Workbenches CR and become ready", func() {
			wb := &componentsv1alpha1.Workbenches{
				ObjectMeta: metav1.ObjectMeta{
					Name: componentsv1alpha1.WorkbenchesInstanceName,
				},
				Spec: componentsv1alpha1.WorkbenchesSpec{
					ManagementState:    "Managed",
					WorkbenchNamespace: "e2e-test-notebooks",
					Platform:           "OpenDataHub",
				},
			}

			err := k8sClient.Create(ctx, wb)
			if errors.IsAlreadyExists(err) {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: wb.Name}, wb)).To(Succeed())
			} else {
				Expect(err).NotTo(HaveOccurred())
			}

			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, wb)
			})

			Eventually(func() bool {
				current := &componentsv1alpha1.Workbenches{}

				err := k8sClient.Get(ctx, types.NamespacedName{Name: wb.Name}, current)
				if err != nil {
					return false
				}

				cond := meta.FindStatusCondition(current.Status.Conditions, "ProvisioningSucceeded")

				return cond != nil && cond.Status == metav1.ConditionTrue
			}, timeout, interval).Should(BeTrue(), "Workbenches should have ProvisioningSucceeded=True")
		})
	})

	Context("Workbench namespace", func() {
		It("Should create the configured workbench namespace", func() {
			ns := &corev1.Namespace{}
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Name: "e2e-test-notebooks"}, ns)
			}, timeout, interval).Should(Succeed())

			Expect(ns.Labels).To(HaveKeyWithValue("opendatahub.io/generated-namespace", "true"))
		})
	})

	Context("Removal", func() {
		It("Should transition to Not Ready when management state is Removed", func() {
			wb := &componentsv1alpha1.Workbenches{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: componentsv1alpha1.WorkbenchesInstanceName,
			}, wb)).To(Succeed())

			wb.Spec.ManagementState = "Removed"
			Expect(k8sClient.Update(ctx, wb)).To(Succeed())

			Eventually(func() string {
				current := &componentsv1alpha1.Workbenches{}

				err := k8sClient.Get(ctx, types.NamespacedName{Name: wb.Name}, current)
				if err != nil {
					return ""
				}

				return current.Status.Phase
			}, timeout, interval).Should(Equal("Not Ready"))
		})
	})

	Context("Deletion recovery", func() {
		It("Should recreate deleted ConfigMap on next reconcile", func() {
			Skip("Requires full manifest rendering to be implemented")
		})

		It("Should recreate deleted Service on next reconcile", func() {
			Skip("Requires full manifest rendering to be implemented")
		})

		It("Should recreate deleted Deployment on next reconcile", func() {
			Skip("Requires full manifest rendering to be implemented")
		})
	})

	Context("MLflow integration", func() {
		It("Should pass mlflow-enabled flag to notebook controller", func() {
			Skip("Requires full manifest rendering and params injection")
		})
	})
})
