# Build stage
FROM --platform=$BUILDPLATFORM registry.access.redhat.com/ubi10/go-toolset:1.26.7-1791911662@sha256:4240036fd19bdad5d72ad15062d80b71cac9bc6b7789a45e53d6cc3e43167165 AS builder

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
FROM --platform=$BUILDPLATFORM registry.access.redhat.com/ubi10/ubi:10.2-1791893534@sha256:a75ad754aa048d808dd3260243452f8c9a665831d0493b4c9d92f0c88fe2f8c3 AS certs

RUN dnf install ca-certificates --releasever 10 --setopt=install_weak_deps=0 --nogpgcheck --nodocs -y && \
    dnf clean all && \
    rm -rf /var/cache/* /var/log/*

# Runtime stage - minimal image
FROM registry.access.redhat.com/ubi10/ubi-micro:10.2-1791898871@sha256:09d42b63a4ef2a23543502bd09e892c495a4ca480d481e28f00b3070d62d2d24

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
