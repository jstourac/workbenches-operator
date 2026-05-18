# Agent Guidelines for workbenches-operator

This file provides context and conventions for AI coding agents working on this repository.

## Project Summary

This is a Kubernetes operator (built with Kubebuilder/controller-runtime) that manages workbench (notebook) infrastructure for Open Data Hub and Red Hat OpenShift AI. It reconciles a cluster-scoped singleton `Workbenches` CR and applies upstream Kustomize manifests via server-side apply.

## Key Architectural Decisions

- **Singleton CR**: Only one `Workbenches` resource is allowed, and it must be named `default`. This is enforced via CEL validation on the CRD.
- **Manifest rendering**: The operator reads Kustomize bundles from a filesystem path (`/opt/manifests` by default) and renders them at runtime using the krusty engine — it does not embed manifests in Go code.
- **Server-side apply**: All manifest application uses SSA with field manager `workbenches-operator`.
- **Platform awareness**: The operator supports two platforms (`OpenDataHub` and `SelfManagedRhoai`), selecting different manifest overlays and namespace defaults per platform.
- **Immutable fields**: `workbenchNamespace` is immutable after initial creation (CEL-enforced).

## Repository Layout

```
cmd/main.go                     Entrypoint — sets up manager, controller, webhooks
api/v1alpha1/                   CRD Go types (Workbenches)
internal/controller/            Reconciler + manifest rendering (manifests.go)
internal/webhook/               Admission webhooks (notebook connections, hardware profile)
internal/metadata/              Label/annotation constants used across packages
internal/platform/              Platform defaults (namespace, section title)
internal/gvk/                   GVK helpers (Notebook, HardwareProfile, ImageStream, etc.)
internal/upgrade/               Annotation migration helpers
config/                         Kustomize manifests (CRD, RBAC, manager, webhooks, samples)
charts/operator/                Helm chart
contrib/odh-operator/           Reference ModuleHandler (not linked into the binary)
opt/manifests/                  Upstream operand manifests (populated by get_all_manifests.sh)
tests/e2e/                      End-to-end Ginkgo tests
tests/upgrade/                  Upgrade/migration tests
bundle/                         OLM bundle
```

## Build and Test Commands

```bash
make build              # Build the manager binary
make test               # Run unit + integration tests (envtest, K8s 1.32.0)
make unit-test          # Unit tests without fmt/vet
make test-e2e           # E2E tests (requires cluster)
make test-upgrade       # Upgrade/migration tests (requires cluster)
make test-handler       # contrib/ handler tests
make lint               # golangci-lint
make manifests          # Regenerate CRD, RBAC, webhook YAML from Go markers
make generate           # Regenerate DeepCopy methods
make manifests-fetch    # Fetch upstream manifests into opt/manifests/
make image-build        # Build container image (podman by default)
```

## Code Conventions

### Go Style
- Go 1.25, standard library conventions.
- Kubebuilder RBAC/CRD/webhook markers in controller and type files — keep them up to date when changing permissions or fields.
- Run `make manifests` after modifying any kubebuilder markers.
- Run `make generate` after modifying CRD types to regenerate `zz_generated.deepcopy.go`.
- Error wrapping uses `fmt.Errorf("context: %w", err)`.
- Logging uses `controller-runtime`'s `log.FromContext(ctx)` — use `.V(1)` for debug-level.

### Testing
- Unit/integration tests use Ginkgo/Gomega with `envtest`.
- Test files live alongside source in the same package.
- `TestRenderRealManifests` in `manifests_test.go` requires `opt/manifests` to exist — run `make manifests-fetch` first.
- E2E tests expect a deployed operator (`workbenches-operator-controller-manager`).

### Labels and Annotations
- Defined in `internal/metadata/` — always use constants, never hardcode label strings.
- Key label: `app.opendatahub.io/workbenches=true` identifies operator-managed deployments.
- Namespace ownership: `opendatahub.io/generated-namespace=true`.

### Manifests
- Upstream manifests in `opt/manifests/` are fetched, not manually authored.
- Do not edit files under `opt/manifests/` directly — they are overwritten by `get_all_manifests.sh`.
- Kustomize params (`params.env`) are merged at render time from CR spec values.

## CRD

**GVK:** `components.platform.opendatahub.io/v1alpha1/Workbenches`

Key spec fields:
- `managementState`: `Managed` (default) or `Removed`
- `workbenchNamespace`: target namespace for notebooks (immutable)
- `platform`: `OpenDataHub` or `SelfManagedRhoai`
- `gatewayDomain`, `mlflowEnabled`: projected by orchestrator

Status conditions: `Ready`, `ProvisioningSucceeded`, `DeploymentsAvailable`, `Degraded`.

## Webhooks

Registered in `internal/webhook/webhook.go` via `RegisterAllWebhooks`:

1. **Connection injection** (`internal/webhook/notebook/`) — path `/platform-connection-notebook`
   - Reads `opendatahub.io/connections` annotation, validates secrets, injects `envFrom`
2. **Hardware profile** (`internal/webhook/hardwareprofile/`) — path `/mutate-hardware-profile`
   - Reads `opendatahub.io/hardware-profile-name` annotation, applies resources/tolerations/nodeSelector
3. **Kueue** (`internal/webhook/kueue/`) — currently disabled (no-op registration)

Webhooks are enabled by default (`--enable-webhooks=true`) but the default kustomize overlay (`config/default`) passes `--enable-webhooks=false`.

## CI

GitHub Actions workflows in `.github/workflows/`:
- `test.yml` — unit tests + manifest render test
- `build.yml` — binary and image build
- `lint.yml` — golangci-lint + kube-linter
- `e2e.yml` — Kind cluster end-to-end tests
- `upgrade-test.yml` — upgrade/migration scenarios
- `validate-related-images.yml` — checks for RELATED_IMAGE env vars

## Common Pitfalls

- Forgetting `make manifests` after changing kubebuilder markers leads to stale CRD/RBAC YAML.
- Forgetting `make generate` after changing CRD types leads to missing DeepCopy methods.
- Tests that use real manifests (`TestRenderRealManifests`) fail without `make manifests-fetch`.
- The `config/manager/kustomization.yaml` image reference may contain local overrides — always check before committing.
- `contrib/odh-operator/` is a reference implementation only — it is not compiled into the operator binary.
