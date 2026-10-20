# Build stage
FROM --platform=$BUILDPLATFORM registry.access.redhat.com/ubi10/go-toolset:1.26.7-1792425924@sha256:ba35f850304db898e9ffa5c602e6e0bde6ee859d986a3e2808dbc2cf4d2e4ab3 AS builder

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
FROM --platform=$BUILDPLATFORM registry.access.redhat.com/ubi10/ubi:10.2-1792456870@sha256:4debe04be3b15ff2861e3428cbfa42379a183b16787ca2b8cf124d7afd6e84a5 AS certs

RUN dnf install ca-certificates --releasever 10 --setopt=install_weak_deps=0 --nogpgcheck --nodocs -y && \
    dnf clean all && \
    rm -rf /var/cache/* /var/log/*

# Runtime stage - minimal image
FROM registry.access.redhat.com/ubi10/ubi-micro:10.2-1792455530@sha256:1c105e547de5d9e0bc20d06ebabee73b20bc6bba994c50ae3e207684fd8bef2b

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
