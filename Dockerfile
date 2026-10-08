# Unified Dockerfile for hermetic builds (Hermeto upstream, Cachi2/Konflux downstream)
# and standard non-hermetic builds (make image-build)

#@follow_tag(registry.redhat.io/rhel10/go-toolset:latest)
# https://registry.access.redhat.com/ubi10/go-toolset
FROM registry.access.redhat.com/ubi10/go-toolset:10.2-1791462472@sha256:7ec4dc8858bd3467b5e13ecccc9f913a0908978e50420958922d4494bf7b65b9 AS builder
ARG TARGETOS
ARG TARGETARCH
# Build as the go-toolset default user (non-root). Hermeto prefetch must chmod the
# /cachi2 cache (see hack/local-hermeto-build.sh and CI) so this user can use gomod offline.
ENV GOPATH=/go/
ENV GOCACHE=/tmp/go-build-cache

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
FROM registry.access.redhat.com/ubi10/ubi:10.2-1791444044@sha256:27e14f4987d7abe56664d7e1b1dddcd0226d4ca593da5726f0f65697a17d0fee AS rpm-builder
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
FROM registry.access.redhat.com/ubi10/ubi-micro:10.2-1791441953@sha256:5b13e670e107509be71c066180032017ff5dddff2b6cd60d16311a5cea309953

COPY --from=rpm-builder /mnt/rootfs /

# RHIDP-4220 - make Konflux preflight and EC checks happy - [check-container] Create a directory named /licenses and include all relevant licensing
COPY LICENSE /licenses/

# Copy manager binary
COPY --from=builder /opt/app-root/src/manager /manager

USER 1001

WORKDIR /

ENTRYPOINT ["/manager"]
