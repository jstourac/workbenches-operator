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

package notebook

import (
	"testing"
)

func TestParseConnectionRefs(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{name: "empty", input: "", want: 0},
		{name: "single", input: "my-secret", want: 1},
		{name: "multiple", input: "secret1,secret2,secret3", want: 3},
		{name: "with spaces", input: " secret1 , secret2 ", want: 2},
		{name: "trailing comma", input: "secret1,", want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refs := parseConnectionRefs(tt.input)
			if len(refs) != tt.want {
				t.Errorf("parseConnectionRefs(%q) returned %d refs, want %d", tt.input, len(refs), tt.want)
			}
		})
	}
}

func TestParseSecretRef(t *testing.T) {
	tests := []struct {
		name      string
		ref       string
		defaultNS string
		wantName  string
		wantNS    string
	}{
		{name: "simple", ref: "my-secret", defaultNS: "ns1", wantName: "my-secret", wantNS: "ns1"},
		{name: "namespaced", ref: "other-ns/my-secret", defaultNS: "ns1", wantName: "my-secret", wantNS: "other-ns"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, ns := parseSecretRef(tt.ref, tt.defaultNS)
			if name != tt.wantName {
				t.Errorf("parseSecretRef(%q) name = %q, want %q", tt.ref, name, tt.wantName)
			}

			if ns != tt.wantNS {
				t.Errorf("parseSecretRef(%q) ns = %q, want %q", tt.ref, ns, tt.wantNS)
			}
		})
	}
}

func TestHasEnvFromSecret(t *testing.T) {
	envFrom := []interface{}{
		map[string]interface{}{
			"secretRef": map[string]interface{}{
				"name": "existing-secret",
			},
		},
	}

	if !hasEnvFromSecret(envFrom, "existing-secret") {
		t.Error("expected hasEnvFromSecret to return true for existing-secret")
	}

	if hasEnvFromSecret(envFrom, "nonexistent") {
		t.Error("expected hasEnvFromSecret to return false for nonexistent")
	}

	if hasEnvFromSecret(nil, "anything") {
		t.Error("expected hasEnvFromSecret to return false for nil slice")
	}
}

func TestIsInOldRefs(t *testing.T) {
	oldRefs := []string{"secret1", "other-ns/secret2"}

	if !isInOldRefs("secret1", oldRefs, "default") {
		t.Error("expected isInOldRefs to return true for secret1")
	}

	if !isInOldRefs("secret2", oldRefs, "default") {
		t.Error("expected isInOldRefs to return true for secret2 (namespaced ref)")
	}

	if isInOldRefs("secret3", oldRefs, "default") {
		t.Error("expected isInOldRefs to return false for secret3")
	}
}
