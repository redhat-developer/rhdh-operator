# Unified Dockerfile for hermetic builds (Hermeto upstream, Cachi2/Konflux downstream)
# and standard non-hermetic builds (make image-build)

#@follow_tag(registry.redhat.io/rhel9/go-toolset:latest)
# https://registry.access.redhat.com/ubi9/go-toolset
FROM registry.access.redhat.com/ubi9/go-toolset:9.8-1787080706@sha256:71e89a1a51ab32cc30634d89ee4dc8ea40ad9991057fa1eae3b1af32bc7db73f AS builder
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
#@follow_tag(registry.redhat.io/ubi9/ubi:latest)
# https://registry.access.redhat.com/ubi9/ubi
FROM registry.access.redhat.com/ubi9/ubi:9.8-1786985871@sha256:5426a8f45e80a07168a30ea24d84f266094b3756624a5508cc53927e6ee39e09 AS rpm-builder
RUN mkdir -p /mnt/rootfs
RUN dnf install --installroot /mnt/rootfs \
    openssl \
    --releasever 9 --setopt=install_weak_deps=0 --nogpgcheck --nodocs -y && \
    dnf --installroot /mnt/rootfs clean all && \
    rm -rf /mnt/rootfs/var/cache/* /mnt/rootfs/var/log/* /mnt/rootfs/tmp/*
RUN echo "backstage:x:1001:0:backstage user:/:/sbin/nologin" >> /mnt/rootfs/etc/passwd

# Final minimal image using UBI micro
#@follow_tag(registry.redhat.io/ubi9/ubi-micro:latest)
# https://registry.access.redhat.com/ubi9/ubi-micro
FROM registry.access.redhat.com/ubi9/ubi-micro:9.8-1786321990@sha256:7e7f79ab747bf2b452e3043dd89f388e92be4c7fdcc8b815b58adf6c99c39c95

COPY --from=rpm-builder /mnt/rootfs /

# RHIDP-4220 - make Konflux preflight and EC checks happy - [check-container] Create a directory named /licenses and include all relevant licensing
COPY LICENSE /licenses/

# Copy manager binary
COPY --from=builder /opt/app-root/src/manager .

USER 1001

WORKDIR /

ENTRYPOINT ["/manager"]
