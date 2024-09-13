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

# Script to preload images and install RHDH operator on an air-gapped OpenShift cluster without using ICSP.
# The script ensures all necessary images are preloaded into the internal registry.

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'

errorf() {
    echo -e "${RED}$1${NC}"
}

infof() {
    echo -e "${GREEN}$1${NC}"
}

usage() {
    echo "
    Usage:
      $0 [OPTIONS]

    Options:
      --latest                     : Install from the latest IIB [default]
      --next                       : Install from the next IIB
      --install-operator <NAME>     : Install operator named <NAME> after creating CatalogSource

    Examples:
      $0 --install-operator rhdh          # Install latest operator version
      $0 --next --install-operator rhdh   # Install next version (from IIB 'next' tag)
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
while [[ "$#" -gt 0 ]]; do
    case $1 in
        '--install-operator') TO_INSTALL="$2"; shift ;;
        '--next') UPSTREAM_IIB="quay.io/rhdh/iib:next-${OCP_VER}-${OCP_ARCH}" ;;
        '--latest') UPSTREAM_IIB="quay.io/rhdh/iib:latest-${OCP_VER}-${OCP_ARCH}" ;;
        '-h'|'--help') usage; exit 0 ;;
        *) errorf "[ERROR] Unknown parameter: $1"; usage; exit 1 ;;
    esac
    shift
done

infof "[INFO] Using IIB image: ${UPSTREAM_IIB}"

# Call prepare-restricted-environment.sh to preload images
infof "[INFO] Preloading images into the internal registry..."
"${PWD}/prepare-restricted-environment.sh" --prod_operator_index "${UPSTREAM_IIB}" --prod_operator_version "${OCP_VER}"

# Ensure all required images are in the internal registry
# Optionally, enhance this part of the script to resolve missing images and copy them from quay.io

# Example logic for resolving missing images using skopeo (if needed)
# This is assuming you want to add this functionality within your script
resolve_missing_images() {
    local image="$1"
    local internal_image="$2"

    # Try to resolve the image from quay.io if not found in the internal registry
    if ! skopeo inspect docker://"${image}" &>/dev/null; then
        infof "[INFO] Attempting to resolve ${image} from quay.io..."
        quay_image="${image/registry.redhat.io/quay.io}"

        if skopeo inspect docker://"${quay_image}" &>/dev/null; then
            infof "[INFO] Resolved ${image} to ${quay_image}"
            skopeo copy --authfile "${TMPDIR}/pull-secret.json" --dest-tls-verify=false --all docker://"${quay_image}" docker://"${internal_image}"
        else
            errorf "[ERROR] Could not resolve image: ${image}"
        fi
    fi
}

# Install the CatalogSource and Operator Subscription
infof "[INFO] Installing CatalogSource and Operator..."
"${PWD}/install-rhdh-catalog-source.sh" --install-operator "${TO_INSTALL}" --latest
infof "[INFO] Installation completed successfully!"
