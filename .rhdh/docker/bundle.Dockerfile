# THIS IS USED BY Konflux builds >= 1.4
# RHIDP-4220 - make Konflux preflight and EC checks happy - need some layer with RPMs even if not doing any pre-processing work
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest as builder-runner

RUN microdnf install -y skopeo python3.11 python3.11-pip npm git jq rsync findutils && \
alternatives --install /usr/bin/python python /usr/bin/python3.11 1 && \
alternatives --install /usr/bin/pip pip /usr/bin/pip3.11 1 && \
pip install --upgrade pip && pip install yq

# Use a new stage to enable caching of the package installations for local development
FROM builder-runner as builder

COPY /build /sync /midstream/build
COPY /sync /midstream/sync
COPY /upstream_repos.yml /midstream/
COPY distgit/containers/rhdh-operator-bundle /midstream/distgit/containers/rhdh-operator-bundle

WORKDIR /midstream/

RUN git init; bash -x ./build/ci/update-bundle.sh --container-nudge

FROM scratch

# RHIDP-4220 - make Konflux preflight and EC checks happy - [check-container] Create a directory named /licenses and include all relevant licensing
COPY --from=builder /midstream/distgit/containers/rhdh-operator-bundle/manifests /manifests/
COPY --from=builder /midstream/distgit/containers/rhdh-operator-bundle/metadata /metadata/
COPY --from=builder /midstream/distgit/containers/rhdh-operator-bundle/licenses /licenses/

# append Brew metadata here
