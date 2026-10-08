# Build stage
FROM --platform=$BUILDPLATFORM registry.access.redhat.com/ubi10/go-toolset:1.26.7-1791362847@sha256:f6b33401d7dc17d32bed91be97ba14c642646031ef2e89630b035a822d1755bf AS builder

ARG TARGETOS=linux
ARG TARGETARCH

# Switch to root for hermetic builds (avoid permission issues)
USER 0

# Use the default go-toolset workdir (has proper permissions for hermetic builds)
WORKDIR /opt/app-root/src

# Copy go mod files first for better layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY cmd/plugin-fetch/ cmd/plugin-fetch/
COPY pkg/ pkg/

# Build static binary for target platform
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-s -w" -o plugin-fetch ./cmd/plugin-fetch/

# CA certificates stage - install ca-certificates package the Red Hat way
FROM --platform=$BUILDPLATFORM registry.access.redhat.com/ubi10/ubi:10.2-1791375475@sha256:3ef7bffd1dc7e55f8d34d5cc9737d188e3b9613e562942d2955e2fc867dd74d2 AS certs

RUN dnf install ca-certificates --setopt=install_weak_deps=0 --nogpgcheck --nodocs -y && \
    dnf clean all && \
    rm -rf /var/cache/* /var/log/*

# Runtime stage - minimal image
FROM registry.access.redhat.com/ubi10/ubi-micro:10.2-1787684489@sha256:37fadb004c6bea628fcdd81376c8fb77bd8d9fd432d90503af4d9e76b1ff7191

WORKDIR /

# Copy CA certificates from certs stage
COPY --from=certs /etc/pki /etc/pki

# Copy binary from builder
COPY --from=builder /opt/app-root/src/plugin-fetch /usr/local/bin/plugin-fetch

# Default environment variables
ENV INPUT_FILE=/input/packages.txt \
    OUTPUT_DIR=/dynamic-plugins-root \
    PARALLEL=4

USER 1001

ENTRYPOINT ["/usr/local/bin/plugin-fetch"]
