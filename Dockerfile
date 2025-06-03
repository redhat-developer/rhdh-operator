# THIS IS USED BY Konflux builds >= 1.4 with Cachi2 enabled

#@follow_tag(registry.redhat.io/rhel9/go-toolset:latest)
# https://registry.access.redhat.com/ubi9/go-toolset
FROM registry.access.redhat.com/ubi9/go-toolset:1.23@sha256:e0ad156b08e0b50ad509d79513e13e8a31f2812c66e9c48c98cea53420ec2bca AS builder
ARG TARGETOS
ARG TARGETARCH
# hadolint ignore=DL3002
USER 0
ENV GOPATH=/go/

# '(micro)dnf update -y' not allowed in Konflux+Cachi2: instead use renovate or https://github.com/konflux-ci/rpm-lockfile-prototype to update the rpms.lock.yaml file
# Downstream comment
RUN dnf -q -y update
#/ Downstream comment

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

# Install openssl for FIPS support
#@follow_tag(registry.redhat.io/ubi9/ubi-minimal:latest)
# https://registry.access.redhat.com/ubi9/ubi-minimal
FROM registry.access.redhat.com/ubi9-minimal:9.6@sha256:92b1d5747a93608b6adb64dfd54515c3c5a360802db4706765ff3d8470df6290 AS runtime

# Downstream uncomment
# RUN cat /cachi2/cachi2.env
#/ Downstream uncomment

# '(micro)dnf update -y' not allowed in Konflux+Cachi2: instead use renovate or https://github.com/konflux-ci/rpm-lockfile-prototype to update the rpms.lock.yaml file
# Downstream comment
RUN microdnf update --setopt=install_weak_deps=0 -y
#/ Downstream comment

RUN microdnf install -y openssl; microdnf clean -y all

ENV EXTERNAL_SOURCE=.
ENV CONTAINER_SOURCE=/opt/app-root/src

# RHIDP-4220 - make Konflux preflight and EC checks happy - [check-container] Create a directory named /licenses and include all relevant licensing
COPY $EXTERNAL_SOURCE/LICENSE /licenses/

ENV HOME=/ \
    USER_NAME=backstage \
    USER_UID=1001

RUN echo "${USER_NAME}:x:${USER_UID}:0:${USER_NAME} user:${HOME}:/sbin/nologin" >> /etc/passwd

# Copy manager binary
COPY --from=builder $CONTAINER_SOURCE/manager .

USER ${USER_UID}

WORKDIR ${HOME}

ENTRYPOINT ["/manager"]
