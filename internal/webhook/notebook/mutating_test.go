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
		first string
	}{
		{name: "empty", input: "", want: 0},
		{name: "single ref", input: "my-secret", want: 1, first: "my-secret"},
		{name: "multiple refs", input: "s1,s2,s3", want: 3, first: "s1"},
		{name: "with spaces", input: " s1 , s2 ", want: 2, first: "s1"},
		{name: "trailing comma", input: "s1,", want: 1, first: "s1"},
		{name: "leading comma", input: ",s1", want: 1, first: "s1"},
		{name: "only commas", input: ",,,", want: 0},
		{name: "whitespace only", input: "   ", want: 0},
		{name: "namespaced ref", input: "ns1/secret1", want: 1, first: "ns1/secret1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refs := parseConnectionRefs(tt.input)
			if len(refs) != tt.want {
				t.Errorf("parseConnectionRefs(%q) len = %d, want %d", tt.input, len(refs), tt.want)
			}

			if tt.want > 0 && refs[0] != tt.first {
				t.Errorf("parseConnectionRefs(%q)[0] = %q, want %q", tt.input, refs[0], tt.first)
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
		{name: "simple ref", ref: "my-secret", defaultNS: "ns1", wantName: "my-secret", wantNS: "ns1"},
		{name: "namespaced ref", ref: "other-ns/my-secret", defaultNS: "ns1", wantName: "my-secret", wantNS: "other-ns"},
		{name: "multiple slashes", ref: "ns/path/secret", defaultNS: "ns1", wantName: "path/secret", wantNS: "ns"},
		{name: "empty ref", ref: "", defaultNS: "ns1", wantName: "", wantNS: "ns1"},
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
			"secretRef": map[string]interface{}{"name": "existing-secret"},
		},
		map[string]interface{}{
			"configMapRef": map[string]interface{}{"name": "some-config"},
		},
	}

	tests := []struct {
		name       string
		envFrom    []interface{}
		secretName string
		want       bool
	}{
		{name: "found", envFrom: envFrom, secretName: "existing-secret", want: true},
		{name: "not found", envFrom: envFrom, secretName: "nonexistent", want: false},
		{name: "nil slice", envFrom: nil, secretName: "anything", want: false},
		{name: "empty slice", envFrom: []interface{}{}, secretName: "anything", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasEnvFromSecret(tt.envFrom, tt.secretName)
			if got != tt.want {
				t.Errorf("hasEnvFromSecret(%q) = %v, want %v", tt.secretName, got, tt.want)
			}
		})
	}
}

func TestIsInOldRefs(t *testing.T) {
	oldRefs := []string{"secret1", "other-ns/secret2"}

	tests := []struct {
		name     string
		secretNm string
		want     bool
	}{
		{name: "direct match", secretNm: "secret1", want: true},
		{name: "namespaced match", secretNm: "secret2", want: true},
		{name: "no match", secretNm: "secret3", want: false},
		{name: "empty name", secretNm: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isInOldRefs(tt.secretNm, oldRefs, "default")
			if got != tt.want {
				t.Errorf("isInOldRefs(%q) = %v, want %v", tt.secretNm, got, tt.want)
			}
		})
	}
}

func TestFilterStaleEnvFrom(t *testing.T) {
	t.Run("removes stale entries", func(t *testing.T) {
		envFrom := []interface{}{
			map[string]interface{}{"secretRef": map[string]interface{}{"name": "old-secret"}},
			map[string]interface{}{"secretRef": map[string]interface{}{"name": "keep-secret"}},
			map[string]interface{}{"configMapRef": map[string]interface{}{"name": "config"}},
		}
		oldRefs := []string{"old-secret", "keep-secret"}
		newSet := map[string]bool{"keep-secret": true}

		result := filterStaleEnvFrom(envFrom, oldRefs, newSet, "default")

		if len(result) != 2 {
			t.Errorf("expected 2 entries, got %d", len(result))
		}
	})

	t.Run("preserves non-connection entries", func(t *testing.T) {
		envFrom := []interface{}{
			map[string]interface{}{"configMapRef": map[string]interface{}{"name": "config"}},
			"invalid-entry",
		}

		result := filterStaleEnvFrom(envFrom, nil, nil, "default")

		if len(result) != 2 {
			t.Errorf("expected 2 entries preserved, got %d", len(result))
		}
	})
}
