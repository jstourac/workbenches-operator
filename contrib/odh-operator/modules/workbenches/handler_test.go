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

package workbenches

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestIsEnabled(t *testing.T) {
	h := &Handler{}

	tests := []struct {
		name    string
		dsc     map[string]interface{}
		want    bool
		wantErr bool
	}{
		{
			name: "enabled when Managed",
			dsc: map[string]interface{}{
				"spec": map[string]interface{}{
					"components": map[string]interface{}{
						"workbenches": map[string]interface{}{
							"managementState": "Managed",
						},
					},
				},
			},
			want: true,
		},
		{
			name: "disabled when Removed",
			dsc: map[string]interface{}{
				"spec": map[string]interface{}{
					"components": map[string]interface{}{
						"workbenches": map[string]interface{}{
							"managementState": "Removed",
						},
					},
				},
			},
			want: false,
		},
		{
			name: "disabled when field missing",
			dsc: map[string]interface{}{
				"spec": map[string]interface{}{
					"components": map[string]interface{}{},
				},
			},
			want: false,
		},
		{
			name: "disabled when spec missing",
			dsc:  map[string]interface{}{},
			want: false,
		},
		{
			name: "disabled when empty string",
			dsc: map[string]interface{}{
				"spec": map[string]interface{}{
					"components": map[string]interface{}{
						"workbenches": map[string]interface{}{
							"managementState": "",
						},
					},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dsc := &unstructured.Unstructured{Object: tt.dsc}

			got, err := h.IsEnabled(dsc)
			if (err != nil) != tt.wantErr {
				t.Fatalf("IsEnabled() error = %v, wantErr %v", err, tt.wantErr)
			}

			if got != tt.want {
				t.Errorf("IsEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildModuleCR(t *testing.T) {
	h := &Handler{}
	ctx := context.Background()

	t.Run("full config", func(t *testing.T) {
		cfg := ModuleHandlerConfig{
			ManagementState:    "Managed",
			WorkbenchNamespace: "rhods-notebooks",
			GatewayDomain:      "gateway.apps.cluster.example.com",
			Platform:           "SelfManagedRhoai",
			MLflowEnabled:      true,
		}

		cr, err := h.BuildModuleCR(ctx, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if cr.GetName() != "default" {
			t.Errorf("expected name 'default', got %q", cr.GetName())
		}

		if cr.GetKind() != "Workbenches" {
			t.Errorf("expected kind 'Workbenches', got %q", cr.GetKind())
		}

		assertSpecField(t, cr, "managementState", "Managed")
		assertSpecField(t, cr, "workbenchNamespace", "rhods-notebooks")
		assertSpecField(t, cr, "gatewayDomain", "gateway.apps.cluster.example.com")
		assertSpecField(t, cr, "platform", "SelfManagedRhoai")
		assertSpecFieldBool(t, cr, "mlflowEnabled", true)
	})

	t.Run("minimal config", func(t *testing.T) {
		cfg := ModuleHandlerConfig{
			ManagementState: "Managed",
		}

		cr, err := h.BuildModuleCR(ctx, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		assertSpecField(t, cr, "managementState", "Managed")
		assertSpecFieldBool(t, cr, "mlflowEnabled", false)

		// Optional fields should not be present when empty
		if _, found, _ := unstructured.NestedString(cr.Object, "spec", "workbenchNamespace"); found {
			t.Error("workbenchNamespace should not be set when empty")
		}

		if _, found, _ := unstructured.NestedString(cr.Object, "spec", "gatewayDomain"); found {
			t.Error("gatewayDomain should not be set when empty")
		}
	})

	t.Run("Removed state", func(t *testing.T) {
		cfg := ModuleHandlerConfig{
			ManagementState: "Removed",
		}

		cr, err := h.BuildModuleCR(ctx, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		assertSpecField(t, cr, "managementState", "Removed")
	})
}

func TestModuleGVK(t *testing.T) {
	h := &Handler{}
	gvk := h.ModuleGVK()

	if gvk.Group != "components.platform.opendatahub.io" {
		t.Errorf("expected group 'components.platform.opendatahub.io', got %q", gvk.Group)
	}

	if gvk.Version != "v1alpha1" {
		t.Errorf("expected version 'v1alpha1', got %q", gvk.Version)
	}

	if gvk.Kind != "Workbenches" {
		t.Errorf("expected kind 'Workbenches', got %q", gvk.Kind)
	}
}

func TestInstanceName(t *testing.T) {
	h := &Handler{}
	if h.InstanceName() != "default" {
		t.Errorf("expected instance name 'default', got %q", h.InstanceName())
	}
}

func TestComponentName(t *testing.T) {
	h := &Handler{}
	if h.ComponentName() != "workbenches" {
		t.Errorf("expected component name 'workbenches', got %q", h.ComponentName())
	}
}

func TestMigrationAnnotations(t *testing.T) {
	h := &Handler{}
	annotations := h.MigrationAnnotations()

	if annotations["platform.opendatahub.io/migrated-to-module"] != "workbenches-operator" {
		t.Error("missing expected migration annotation")
	}
}

func assertSpecField(t *testing.T, cr *unstructured.Unstructured, field, expected string) {
	t.Helper()

	val, found, err := unstructured.NestedString(cr.Object, "spec", field)
	if err != nil {
		t.Fatalf("error reading spec.%s: %v", field, err)
	}

	if !found {
		t.Fatalf("spec.%s not found", field)
	}

	if val != expected {
		t.Errorf("spec.%s = %q, want %q", field, val, expected)
	}
}

func assertSpecFieldBool(t *testing.T, cr *unstructured.Unstructured, field string, expected bool) {
	t.Helper()

	val, found, err := unstructured.NestedBool(cr.Object, "spec", field)
	if err != nil {
		t.Fatalf("error reading spec.%s: %v", field, err)
	}

	if !found {
		t.Fatalf("spec.%s not found", field)
	}

	if val != expected {
		t.Errorf("spec.%s = %v, want %v", field, val, expected)
	}
}
