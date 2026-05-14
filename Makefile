
# Image URL to use all building/pushing image targets
IMG ?= quay.io/opendatahub/workbenches-operator:dev
# Container engine to use for building and pushing images (podman or docker)
CONTAINER_ENGINE ?= podman
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.32.0

# Get the currently used golang install path (in GOBIN, currentl directory or GOPATH/bin)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

# Use existing local.mk for dev overrides
-include local.mk

.PHONY: all
all: build

##@ General

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests-fetch
manifests-fetch: ## Fetch upstream component manifests into opt/manifests/ for local development.
	bash get_all_manifests.sh

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases output:rbac:artifacts:config=config/rbac

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter.
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes.
	$(GOLANGCI_LINT) run --fix

##@ Testing

.PHONY: test
test: manifests generate fmt vet envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" \
		go test $$(go list ./... | grep -v /e2e) -coverprofile cover.out

.PHONY: unit-test
unit-test: manifests generate envtest ## Run unit tests (no fmt/vet check).
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" \
		go test $$(go list ./... | grep -v /e2e) -coverprofile cover.out

.PHONY: test-e2e
test-e2e: ## Run end-to-end tests against the cluster specified in ~/.kube/config.
	go test ./tests/e2e/ -v -timeout 30m

.PHONY: test-upgrade
test-upgrade: ## Run upgrade and migration tests against the cluster specified in ~/.kube/config.
	go test ./tests/upgrade/ -v -timeout 30m

.PHONY: test-handler
test-handler: ## Run ModuleHandler reference implementation tests.
	go test ./contrib/odh-operator/... -v -count=1

.PHONY: test-coverage
test-coverage: test ## Generate HTML coverage report.
	go tool cover -html=cover.out -o coverage.html
	@echo "Coverage report written to coverage.html"

##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	go build -o bin/manager cmd/main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./cmd/main.go

# Operator image — the main deliverable.
# Contains the compiled manager binary and operand manifests.
# Deploy this image to run the workbenches-operator on a cluster.
#   make image-build IMG=quay.io/myorg/workbenches-operator:v1.0.0
#   make image-build-push IMG=quay.io/myorg/workbenches-operator:v1.0.0

.PHONY: image-build
image-build: ## Build the operator container image.
	$(CONTAINER_ENGINE) build -t $(IMG) .

.PHONY: image-push
image-push: ## Push the operator container image to a registry.
	$(CONTAINER_ENGINE) push $(IMG)

.PHONY: image-build-push
image-build-push: image-build image-push ## Build and push the operator container image.

PLATFORMS ?= linux/amd64,linux/arm64
.PHONY: image-buildx
image-buildx: ## Build and push the operator image for multiple architectures.
	$(CONTAINER_ENGINE) buildx build --push --platform=$(PLATFORMS) --tag $(IMG) .

# OLM (Operator Lifecycle Manager) bundle image.
# Packages the CSV, CRD, RBAC, and webhook manifests for OLM-based installation
# (e.g. OperatorHub, OLM catalogs, disconnected environments).
# The bundle image references the operator image — build the operator image first.
#   1. make bundle IMG=quay.io/myorg/workbenches-operator:v1.0.0
#   2. make bundle-build
#   3. make bundle-push
##@ Bundle

VERSION ?= 0.1.0
IMG_REPO ?= $(firstword $(subst :, ,$(IMG)))
BUNDLE_IMG ?= $(IMG_REPO)-bundle:v$(VERSION)

.PHONY: bundle
bundle: manifests kustomize ## Generate OLM bundle manifests from the current kustomize output.
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
	$(KUSTOMIZE) build config/crd > bundle/manifests/workbenches-operator-crd.yaml
	@echo "Bundle generated in bundle/"

.PHONY: bundle-build
bundle-build: ## Build the OLM bundle image (contains CSV, CRD, and metadata).
	$(CONTAINER_ENGINE) build -f bundle.Dockerfile -t $(BUNDLE_IMG) .

.PHONY: bundle-push
bundle-push: ## Push the OLM bundle image to a registry.
	$(CONTAINER_ENGINE) push $(BUNDLE_IMG)

##@ Catalog

CATALOG_IMG ?= $(IMG_REPO)-catalog:v$(VERSION)
CATALOG_DIR ?= catalog

.PHONY: catalog-render
catalog-render: opm ## Render a file-based catalog (FBC) from the bundle image.
	rm -rf $(CATALOG_DIR)
	mkdir -p $(CATALOG_DIR)
	$(OPM) render $(BUNDLE_IMG) -o yaml > $(CATALOG_DIR)/operator.yaml
	@printf '%s\n' \
		"---" \
		"schema: olm.package" \
		"name: workbenches-operator" \
		"defaultChannel: alpha" \
		"---" \
		"schema: olm.channel" \
		"package: workbenches-operator" \
		"name: alpha" \
		"entries:" \
		"  - name: workbenches-operator.v$(VERSION)" \
		>> $(CATALOG_DIR)/operator.yaml
	$(OPM) validate $(CATALOG_DIR)
	@echo "File-based catalog generated in $(CATALOG_DIR)/"

.PHONY: catalog-build
catalog-build: catalog-render ## Build an OLM catalog image from the file-based catalog.
	$(CONTAINER_ENGINE) build -t $(CATALOG_IMG) -f catalog.Dockerfile .

.PHONY: catalog-push
catalog-push: ## Push the OLM catalog image to a registry.
	$(CONTAINER_ENGINE) push $(CATALOG_IMG)

.PHONY: catalog-build-push
catalog-build-push: catalog-build catalog-push ## Build and push the OLM catalog image.

.PHONY: olm-deploy
olm-deploy: image-build-push bundle bundle-build bundle-push catalog-build-push ## Build and push operator, bundle, and catalog images for OLM deployment.

CATALOG_NAMESPACE ?= openshift-marketplace
CATALOG_SOURCE_NAME ?= workbenches-operator

.PHONY: catalog-source-apply
catalog-source-apply: ## Create or update a CatalogSource on the cluster for the catalog image.
	@printf '%s\n' \
		"apiVersion: operators.coreos.com/v1alpha1" \
		"kind: CatalogSource" \
		"metadata:" \
		"  name: $(CATALOG_SOURCE_NAME)" \
		"  namespace: $(CATALOG_NAMESPACE)" \
		"spec:" \
		"  sourceType: grpc" \
		"  image: $(CATALOG_IMG)" \
		"  displayName: Workbenches Operator" \
		"  publisher: Red Hat" \
		"  updateStrategy:" \
		"    registryPoll:" \
		"      interval: 10m" \
		| kubectl apply -f -
	@echo "CatalogSource $(CATALOG_SOURCE_NAME) applied in $(CATALOG_NAMESPACE)"

.PHONY: catalog-source-delete
catalog-source-delete: ## Delete the CatalogSource from the cluster.
	kubectl delete catalogsource $(CATALOG_SOURCE_NAME) -n $(CATALOG_NAMESPACE) --ignore-not-found
	@echo "CatalogSource $(CATALOG_SOURCE_NAME) deleted from $(CATALOG_NAMESPACE)"

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
	$(KUSTOMIZE) build config/default | kubectl apply -f -

.PHONY: undeploy
undeploy: kustomize ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/default | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

##@ Dependencies

LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUBECTL ?= kubectl
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
GOLANGCI_LINT ?= $(LOCALBIN)/golangci-lint
OPM ?= $(LOCALBIN)/opm

## Tool Versions
KUSTOMIZE_VERSION ?= v5.6.0
CONTROLLER_TOOLS_VERSION ?= v0.18.0
ENVTEST_VERSION ?= release-0.23
GOLANGCI_LINT_VERSION ?= v2.5.0
OPM_VERSION ?= v1.66.0

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

.PHONY: opm
opm: $(OPM) ## Download opm locally if necessary.
$(OPM): $(LOCALBIN)
	@[ -f "$(OPM)-$(OPM_VERSION)" ] || { \
		set -e; \
		echo "Downloading opm $(OPM_VERSION)"; \
		OS=$(shell go env GOOS) && ARCH=$(shell go env GOARCH) && \
		curl -sSLo $(OPM) https://github.com/operator-framework/operator-registry/releases/download/$(OPM_VERSION)/$${OS}-$${ARCH}-opm && \
		chmod +x $(OPM) && \
		mv $(OPM) $(OPM)-$(OPM_VERSION); \
	}
	@ln -sf $(OPM)-$(OPM_VERSION) $(OPM)

# go-install-tool will 'go install' any package with custom target and target directory.
# $1 - target path, $2 - package, $3 - version
define go-install-tool
@[ -f "$(1)-$(3)" ] || { \
	set -e; \
	package=$(2)@$(3) ;\
	echo "Downloading $${package}" ;\
	rm -f $(1) || true ;\
	GOBIN=$(LOCALBIN) go install $${package} ;\
	mv $(1) $(1)-$(3) ;\
}
@ln -sf $(1)-$(3) $(1)
endef
