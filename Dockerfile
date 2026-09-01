# Unified Dockerfile for hermetic builds (Hermeto upstream, Cachi2/Konflux downstream)
# and standard non-hermetic builds (make image-build)

#@follow_tag(registry.redhat.io/rhel10/go-toolset:latest)
# https://registry.access.redhat.com/ubi10/go-toolset
FROM registry.access.redhat.com/ubi10/go-toolset:1.26.5-1786496329@sha256:1db86a2b0f77c1197b011de5140236effc27b1a1724c0105d4926857a0756de5 AS builder
ARG TARGETOS
ARG TARGETARCH
# hadolint ignore=DL3002
USER 0
ENV GOPATH=/go/

ENV EXTERNAL_SOURCE=.
ENV CONTAINER_SOURCE=/opt/app-root/src
WORKDIR $CONTAINER_SOURCE

# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
COPY $EXTERNAL_SOURCE/go.mod $CONTAINER_SOURCE/go.mod
COPY $EXTERNAL_SOURCE/go.sum $CONTAINER_SOURCE/go.sum
RUN go mod download

COPY $EXTERNAL_SOURCE $CONTAINER_SOURCE

# Build
# hadolint ignore=SC3010
# Build
# the GOARCH has not a default value to allow the binary be built according to the host where the command
# was called. For example, if we call make docker-build in a local env which has the Apple Silicon M1 SO
# the docker BUILDPLATFORM arg will be linux/arm64 when for Apple x86 it will be linux/amd64. Therefore,
# by leaving it empty we can ensure that the container and binary shipped on it will have the same platform.
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o manager cmd/main.go

# Install openssl for FIPS support into an isolated rootfs
#@follow_tag(registry.redhat.io/ubi10/ubi:latest)
# https://registry.access.redhat.com/ubi10/ubi
FROM registry.access.redhat.com/ubi10/ubi:10.2-1786928703@sha256:a3210c44455d3de518c9ebf53f391b31f5cb5e9b7f101a130ea2d87b17b32dc0 AS rpm-builder
RUN mkdir -p /mnt/rootfs
RUN dnf install --installroot /mnt/rootfs \
    openssl \
    --releasever 10 --setopt=install_weak_deps=0 --nogpgcheck --nodocs -y && \
    dnf --installroot /mnt/rootfs clean all && \
    rm -rf /mnt/rootfs/var/cache/* /mnt/rootfs/var/log/* /mnt/rootfs/tmp/*
RUN echo "backstage:x:1001:0:backstage user:/:/sbin/nologin" >> /mnt/rootfs/etc/passwd

# Final minimal image using UBI micro
#@follow_tag(registry.redhat.io/ubi10/ubi-micro:latest)
# https://registry.access.redhat.com/ubi10/ubi-micro
FROM registry.access.redhat.com/ubi10/ubi-micro:10.2-1786324819@sha256:cabedb588644e9da2c95ebb173a67b78d58aaedcb0eaa42a86f880bcef8a0b2f

COPY --from=rpm-builder /mnt/rootfs /

# RHIDP-4220 - make Konflux preflight and EC checks happy - [check-container] Create a directory named /licenses and include all relevant licensing
COPY LICENSE /licenses/

# Copy manager binary
COPY --from=builder /opt/app-root/src/manager /manager

USER 1001

WORKDIR /

ENTRYPOINT ["/manager"]
