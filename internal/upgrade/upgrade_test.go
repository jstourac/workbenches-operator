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
	"testing"
)

func TestDetermineHWPFromAnnotations(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		wantName    string
		wantNS      string
		wantSource  string
	}{
		{
			name:        "no annotations",
			annotations: map[string]string{},
			wantName:    "",
			wantSource:  "",
		},
		{
			name: "accelerator profile simple",
			annotations: map[string]string{
				acceleratorNameAnnotation: "NVIDIA A100",
			},
			wantName:   "nvidia-a100-notebooks",
			wantSource: "AcceleratorProfile",
		},
		{
			name: "accelerator with spaces and caps",
			annotations: map[string]string{
				acceleratorNameAnnotation: "Small GPU",
			},
			wantName:   "small-gpu-notebooks",
			wantSource: "AcceleratorProfile",
		},
		{
			name: "accelerator with namespace",
			annotations: map[string]string{
				acceleratorNameAnnotation:      "small-gpu",
				acceleratorNamespaceAnnotation: "redhat-ods-apps",
			},
			wantName:   "small-gpu-notebooks",
			wantNS:     "redhat-ods-apps",
			wantSource: "AcceleratorProfile",
		},
		{
			name: "container size",
			annotations: map[string]string{
				lastSizeSelectionAnnotation: "Large",
			},
			wantName:   "container-size-large-notebooks",
			wantSource: "ContainerSize",
		},
		{
			name: "container size with spaces",
			annotations: map[string]string{
				lastSizeSelectionAnnotation: "Extra Large",
			},
			wantName:   "container-size-extra-large-notebooks",
			wantSource: "ContainerSize",
		},
		{
			name: "accelerator takes priority over container size",
			annotations: map[string]string{
				acceleratorNameAnnotation:   "T4",
				lastSizeSelectionAnnotation: "Medium",
			},
			wantName:   "t4-notebooks",
			wantSource: "AcceleratorProfile",
		},
		{
			name: "empty accelerator falls through to container size",
			annotations: map[string]string{
				acceleratorNameAnnotation:   "",
				lastSizeSelectionAnnotation: "Small",
			},
			wantName:   "container-size-small-notebooks",
			wantSource: "ContainerSize",
		},
		{
			name: "empty both returns nothing",
			annotations: map[string]string{
				acceleratorNameAnnotation:   "",
				lastSizeSelectionAnnotation: "",
			},
			wantName:   "",
			wantSource: "",
		},
		{
			name: "unrelated annotations are ignored",
			annotations: map[string]string{
				"some-other-annotation": "value",
			},
			wantName:   "",
			wantSource: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, ns, source := determineHWPFromAnnotations(tt.annotations, "app-ns")
			if name != tt.wantName {
				t.Errorf("name = %q, want %q", name, tt.wantName)
			}

			if ns != tt.wantNS {
				t.Errorf("ns = %q, want %q", ns, tt.wantNS)
			}

			if source != tt.wantSource {
				t.Errorf("source = %q, want %q", source, tt.wantSource)
			}
		})
	}
}

func TestShouldSkipForKueueConstants(t *testing.T) {
	if acceleratorNameAnnotation != "opendatahub.io/accelerator-name" {
		t.Errorf("unexpected acceleratorNameAnnotation: %s", acceleratorNameAnnotation)
	}

	if lastSizeSelectionAnnotation != "notebooks.opendatahub.io/last-size-selection" {
		t.Errorf("unexpected lastSizeSelectionAnnotation: %s", lastSizeSelectionAnnotation)
	}

	if containerSizeHWPPrefix != "container-size-" {
		t.Errorf("unexpected containerSizeHWPPrefix: %s", containerSizeHWPPrefix)
	}
}
