FROM registry.access.redhat.com/ubi9/ubi-minimal:latest as builder-runner
USER 1001

FROM scratch

# Copy files to locations specified by labels.
COPY manifests /manifests/
COPY metadata /metadata/

# append Brew metadata here
