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

package kueue

import (
	"testing"
)

func TestKueueConstants(t *testing.T) {
	if KueueQueueNameLabel != "kueue.x-k8s.io/queue-name" {
		t.Errorf("unexpected KueueQueueNameLabel: %s", KueueQueueNameLabel)
	}

	if KueueManagedLabelKey != "kueue.x-k8s.io/managed" {
		t.Errorf("unexpected KueueManagedLabelKey: %s", KueueManagedLabelKey)
	}
}

func TestErrorMessages(t *testing.T) {
	if errMissingQueueLabel.Error() != `missing required label "kueue.x-k8s.io/queue-name"` {
		t.Errorf("unexpected errMissingQueueLabel: %s", errMissingQueueLabel)
	}

	if errEmptyQueueLabel.Error() != `label "kueue.x-k8s.io/queue-name" is set but empty` {
		t.Errorf("unexpected errEmptyQueueLabel: %s", errEmptyQueueLabel)
	}
}
