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

// Package workbenches contains the ModuleHandler reference implementation for
// the workbenches module operator. This code is intended to be contributed to
// the opendatahub-operator repository at:
//
//	internal/controller/modules/workbenches/handler.go
//
// It is NOT compiled as part of the workbenches-operator binary; it lives here
// as a reference for the ODH operator contribution.
package workbenches

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	componentName = "workbenches"
	instanceName  = "default"
)

// ModuleHandlerConfig holds the projected configuration the orchestrator
// extracts from DSC / DSCI resources and passes to the module CR.
type ModuleHandlerConfig struct {
	ManagementState    string
	WorkbenchNamespace string
	GatewayDomain      string
	Platform           string
	MLflowEnabled      bool
}

// Handler implements the ModuleHandler interface defined in odh-platform-utilities.
// It bridges the gap between the DSC component definition and the standalone
// workbenches module operator by:
//   - Determining whether the workbenches component is enabled (IsEnabled)
//   - Building the module CR spec from DSC/DSCI fields (BuildModuleCR)
//   - Providing the GVK and instance name for the module (ModuleGVK, InstanceName)
type Handler struct {
	Client client.Client
}

// ModuleGVK returns the GroupVersionKind of the module CR that this handler manages.
func (h *Handler) ModuleGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   "components.platform.opendatahub.io",
		Version: "v1alpha1",
		Kind:    "Workbenches",
	}
}

// InstanceName returns the singleton name for the module CR.
func (h *Handler) InstanceName() string {
	return instanceName
}

// ComponentName returns the component name used for logging and identification.
func (h *Handler) ComponentName() string {
	return componentName
}

// IsEnabled checks whether the workbenches component is enabled in the DSC.
// In a real ODH operator integration this reads:
//
//	dsc.Spec.Components.Workbenches.ManagementState == operatorv1.Managed
func (h *Handler) IsEnabled(dsc *unstructured.Unstructured) (bool, error) {
	state, found, err := unstructured.NestedString(
		dsc.Object,
		"spec", "components", "workbenches", "managementState",
	)
	if err != nil {
		return false, fmt.Errorf("failed to read workbenches managementState: %w", err)
	}

	if !found {
		return false, nil
	}

	return state == "Managed", nil
}

// BuildModuleCR creates the module CR spec by projecting fields from DSC and DSCI.
//
// The orchestrator calls this method when it detects the workbenches component
// is enabled. The returned unstructured object is applied (SSA) to the cluster,
// where the module operator picks it up.
//
// Fields projected from the orchestrator:
//   - managementState: from DSC spec
//   - workbenchNamespace: from DSCI applicationNamespace or platform default
//   - gatewayDomain: from gateway configuration
//   - platform: detected platform type
//   - mlflowEnabled: from DSC MLflowOperator state
func (h *Handler) BuildModuleCR(ctx context.Context, cfg ModuleHandlerConfig) (*unstructured.Unstructured, error) {
	cr := &unstructured.Unstructured{}
	cr.SetGroupVersionKind(h.ModuleGVK())
	cr.SetName(instanceName)

	spec := map[string]interface{}{
		"managementState": cfg.ManagementState,
	}

	if cfg.WorkbenchNamespace != "" {
		spec["workbenchNamespace"] = cfg.WorkbenchNamespace
	}

	if cfg.GatewayDomain != "" {
		spec["gatewayDomain"] = cfg.GatewayDomain
	}

	if cfg.Platform != "" {
		spec["platform"] = cfg.Platform
	}

	spec["mlflowEnabled"] = cfg.MLflowEnabled

	if setErr := unstructured.SetNestedField(cr.Object, spec, "spec"); setErr != nil {
		return nil, fmt.Errorf("failed to set module CR spec: %w", setErr)
	}

	_ = ctx

	return cr, nil
}

// MigrationAnnotations returns annotations that should be applied during
// component-to-module migration. These help the orchestrator track the
// migration state for the old in-tree Workbenches component CR.
func (h *Handler) MigrationAnnotations() map[string]string {
	return map[string]string{
		"platform.opendatahub.io/migrated-to-module": "workbenches-operator",
	}
}
