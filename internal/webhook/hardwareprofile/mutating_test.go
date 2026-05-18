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

package hardwareprofile

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestFindMainContainer(t *testing.T) {
	tests := []struct {
		name       string
		containers []interface{}
		notebook   string
		wantIdx    int
	}{
		{
			name: "finds matching container",
			containers: []interface{}{
				map[string]interface{}{"name": "sidecar"},
				map[string]interface{}{"name": "my-notebook"},
				map[string]interface{}{"name": "oauth-proxy"},
			},
			notebook: "my-notebook",
			wantIdx:  1,
		},
		{
			name: "returns -1 for no match",
			containers: []interface{}{
				map[string]interface{}{"name": "sidecar"},
			},
			notebook: "nonexistent",
			wantIdx:  -1,
		},
		{
			name:       "returns -1 for empty slice",
			containers: []interface{}{},
			notebook:   "notebook",
			wantIdx:    -1,
		},
		{
			name: "handles invalid container type",
			containers: []interface{}{
				"not-a-map",
				map[string]interface{}{"name": "target"},
			},
			notebook: "target",
			wantIdx:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx := findMainContainer(tt.containers, tt.notebook)
			if idx != tt.wantIdx {
				t.Errorf("findMainContainer() = %d, want %d", idx, tt.wantIdx)
			}
		})
	}
}

func TestBuildResourceLists(t *testing.T) {
	t.Run("minCount and maxCount take priority over defaultCount", func(t *testing.T) {
		identifiers := []interface{}{
			map[string]interface{}{
				"identifier":   "cpu",
				"minCount":     "1",
				"maxCount":     "4",
				"defaultCount": "2",
			},
		}

		requests, limits := buildResourceLists(identifiers)

		if v := requests["cpu"]; v.String() != "1" {
			t.Errorf("expected cpu request 1, got %s", v.String())
		}

		if v := limits["cpu"]; v.String() != "4" {
			t.Errorf("expected cpu limit 4, got %s", v.String())
		}
	})

	t.Run("defaultCount used when min/max absent", func(t *testing.T) {
		identifiers := []interface{}{
			map[string]interface{}{
				"identifier":   "nvidia.com/gpu",
				"defaultCount": "1",
			},
		}

		requests, limits := buildResourceLists(identifiers)

		if v := requests["nvidia.com/gpu"]; v.String() != "1" {
			t.Errorf("expected gpu request 1, got %s", v.String())
		}

		if v := limits["nvidia.com/gpu"]; v.String() != "1" {
			t.Errorf("expected gpu limit 1, got %s", v.String())
		}
	})

	t.Run("skips entries without identifier", func(t *testing.T) {
		identifiers := []interface{}{
			map[string]interface{}{"displayName": "CPU", "defaultCount": "1"},
		}

		requests, limits := buildResourceLists(identifiers)

		if len(requests) != 0 {
			t.Errorf("expected 0 requests, got %d", len(requests))
		}

		if len(limits) != 0 {
			t.Errorf("expected 0 limits, got %d", len(limits))
		}
	})

	t.Run("handles memory resources", func(t *testing.T) {
		identifiers := []interface{}{
			map[string]interface{}{
				"identifier": "memory",
				"minCount":   "1Gi",
				"maxCount":   "8Gi",
			},
		}

		requests, limits := buildResourceLists(identifiers)

		if v := requests["memory"]; v.String() != "1Gi" {
			t.Errorf("expected memory request 1Gi, got %s", v.String())
		}

		if v := limits["memory"]; v.String() != "8Gi" {
			t.Errorf("expected memory limit 8Gi, got %s", v.String())
		}
	})
}

func TestBuildResourcesMap(t *testing.T) {
	t.Run("builds map with both requests and limits", func(t *testing.T) {
		identifiers := []interface{}{
			map[string]interface{}{
				"identifier": "cpu",
				"minCount":   "100m",
				"maxCount":   "2",
			},
		}

		requests, limits := buildResourceLists(identifiers)
		resources := buildResourcesMap(requests, limits)

		if _, ok := resources["requests"]; !ok {
			t.Error("expected requests key")
		}

		if _, ok := resources["limits"]; !ok {
			t.Error("expected limits key")
		}
	})

	t.Run("omits empty requests or limits", func(t *testing.T) {
		identifiers := []interface{}{}

		requests, limits := buildResourceLists(identifiers)
		resources := buildResourcesMap(requests, limits)

		if _, ok := resources["requests"]; ok {
			t.Error("expected no requests key for empty identifiers")
		}
	})
}

func TestApplyHardwareProfileToNotebook(t *testing.T) {
	t.Run("applies nodeSelector tolerations and resources", func(t *testing.T) {
		hwp := makeHWP(map[string]interface{}{
			"nvidia.com/gpu.present": "true",
		}, []interface{}{
			map[string]interface{}{
				"key":      "nvidia.com/gpu",
				"operator": "Exists",
				"effect":   "NoSchedule",
			},
		}, []interface{}{
			map[string]interface{}{
				"identifier": "cpu",
				"minCount":   "1",
				"maxCount":   "4",
			},
		})

		notebook := makeNotebook("my-notebook", "test-ns")

		err := applyHardwareProfileToNotebook(hwp, notebook)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		nodeSelector, _, _ := unstructured.NestedStringMap(notebook.Object, "spec", "template", "spec", "nodeSelector")
		if nodeSelector["nvidia.com/gpu.present"] != "true" {
			t.Error("expected nodeSelector to be set")
		}

		tolerations, _, _ := unstructured.NestedSlice(notebook.Object, "spec", "template", "spec", "tolerations")
		if len(tolerations) != 1 {
			t.Errorf("expected 1 toleration, got %d", len(tolerations))
		}
	})

	t.Run("succeeds with empty identifiers", func(t *testing.T) {
		hwp := makeHWP(nil, nil, nil)
		notebook := makeNotebook("nb", "ns")

		err := applyHardwareProfileToNotebook(hwp, notebook)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("returns error for missing main container", func(t *testing.T) {
		hwp := makeHWP(nil, nil, []interface{}{
			map[string]interface{}{"identifier": "cpu", "defaultCount": "1"},
		})

		notebook := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"metadata": map[string]interface{}{"name": "nb", "namespace": "ns"},
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"containers": []interface{}{
								map[string]interface{}{"name": "wrong-name", "image": "img"},
							},
						},
					},
				},
			},
		}

		err := applyHardwareProfileToNotebook(hwp, notebook)
		if err == nil {
			t.Error("expected error for missing container, got nil")
		}
	})
}

func makeHWP(nodeSelector map[string]interface{}, tolerations, identifiers []interface{}) *unstructured.Unstructured {
	spec := map[string]interface{}{}

	if nodeSelector != nil || tolerations != nil {
		node := map[string]interface{}{}
		if nodeSelector != nil {
			node["nodeSelector"] = nodeSelector
		}

		if tolerations != nil {
			node["tolerations"] = tolerations
		}

		spec["scheduling"] = map[string]interface{}{"node": node}
	}

	if identifiers != nil {
		spec["identifiers"] = identifiers
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{"spec": spec},
	}
}

func makeNotebook(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{"name": name, "namespace": namespace},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{"name": name, "image": "test-image:latest"},
						},
					},
				},
			},
		},
	}
}
