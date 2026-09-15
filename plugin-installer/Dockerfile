# Build stage
FROM --platform=$BUILDPLATFORM registry.access.redhat.com/ubi10/go-toolset:1.26.7-1788411200@sha256:be70aa468168f1ecd46e56d5f362e697243bcf9d3a2d98819597e43471a5d0e4 AS builder

ARG TARGETOS=linux
ARG TARGETARCH

WORKDIR /workspace

# Copy go mod files first for better layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY cmd/plugin-fetch/ cmd/plugin-fetch/
COPY pkg/ pkg/

# Build static binary for target platform
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-s -w" -o plugin-fetch ./cmd/plugin-fetch/

# CA certificates stage - install ca-certificates package the Red Hat way
FROM registry.access.redhat.com/ubi10/ubi:10.2-1788218897@sha256:4690398669a07627339936c9e79b05233053056ce688efeb4400d3c1c530486b AS certs

RUN mkdir -p /mnt/rootfs && \
    dnf install --installroot /mnt/rootfs ca-certificates \
        --releasever 9 --setopt=install_weak_deps=0 --nodocs -y && \
    dnf --installroot /mnt/rootfs clean all && \
    rm -rf /mnt/rootfs/var/cache/* /mnt/rootfs/var/log/* /mnt/rootfs/tmp/*

# Runtime stage - minimal image
FROM registry.access.redhat.com/ubi10/ubi-micro:10.2-1787684489@sha256:37fadb004c6bea628fcdd81376c8fb77bd8d9fd432d90503af4d9e76b1ff7191

WORKDIR /

# Copy CA certificates from certs stage
COPY --from=certs /mnt/rootfs/etc/pki /etc/pki

# Copy binary from builder
COPY --from=builder /workspace/plugin-fetch /usr/local/bin/plugin-fetch

# Default environment variables
ENV INPUT_FILE=/input/packages.txt \
    OUTPUT_DIR=/dynamic-plugins-root \
    PARALLEL=4

USER 1001

ENTRYPOINT ["/usr/local/bin/plugin-fetch"]
