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

package upgrade

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	componentsv1alpha1 "github.com/opendatahub-io/workbenches-operator/api/v1alpha1"
	"github.com/opendatahub-io/workbenches-operator/internal/gvk"
)

const (
	timeout  = 5 * time.Minute
	interval = 5 * time.Second
)

var _ = Describe("Component-to-Module Migration", Ordered, func() {
	Context("Post-migration verification", func() {
		It("Should have the module operator deployment running", func() {
			deploy := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      "workbenches-operator-controller-manager",
					Namespace: operatorNS,
				}, deploy)
			}, timeout, interval).Should(Succeed())

			Expect(deploy.Status.ReadyReplicas).To(BeNumerically(">=", 1),
				"module operator should have at least one ready replica")
		})

		It("Should have the module CR created and in Ready state", func() {
			Eventually(func() bool {
				wb := &componentsv1alpha1.Workbenches{}

				err := k8sClient.Get(ctx, types.NamespacedName{Name: "default"}, wb)
				if err != nil {
					return false
				}

				cond := meta.FindStatusCondition(wb.Status.Conditions, "Ready")

				return cond != nil && cond.Status == metav1.ConditionTrue
			}, timeout, interval).Should(BeTrue(), "module CR should reach Ready state")
		})

		It("Should have adopted the workbench namespace", func() {
			ns := &corev1.Namespace{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: workbenchNS}, ns)).To(Succeed())
			Expect(ns.Labels).To(HaveKeyWithValue("opendatahub.io/generated-namespace", "true"))
		})

		It("Should have the old in-tree component CR removed", func() {
			Skip("Requires the old ODH operator to have been deployed first")

			oldCR := &unstructured.Unstructured{}
			oldCR.SetGroupVersionKind(gvk.Notebook)
			oldCR.SetName("default-workbenches")

			err := k8sClient.Get(ctx, types.NamespacedName{Name: "default-workbenches"}, oldCR)
			Expect(errors.IsNotFound(err)).To(BeTrue(), "old component CR should have been deleted")
		})
	})

	Context("Zero-downtime verification", func() {
		It("Should not restart notebook controller pods during migration", func() {
			Skip("Requires pre-migration pod restart count baseline")

			pods := &corev1.PodList{}
			Expect(k8sClient.List(ctx, pods,
				client.InNamespace(workbenchNS),
				client.MatchingLabels{"app": "notebook-controller"},
			)).To(Succeed())

			for _, pod := range pods.Items {
				for _, cs := range pod.Status.ContainerStatuses {
					Expect(cs.RestartCount).To(BeNumerically("==", 0),
						fmt.Sprintf("pod %s container %s should have zero restarts", pod.Name, cs.Name))
				}
			}
		})

		It("Should preserve existing Notebooks without modification", func() {
			Skip("Requires pre-migration Notebook baseline")

			notebooks := &unstructured.UnstructuredList{}
			notebooks.SetGroupVersionKind(gvk.Notebook)

			err := k8sClient.List(ctx, notebooks, client.InNamespace(workbenchNS))
			if err != nil {
				return
			}

			for _, nb := range notebooks.Items {
				Expect(nb.GetDeletionTimestamp()).To(BeNil(),
					fmt.Sprintf("Notebook %s should not be marked for deletion", nb.GetName()))
			}
		})
	})

	Context("HardwareProfile annotation migration", func() {
		It("Should migrate AcceleratorProfile annotations to HardwareProfile", func() {
			Skip("Requires pre-seeded Notebooks with AcceleratorProfile annotations")

			notebooks := &unstructured.UnstructuredList{}
			notebooks.SetGroupVersionKind(gvk.Notebook)

			err := k8sClient.List(ctx, notebooks, client.InNamespace(workbenchNS))
			if err != nil || len(notebooks.Items) == 0 {
				return
			}

			for _, nb := range notebooks.Items {
				annotations := nb.GetAnnotations()
				if annotations == nil {
					continue
				}

				if annotations["opendatahub.io/accelerator-name"] != "" {
					Expect(annotations["opendatahub.io/hardware-profile-name"]).NotTo(BeEmpty(),
						fmt.Sprintf("Notebook %s with AP annotation should have HWP annotation after migration", nb.GetName()))
				}
			}
		})

		It("Should migrate container size annotations to HardwareProfile", func() {
			Skip("Requires pre-seeded Notebooks with container size annotations")

			notebooks := &unstructured.UnstructuredList{}
			notebooks.SetGroupVersionKind(gvk.Notebook)

			err := k8sClient.List(ctx, notebooks, client.InNamespace(workbenchNS))
			if err != nil || len(notebooks.Items) == 0 {
				return
			}

			for _, nb := range notebooks.Items {
				annotations := nb.GetAnnotations()
				if annotations == nil {
					continue
				}

				if annotations["notebooks.opendatahub.io/last-size-selection"] != "" {
					Expect(annotations["opendatahub.io/hardware-profile-name"]).NotTo(BeEmpty(),
						fmt.Sprintf("Notebook %s with size annotation should have HWP annotation after migration", nb.GetName()))
				}
			}
		})

		It("Should skip migration for Notebooks in Kueue-managed namespaces without queue label", func() {
			Skip("Requires pre-seeded Kueue-managed namespace and Notebooks")
		})
	})

	Context("Webhook continuity", func() {
		It("Should have the connection webhook registered", func() {
			Skip("Requires webhook infrastructure to be fully deployed")
		})

		It("Should have the hardware profile webhook registered", func() {
			Skip("Requires webhook infrastructure to be fully deployed")
		})
	})

	Context("Rollback verification", func() {
		It("Should allow rollback to in-tree component when module operator is removed", func() {
			Skip("Requires coordinated rollback test with ODH operator downgrade")
		})
	})
})
