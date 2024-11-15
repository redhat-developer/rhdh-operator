# THIS IS USED BY Konflux builds >= 1.4
# TODO verify this works with Cachi2 once we enable that, or switch to use ../.rhdh/docker/Dockerfile as input

#@follow_tag(registry.redhat.io/rhel9/go-toolset:latest)
# https://registry.access.redhat.com/ubi9/go-toolset
FROM registry.access.redhat.com/ubi9/go-toolset:9.5-1730550521@sha256:b23212b6a296e95cbed5dd415d4cd5567db9e8057aa08e291f1fddc087cea944 AS builder
ARG TARGETOS
ARG TARGETARCH
# hadolint ignore=DL3002
USER 0
ENV GOPATH=/go/
# update RPMs
RUN dnf -q -y update

# Upstream sources
# Downstream comment
ENV EXTERNAL_SOURCE=.
ENV CONTAINER_SOURCE=/opt/app-root/src
WORKDIR /workspace
#/ Downstream comment

# Downstream sources
# Downstream uncomment
# ENV EXTERNAL_SOURCE=$REMOTE_SOURCES/upstream1/app/distgit/containers/rhdh-operator
# ENV CONTAINER_SOURCE=$REMOTE_SOURCES_DIR
# WORKDIR $CONTAINER_SOURCE/
#/ Downstream uncomment

# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
# Downstream comment
COPY $EXTERNAL_SOURCE/go.mod ./go.mod
COPY $EXTERNAL_SOURCE/go.sum ./go.sum
RUN go mod download
#/ Downstream comment

# Downstream uncomment
# COPY $REMOTE_SOURCES/upstream1/cachito.env ./
# RUN source ./cachito.env && rm -f ./cachito.env && mkdir -p /workspace
#/ Downstream uncomment

COPY $EXTERNAL_SOURCE/api/ ./api/
COPY $EXTERNAL_SOURCE/cmd/ ./cmd/
COPY $EXTERNAL_SOURCE/config/ ./config/
COPY $EXTERNAL_SOURCE/internal/ ./internal/
COPY $EXTERNAL_SOURCE/pkg/ ./pkg/

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
FROM registry.access.redhat.com/ubi9-minimal:9.5-1730489338@sha256:6907fbacb294ab6ba988f8bcc6bd5127f589966e5808fcb454de3e104983ae5b AS runtime
RUN microdnf update --setopt=install_weak_deps=0 -y && microdnf install -y openssl; microdnf clean -y all

# RHIDP-4220 - make Konflux preflight and EC checks happy - [check-container] Create a directory named /licenses and include all relevant licensing
COPY $EXTERNAL_SOURCE/LICENSE /licenses/

# Upstream sources
# Downstream comment
ENV CONTAINER_SOURCE=/workspace
#/ Downstream comment

# Downstream sources
# Downstream uncomment
# ENV CONTAINER_SOURCE=$REMOTE_SOURCES_DIR
#/ Downstream uncomment

ENV HOME=/ \
    USER_NAME=backstage \
    USER_UID=1001

RUN echo "${USER_NAME}:x:${USER_UID}:0:${USER_NAME} user:${HOME}:/sbin/nologin" >> /etc/passwd

# Copy manager binary
COPY --from=builder $CONTAINER_SOURCE/manager .

USER ${USER_UID}

WORKDIR ${HOME}

ENTRYPOINT ["/manager"]

