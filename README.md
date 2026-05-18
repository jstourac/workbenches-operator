# Workbenches Operator

A Kubernetes operator for managing workbench (notebook) infrastructure in [Open Data Hub](https://opendatahub.io/) and Red Hat OpenShift AI (RHOAI) platforms. It reconciles a cluster-scoped `Workbenches` custom resource, provisions the workbench namespace, renders upstream Kustomize manifests, and applies them via server-side apply.

## Overview

The workbenches-operator manages the lifecycle of:

- **Kubeflow Notebook Controller** — core notebook CRD and controller from the Kubeflow project
- **ODH Notebook Controller** — Open Data Hub extensions for notebook management
- **Notebook Images** — platform-specific ImageStream definitions for data science notebook environments

It also provides **mutating admission webhooks** for Kubeflow `Notebook` objects:

| Webhook | Path | Purpose |
|---------|------|---------|
| Connection injection | `/platform-connection-notebook` | Injects data connections (secrets) as `envFrom` entries |
| Hardware profile | `/mutate-hardware-profile` | Applies node selectors, tolerations, and resource limits from `HardwareProfile` CRs |

## Architecture

```
Workbenches CR (cluster-scoped, singleton "default")
        │
        ▼
WorkbenchesReconciler
        │
        ├─ Ensure workbench namespace exists
        ├─ Render Kustomize manifests from /opt/manifests
        ├─ Server-side apply with field manager
        └─ Monitor deployment readiness
```

The operator reads component manifests from a filesystem tree (default `/opt/manifests`), which is populated at image build time from upstream repositories.

## Custom Resource

**API Group:** `components.platform.opendatahub.io/v1alpha1`
**Kind:** `Workbenches`
**Scope:** Cluster

```yaml
apiVersion: components.platform.opendatahub.io/v1alpha1
kind: Workbenches
metadata:
  name: default
spec:
  managementState: Managed        # Managed | Removed
  workbenchNamespace: opendatahub # immutable after first set
  platform: OpenDataHub           # OpenDataHub | SelfManagedRhoai
  gatewayDomain: ""               # projected by orchestrator
  mlflowEnabled: false            # projected by orchestrator
```

Only a single instance named `default` is permitted (enforced via CEL validation).

### Status Conditions

| Condition | Description |
|-----------|-------------|
| `Ready` | Overall component readiness |
| `ProvisioningSucceeded` | Manifests were applied successfully |
| `DeploymentsAvailable` | All controller deployments have ready replicas |
| `Degraded` | Component is not operating normally |

## Prerequisites

- Go 1.25+
- Kubernetes 1.28+ / OpenShift 4.x cluster
- `kubectl` configured with cluster access
- `podman` or `docker` for container builds

## Getting Started

### Local Development

```bash
# Install CRDs into the cluster
make install

# Run the controller locally (webhooks disabled)
make run

# Create a sample Workbenches resource
kubectl apply -f config/samples/components_v1alpha1_workbenches.yaml
```

### Fetch Upstream Manifests

The operator requires component manifests at runtime. For local development:

```bash
# Download manifests from upstream repositories into opt/manifests/
make manifests-fetch
```

This runs `get_all_manifests.sh`, which clones from:

| Component | Source Repository |
|-----------|-------------------|
| `kf-notebook-controller` | `opendatahub-io/kubeflow` — `components/notebook-controller/config` |
| `odh-notebook-controller` | `opendatahub-io/kubeflow` — `components/odh-notebook-controller/config` |
| `notebooks` | `opendatahub-io/notebooks` — `manifests` |

Override sources with CLI flags:

```bash
./get_all_manifests.sh --workbenches/kf-notebook-controller=myorg:myrepo:mybranch:path/to/config
```

### Build and Deploy

```bash
# Build the operator binary
make build

# Build the container image
make image-build IMG=quay.io/myorg/workbenches-operator:latest

# Deploy to a cluster
make deploy IMG=quay.io/myorg/workbenches-operator:latest

# Remove from cluster
make undeploy
```

The Dockerfile is a multi-stage build that fetches manifests, compiles the Go binary (with FIPS support via `-tags strictfipsruntime`), and produces a minimal UBI 9 runtime image.

## Testing

```bash
# Unit and integration tests (envtest)
make test

# Unit tests only (skip fmt/vet)
make unit-test

# End-to-end tests (requires a running cluster)
make test-e2e

# Upgrade and migration tests
make test-upgrade

# ModuleHandler reference implementation tests
make test-handler

# Generate HTML coverage report
make test-coverage
```

The integration test suite uses `envtest` with Kubernetes 1.32.0 assets.

## Project Structure

```
cmd/main.go                    Operator entrypoint
api/v1alpha1/                  Workbenches CRD types
internal/
  controller/                  WorkbenchesReconciler and manifest rendering
  webhook/                     Admission webhooks (notebook, hardwareprofile, kueue)
  metadata/                    Labels and annotations constants
  platform/                    Platform-specific defaults (namespace, section titles)
  gvk/                         GroupVersionKind helpers
  upgrade/                     Annotation migration helpers
config/
  crd/                         Generated CRD manifests
  rbac/                        RBAC (ClusterRole, bindings, ServiceAccount)
  manager/                     Controller manager Deployment
  default/                     Default kustomize overlay
  webhook/                     MutatingWebhookConfiguration
  samples/                     Example Workbenches CR
charts/operator/               Helm chart for deployment
contrib/odh-operator/          Reference ModuleHandler for ODH operator integration
opt/manifests/                 Upstream operand manifests (fetched at build time)
bundle/                        OLM bundle (CSV, metadata)
tests/
  e2e/                         End-to-end Ginkgo tests
  upgrade/                     Upgrade and migration tests
hack/                          Code generation boilerplate
.github/workflows/             CI workflows (test, build, lint, e2e, upgrade)
```

## Makefile Targets

| Target | Description |
|--------|-------------|
| `make build` | Build the manager binary |
| `make run` | Run the controller locally |
| `make test` | Run unit/integration tests |
| `make test-e2e` | Run end-to-end tests |
| `make lint` | Run golangci-lint |
| `make manifests` | Generate CRD, RBAC, and webhook manifests |
| `make manifests-fetch` | Fetch upstream component manifests |
| `make generate` | Generate DeepCopy methods |
| `make install` | Install CRDs into cluster |
| `make deploy` | Deploy controller to cluster |
| `make image-build` | Build container image |
| `make bundle` | Generate OLM bundle |

A `local.mk` file (gitignored) can be used for personal overrides.

## Tool Versions

| Tool | Version |
|------|---------|
| Go | 1.25.0 |
| Kustomize | v5.6.0 |
| controller-gen | v0.18.0 |
| setup-envtest | release-0.23 |
| golangci-lint | v2.5.0 |

## License

Apache License 2.0 — see [LICENSE](LICENSE) for details.
