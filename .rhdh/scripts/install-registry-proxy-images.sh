#!/bin/bash
#
#  Copyright (c) 2024 Red Hat, Inc.
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.

# Ensure you have skopeo, podman, jq, yq, opm, and oc installed and configured.
# This script assumes access to a connected environment for pulling images from quay.io.

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'

NAMESPACE_CATALOGSOURCE="openshift-marketplace"
NAMESPACE_SUBSCRIPTION="rhdh-operator"
OLM_CHANNEL="fast"
OCP_VER=""
OCP_ARCH=""

errorf() {
    echo -e "${RED}$1${NC}"
}

infof() {
    echo -e "${GREEN}$1${NC}"
}

usage() {
    echo "
    Usage:
      $0 [OPTIONS] [IMAGES]

    Options:
      --latest                     : Install from iib quay.io/rhdh/iib:latest-\$OCP_VER-\$OCP_ARCH [default]
      --next                       : Install from iib quay.io/rhdh/iib:next-\$OCP_VER-\$OCP_ARCH
      --install-operator <NAME>     : Install operator named \$NAME after creating CatalogSource
      IMAGES                        : List of image references (optional)

    Example:
      $0 --install-operator rhdh quay.io/rhdh/rhdh-hub-rhel9:1.4 quay.io/rhdh/rhdh-operator-bundle:1.3
    "
}

# Ensure the minimum required tools are available
for cmd in oc jq skopeo; do
    if ! command -v $cmd &>/dev/null; then
        errorf "Please install $cmd"
        exit 1
    fi
done

# Ensure logged into an OpenShift cluster
if ! oc whoami &>/dev/null; then
    errorf "Not logged into an OpenShift cluster"
    exit 1
fi

# Get OCP version and architecture
OCP_VER="v$(oc version -o json | jq -r '.openshiftVersion' | sed -r -e 's#([0-9]+\.[0-9]+)\..+#\1#')"
OCP_ARCH="$(oc version -o json | jq -r '.serverVersion.platform' | sed -r -e 's#linux/##')"
[[ $OCP_ARCH == "amd64" ]] && OCP_ARCH="x86_64"

# Default IIB source
UPSTREAM_IIB="quay.io/rhdh/iib:latest-${OCP_VER}-${OCP_ARCH}"

# Parse arguments
TO_INSTALL=""
IMAGES=()
while [[ "$#" -gt 0 ]]; do
    case $1 in
        '--install-operator') TO_INSTALL="$2"; shift ;;
        '--next') UPSTREAM_IIB="quay.io/rhdh/iib:next-${OCP_VER}-${OCP_ARCH}" ;;
        '--latest') UPSTREAM_IIB="quay.io/rhdh/iib:latest-${OCP_VER}-${OCP_ARCH}" ;;
        '-h'|'--help') usage; exit 0 ;;
        *) IMAGES+=("$1") ;;
    esac
    shift
done

# If no images were provided, default to IIB
if [ ${#IMAGES[@]} -eq 0 ]; then
    IMAGES+=("$UPSTREAM_IIB")
fi

# Enable and get the internal registry route dynamically
infof "[INFO] Enabling default route for internal registry if not already enabled..."
oc patch configs.imageregistry.operator.openshift.io/cluster --patch '{"spec":{"defaultRoute":true}}' --type=merge

infof "[INFO] Retrieving internal registry route..."
INTERNAL_REGISTRY=$(oc get route default-route -n openshift-image-registry --template='{{ .spec.host }}')

if [[ -z "$INTERNAL_REGISTRY" ]]; then
    errorf "[ERROR] Unable to retrieve internal registry route."
    exit 1
fi

infof "[INFO] Using internal registry: ${INTERNAL_REGISTRY}"

# Authenticate using the external route for image registry
infof "[INFO] Authenticating to the external registry route..."
REGISTRY_URL="${INTERNAL_REGISTRY}"  # Remove the https:// part
oc registry login --registry=${REGISTRY_URL} --to=/tmp/pull-secret.json

# Pull and push images to the internal registry
for IMAGE in "${IMAGES[@]}"; do
    # Convert to quay.io equivalent if needed
    QUAY_IMAGE="${IMAGE/registry.redhat.io/quay.io}"

    infof "[INFO] Checking if image ${IMAGE} exists..."
    if ! skopeo inspect docker://"${IMAGE}" &>/dev/null; then
        infof "[INFO] Image ${IMAGE} not found in registry.redhat.io. Checking quay.io..."

        # Try quay.io if not found in registry.redhat.io
        if ! skopeo inspect docker://"${QUAY_IMAGE}" &>/dev/null; then
            errorf "[ERROR] Image not found in quay.io or registry.redhat.io: ${QUAY_IMAGE}"
            exit 2
        else
            IMAGE="${QUAY_IMAGE}"
        fi
    fi

    # Pull the image locally
    infof "[INFO] Pulling image: ${IMAGE}"
    skopeo copy docker://"${IMAGE}" dir:/tmp/"${IMAGE##*/}"

    # Push to the airgapped cluster using registry-proxy
    LOCAL_IMAGE="${INTERNAL_REGISTRY}/rhdh-operator/$(basename ${IMAGE})"  # Add your namespace here
    infof "[INFO] Pushing image to cluster: ${LOCAL_IMAGE}"
    skopeo copy --authfile /tmp/pull-secret.json dir:/tmp/"${IMAGE##*/}" docker://${LOCAL_IMAGE}
done

# Clean up only the temporary directories you created
for IMAGE in "${IMAGES[@]}"; do
    rm -rf /tmp/"${IMAGE##*/}"
done

# Create a temporary directory for YAMLs
TMPDIR=$(mktemp -d)
trap 'rm -fr "$TMPDIR"' EXIT

# Set up CatalogSource for the IIB image
infof "[INFO] Creating CatalogSource..."
cat <<EOF > "$TMPDIR/catalogsource.yml"
apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: rhdh-catalogsource
  namespace: ${NAMESPACE_CATALOGSOURCE}
spec:
  sourceType: grpc
  image: ${UPSTREAM_IIB}
  publisher: IIB testing Red Hat Developer Hub
  displayName: IIB testing catalog Red Hat Developer Hub
EOF

oc apply -f "$TMPDIR/catalogsource.yml"

# Optional: Install the operator if specified
if [[ -n "$TO_INSTALL" ]]; then
    infof "[INFO] Installing Operator: ${TO_INSTALL}"

    # Create OperatorGroup
    cat <<EOF > "$TMPDIR/operatorgroup.yml"
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: ${TO_INSTALL}-operatorgroup
  namespace: ${NAMESPACE_SUBSCRIPTION}
EOF
    oc apply -f "$TMPDIR/operatorgroup.yml"

    # Create Subscription
    cat <<EOF > "$TMPDIR/subscription.yml"
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: ${TO_INSTALL}
  namespace: ${NAMESPACE_SUBSCRIPTION}
spec:
  channel: ${OLM_CHANNEL}
  name: ${TO_INSTALL}
  source: rhdh-catalogsource
  sourceNamespace: ${NAMESPACE_CATALOGSOURCE}
  installPlanApproval: Automatic
EOF
    oc apply -f "$TMPDIR/subscription.yml"
fi

infof "[INFO] Completed successfully!"
