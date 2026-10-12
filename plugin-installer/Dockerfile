# Build stage
FROM --platform=$BUILDPLATFORM registry.access.redhat.com/ubi10/go-toolset:1.26.7-1791462472@sha256:7ec4dc8858bd3467b5e13ecccc9f913a0908978e50420958922d4494bf7b65b9 AS builder

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
FROM --platform=$BUILDPLATFORM registry.access.redhat.com/ubi10/ubi:10.2-1791784404@sha256:ad0d8e4fcd4c83cf4a2d025de402f010fd068e43f253d0a57b5681d1ad45f044 AS certs

RUN dnf install ca-certificates --releasever 10 --setopt=install_weak_deps=0 --nogpgcheck --nodocs -y && \
    dnf clean all && \
    rm -rf /var/cache/* /var/log/*

# Runtime stage - minimal image
FROM registry.access.redhat.com/ubi10/ubi-micro:10.2-1791441953@sha256:5b13e670e107509be71c066180032017ff5dddff2b6cd60d16311a5cea309953

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
