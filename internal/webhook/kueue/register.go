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
	ctrl "sigs.k8s.io/controller-runtime"
)

// RegisterWebhooks registers the Kueue webhook. Currently disabled (no-op).
func RegisterWebhooks(_ ctrl.Manager) error {
	// Kueue webhook is disabled. Uncomment to re-enable:
	// return (&Validator{
	// 	Client:  mgr.GetAPIReader(),
	// 	Decoder: admission.NewDecoder(mgr.GetScheme()),
	// 	Name:    "kueue-validating",
	// }).SetupWithManager(mgr)
	return nil
}
