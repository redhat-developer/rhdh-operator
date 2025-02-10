# THIS IS USED BY Konflux builds >= 1.4

# RHIDP-4220 - make Konflux preflight and EC checks happy - need some layer with RPMs even if not doing any pre-processing work
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest as builder-runner
USER 1001

FROM scratch

# Copy files to locations specified by labels.
COPY manifests /manifests/
COPY metadata /metadata/

# RHIDP-4220 - make Konflux preflight and EC checks happy - [check-container] Create a directory named /licenses and include all relevant licensing
COPY licenses /licenses/

# append Brew metadata here
