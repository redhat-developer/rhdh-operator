# Build stage
FROM --platform=$BUILDPLATFORM registry.access.redhat.com/ubi10/go-toolset:1.27.3-1793903644@sha256:05f3da136a4dbe077f30606e2e53a4f5cab185989c98309823ea06a9a1c52a50 AS builder

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
FROM registry.access.redhat.com/ubi10/ubi:10.3-1794519911@sha256:73ea54bfbfb586075bd3d45df6483a7f68875233c8cc7cebcf9b2bb42306c308 AS certs

RUN mkdir -p /mnt/rootfs && \
    dnf install --installroot /mnt/rootfs ca-certificates \
        --releasever 9 --setopt=install_weak_deps=0 --nodocs -y && \
    dnf --installroot /mnt/rootfs clean all && \
    rm -rf /mnt/rootfs/var/cache/* /mnt/rootfs/var/log/* /mnt/rootfs/tmp/*

# Runtime stage - minimal image
FROM registry.access.redhat.com/ubi10/ubi-micro:10.3-1794483856@sha256:2ad0f98cb13160c4773c945ef93f88b25831c7f27cd3780d72a7e78b210ea72b

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
