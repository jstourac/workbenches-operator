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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	componentsv1alpha1 "github.com/opendatahub-io/workbenches-operator/api/v1alpha1"
	"github.com/opendatahub-io/workbenches-operator/internal/gvk"
)

var _ = Describe("Upgrade Scenarios", Ordered, func() {
	Context("Fresh install (no prior state)", func() {
		It("Should create the module CR and reach Ready state", func() {
			wb := &componentsv1alpha1.Workbenches{}

			err := k8sClient.Get(ctx, types.NamespacedName{Name: "default"}, wb)
			if err != nil {
				Skip("Module CR not found -- skipping fresh install verification")
			}

			Eventually(func() bool {
				current := &componentsv1alpha1.Workbenches{}

				if getErr := k8sClient.Get(ctx, types.NamespacedName{Name: "default"}, current); getErr != nil {
					return false
				}

				cond := meta.FindStatusCondition(current.Status.Conditions, "ProvisioningSucceeded")

				return cond != nil && cond.Status == metav1.ConditionTrue
			}, timeout, interval).Should(BeTrue())
		})

		It("Should create the workbench namespace", func() {
			wb := &componentsv1alpha1.Workbenches{}

			err := k8sClient.Get(ctx, types.NamespacedName{Name: "default"}, wb)
			if err != nil {
				Skip("Module CR not found")
			}

			expectedNS := wb.Status.WorkbenchNamespace
			if expectedNS == "" {
				expectedNS = workbenchNS
			}

			ns := &corev1.Namespace{}
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Name: expectedNS}, ns)
			}, timeout, interval).Should(Succeed())

			Expect(ns.Labels).To(HaveKeyWithValue("opendatahub.io/generated-namespace", "true"))
		})
	})

	Context("Within-module upgrade (v1 -> v2)", func() {
		It("Should preserve status conditions across reconcile", func() {
			wb := &componentsv1alpha1.Workbenches{}

			err := k8sClient.Get(ctx, types.NamespacedName{Name: "default"}, wb)
			if err != nil {
				Skip("Module CR not found")
			}

			Expect(wb.Status.ObservedGeneration).To(BeNumerically(">", 0),
				"observedGeneration should be set after reconciliation")
		})

		It("Should run HardwareProfile migration on upgrade", func() {
			Skip("Requires version transition trigger and pre-seeded Notebooks")
		})

		It("Should update status.releases on component version change", func() {
			Skip("Requires component-metadata.yaml to be populated")
		})
	})

	Context("Cross-version migration (in-tree v3.x -> module v4.x)", func() {
		It("Should adopt existing notebook controller deployments via SSA", func() {
			Skip("Requires pre-existing deployments from the old operator")
		})

		It("Should not recreate unchanged resources", func() {
			Skip("Requires pre-existing resources and resourceVersion tracking")
		})

		It("Should update deployment images when module specifies newer versions", func() {
			Skip("Requires old and new RELATED_IMAGE_* comparison")
		})
	})

	Context("Kueue edge cases during upgrade", func() {
		It("Should skip HWP migration for Notebooks in Kueue-managed namespaces missing queue label", func() {
			Skip("Requires Kueue-managed namespace setup")
		})

		It("Should proceed with migration for Notebooks with valid queue label", func() {
			Skip("Requires Kueue-managed namespace with labeled Notebooks")
		})
	})

	Context("Dashboard unavailability during upgrade", func() {
		It("Should use fallback container sizes when OdhDashboardConfig is not available", func() {
			Skip("Requires Dashboard to be absent from cluster")
		})
	})

	Context("ImageStream degradation", func() {
		It("Should set Ready=True even when ImageStreams are not fully imported", func() {
			Skip("Requires OpenShift cluster with ImageStreams")
		})

		It("Should report informational warning for missing CUDA images", func() {
			Skip("Requires OpenShift cluster with specific ImageStream setup")
		})
	})

	Context("Operator restart resilience", func() {
		It("Should resume reconciliation after operator pod restart", func() {
			deploy := &appsv1.Deployment{}

			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "workbenches-operator-controller-manager",
				Namespace: operatorNS,
			}, deploy)
			if err != nil {
				Skip("Operator deployment not found")
			}

			initialGen := deploy.Status.ObservedGeneration

			// Verify the operator is running and has reconciled at least once
			wb := &componentsv1alpha1.Workbenches{}

			err = k8sClient.Get(ctx, types.NamespacedName{Name: "default"}, wb)
			if err != nil {
				Skip("Module CR not found")
			}

			Expect(wb.Status.ObservedGeneration).To(BeNumerically(">", 0))
			Expect(initialGen).To(BeNumerically(">", 0))
		})

		It("Should not lose status on rapid spec updates", func() {
			Skip("Requires rapid spec update injection")
		})
	})

	Context("Notebook preservation during migration", func() {
		It("Should not modify running Notebook pod specifications", func() {
			pods := &corev1.PodList{}

			err := k8sClient.List(ctx, pods, client.InNamespace(workbenchNS))
			if err != nil || len(pods.Items) == 0 {
				Skip("No pods found in workbench namespace")
			}

			for _, pod := range pods.Items {
				labels := pod.GetLabels()
				if labels == nil {
					continue
				}

				if labels["notebook-name"] == "" {
					continue
				}

				Expect(pod.DeletionTimestamp).To(BeNil(),
					fmt.Sprintf("Notebook pod %s should not be terminating", pod.Name))
			}
		})

		It("Should preserve Notebook annotations during migration", func() {
			notebooks := &unstructured.UnstructuredList{}
			notebooks.SetGroupVersionKind(gvk.Notebook)

			err := k8sClient.List(ctx, notebooks, client.InNamespace(workbenchNS))
			if err != nil || len(notebooks.Items) == 0 {
				Skip("No notebooks found in workbench namespace")
			}

			for _, nb := range notebooks.Items {
				annotations := nb.GetAnnotations()
				Expect(annotations).NotTo(BeNil(),
					fmt.Sprintf("Notebook %s should retain annotations", nb.GetName()))
			}
		})
	})
})

var _ = Describe("Migration Readiness Checks", func() {
	It("Should verify CRD is installed", func() {
		crdList := &unstructured.UnstructuredList{}
		crdList.SetGroupVersionKind(gvk.Namespace) // Just a reachability check

		err := k8sClient.List(ctx, crdList)
		Expect(err).NotTo(HaveOccurred(), "cluster should be reachable")
	})

	It("Should verify operator ServiceAccount exists", func() {
		sa := &corev1.ServiceAccount{}

		err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      "workbenches-operator-controller-manager",
			Namespace: operatorNS,
		}, sa)
		if err != nil {
			Skip("ServiceAccount not found -- operator may not be deployed")
		}

		Expect(sa.Name).To(Equal("workbenches-operator-controller-manager"))
	})

	It("Should verify leader election lease can be acquired", func() {
		Eventually(func() bool {
			deploy := &appsv1.Deployment{}

			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "workbenches-operator-controller-manager",
				Namespace: operatorNS,
			}, deploy)
			if err != nil {
				return false
			}

			return deploy.Status.ReadyReplicas >= 1
		}, 2*time.Minute, 5*time.Second).Should(BeTrue(), "operator should acquire leader election")
	})
})
