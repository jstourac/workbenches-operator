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

// Package upgrade contains one-time migration logic for upgrading from
// the legacy component-based workbenches to the module operator.
package upgrade

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/opendatahub-io/workbenches-operator/internal/gvk"
	"github.com/opendatahub-io/workbenches-operator/internal/metadata"
)

const (
	acceleratorNameAnnotation      = "opendatahub.io/accelerator-name"
	acceleratorNamespaceAnnotation = "opendatahub.io/accelerator-profile-namespace"
	lastSizeSelectionAnnotation    = "notebooks.opendatahub.io/last-size-selection"
	containerSizeHWPPrefix         = "container-size-"
)

// MigrateNotebookAnnotations migrates AcceleratorProfile and container size annotations
// on Notebooks to HardwareProfile annotations. This is called on operator startup to
// perform one-time migration from the old annotation scheme to the new one.
func MigrateNotebookAnnotations(ctx context.Context, cli client.Client, applicationNS string) error {
	l := logf.FromContext(ctx).WithName("upgrade")

	if applicationNS == "" {
		l.Info("application namespace is empty, skipping notebook annotation migration")

		return nil
	}

	notebooks, err := getNotebooks(ctx, cli)
	if err != nil {
		return fmt.Errorf("failed to list notebooks: %w", err)
	}

	if len(notebooks) == 0 {
		l.Info("no notebooks found, skipping annotation migration")

		return nil
	}

	var errs []error

	for i := range notebooks {
		nb := &notebooks[i]
		annotations := nb.GetAnnotations()

		if annotations == nil {
			continue
		}

		// Already migrated
		if annotations[metadata.HardwareProfileNameAnnotation] != "" {
			continue
		}

		hwpName, hwpNamespace, source := determineHWPFromAnnotations(annotations, applicationNS)
		if hwpName == "" {
			continue
		}

		// Check Kueue constraints before migrating
		if shouldSkipForKueue(ctx, cli, nb) {
			l.Info("skipping HWP migration for Notebook in Kueue namespace missing queue label",
				"notebook", nb.GetName(), "namespace", nb.GetNamespace())

			continue
		}

		if err := setHardwareProfileAnnotation(ctx, cli, nb, hwpName, hwpNamespace, applicationNS); err != nil {
			l.Error(err, "failed to set HardwareProfile annotation",
				"notebook", nb.GetName(), "source", source)
			errs = append(errs, err)

			continue
		}

		l.Info("migrated annotation to HardwareProfile",
			"notebook", nb.GetName(), "source", source, "hardwareProfile", hwpName)
	}

	if len(errs) > 0 {
		return fmt.Errorf("encountered %d errors during migration: %w", len(errs), errs[0])
	}

	return nil
}

func determineHWPFromAnnotations(annotations map[string]string, _ string) (name, namespace, source string) {
	// AcceleratorProfile annotation takes priority
	if apName := annotations[acceleratorNameAnnotation]; apName != "" {
		name = strings.ReplaceAll(strings.ToLower(apName), " ", "-") + "-notebooks"
		namespace = annotations[acceleratorNamespaceAnnotation]

		return name, namespace, "AcceleratorProfile"
	}

	// Container size annotation
	if sizeSelection := annotations[lastSizeSelectionAnnotation]; sizeSelection != "" {
		name = fmt.Sprintf("%s%s-notebooks",
			containerSizeHWPPrefix,
			strings.ReplaceAll(strings.ToLower(sizeSelection), " ", "-"))

		return name, "", "ContainerSize"
	}

	return "", "", ""
}

func shouldSkipForKueue(ctx context.Context, cli client.Reader, nb *unstructured.Unstructured) bool {
	ns := &unstructured.Unstructured{}
	ns.SetGroupVersionKind(gvk.Namespace)

	err := cli.Get(ctx, types.NamespacedName{Name: nb.GetNamespace()}, ns)
	if err != nil {
		return false
	}

	nsLabels := ns.GetLabels()
	if nsLabels == nil {
		return false
	}

	if nsLabels["kueue.x-k8s.io/managed"] != "true" {
		return false
	}

	nbLabels := nb.GetLabels()
	if nbLabels == nil {
		return true
	}

	return nbLabels["kueue.x-k8s.io/queue-name"] == ""
}

func setHardwareProfileAnnotation(ctx context.Context, cli client.Client, nb *unstructured.Unstructured, hwpName, hwpNamespace, applicationNS string) error {
	annotations := nb.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}

	annotations[metadata.HardwareProfileNameAnnotation] = hwpName

	if hwpNamespace != "" {
		annotations[metadata.HardwareProfileNamespaceAnnotation] = hwpNamespace
	} else {
		annotations[metadata.HardwareProfileNamespaceAnnotation] = applicationNS
	}

	nb.SetAnnotations(annotations)

	return cli.Update(ctx, nb)
}

func getNotebooks(ctx context.Context, cli client.Reader) ([]unstructured.Unstructured, error) {
	notebookList := &unstructured.UnstructuredList{}
	notebookList.SetGroupVersionKind(gvk.Notebook)

	if err := cli.List(ctx, notebookList); err != nil {
		return nil, err
	}

	return notebookList.Items, nil
}
