FROM registry.access.redhat.com/ubi9/ubi-minimal:latest AS builder

RUN microdnf install -y findutils && microdnf clean all

COPY catalog/ /tmp/catalog/

FROM quay.io/operator-framework/opm:latest

COPY --from=builder /tmp/catalog/ /configs/

LABEL operators.operatorframework.io.index.configs.v1=/configs

ENTRYPOINT ["/bin/opm"]
CMD ["serve", "/configs"]
