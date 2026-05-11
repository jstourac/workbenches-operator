## Stage 1: Build the manager binary
FROM registry.access.redhat.com/ubi9/go-toolset:1.25 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# Cache deps before building and copying source so that we don't need to re-download
# as much and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source and generated files
COPY cmd/main.go cmd/main.go
COPY api/ api/
COPY internal/ internal/
COPY hack/ hack/
COPY Makefile Makefile
COPY PROJECT PROJECT

# Generate deepcopy methods and build
USER root
RUN make generate && \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} \
    go build -tags strictfipsruntime -a -ldflags="-s -w" -o manager cmd/main.go

## Stage 2: Runtime
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
