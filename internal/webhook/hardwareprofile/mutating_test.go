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
	containers := []interface{}{
		map[string]interface{}{"name": "sidecar"},
		map[string]interface{}{"name": "my-notebook"},
		map[string]interface{}{"name": "oauth-proxy"},
	}

	idx := findMainContainer(containers, "my-notebook")
	if idx != 1 {
		t.Errorf("expected index 1, got %d", idx)
	}

	idx = findMainContainer(containers, "nonexistent")
	if idx != -1 {
		t.Errorf("expected index -1, got %d", idx)
	}
}

func TestBuildResourceLists(t *testing.T) {
	identifiers := []interface{}{
		map[string]interface{}{
			"identifier":   "cpu",
			"minCount":     "1",
			"maxCount":     "4",
			"defaultCount": "2",
		},
		map[string]interface{}{
			"identifier":   "memory",
			"minCount":     "1Gi",
			"maxCount":     "8Gi",
			"defaultCount": "2Gi",
		},
		map[string]interface{}{
			"identifier":   "nvidia.com/gpu",
			"defaultCount": "1",
		},
	}

	requests, limits := buildResourceLists(identifiers)

	if len(requests) != 3 {
		t.Errorf("expected 3 requests, got %d", len(requests))
	}

	if len(limits) != 3 {
		t.Errorf("expected 3 limits, got %d", len(limits))
	}

	if v, ok := requests["cpu"]; !ok || v.String() != "1" {
		t.Errorf("expected cpu request of 1, got %v", v)
	}

	if v, ok := limits["cpu"]; !ok || v.String() != "4" {
		t.Errorf("expected cpu limit of 4, got %v", v)
	}

	if v, ok := requests["nvidia.com/gpu"]; !ok || v.String() != "1" {
		t.Errorf("expected nvidia.com/gpu request of 1, got %v", v)
	}
}

func TestBuildResourcesMap(t *testing.T) {
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
		t.Error("expected requests key in resources map")
	}

	if _, ok := resources["limits"]; !ok {
		t.Error("expected limits key in resources map")
	}
}

func TestApplyHardwareProfileToNotebook(t *testing.T) {
	hwp := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"spec": map[string]interface{}{
				"scheduling": map[string]interface{}{
					"node": map[string]interface{}{
						"nodeSelector": map[string]interface{}{
							"nvidia.com/gpu.present": "true",
						},
						"tolerations": []interface{}{
							map[string]interface{}{
								"key":      "nvidia.com/gpu",
								"operator": "Exists",
								"effect":   "NoSchedule",
							},
						},
					},
				},
				"identifiers": []interface{}{
					map[string]interface{}{
						"identifier": "cpu",
						"minCount":   "1",
						"maxCount":   "4",
					},
				},
			},
		},
	}

	notebook := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name":      "my-notebook",
				"namespace": "test-ns",
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name":  "my-notebook",
								"image": "test-image:latest",
							},
						},
					},
				},
			},
		},
	}

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
}
