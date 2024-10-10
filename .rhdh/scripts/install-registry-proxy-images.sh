#!/bin/bash

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
USE_QUAY="false"
while [[ "$#" -gt 0 ]]; do
    case $1 in
        '--install-operator') TO_INSTALL="$2"; shift ;;
        '--next') UPSTREAM_IIB="quay.io/rhdh/iib:next-${OCP_VER}-${OCP_ARCH}" ;;
        '--latest') UPSTREAM_IIB="quay.io/rhdh/iib:latest-${OCP_VER}-${OCP_ARCH}" ;;
        '-y'|'--quay') USE_QUAY="true";;
        '-h'|'--help') usage; exit 0 ;;
        *) errorf "[ERROR] Unknown parameter: $1"; usage; exit 1 ;;
    esac
    shift
done

infof "[INFO] Using IIB image: ${UPSTREAM_IIB}"

# Call prepare-restricted-environment.sh to preload images
infof "[INFO] Preloading images into the internal registry..."
"${PWD}/prepare-restricted-environment.sh" \
    --prod_operator_index "${UPSTREAM_IIB}" \
    --prod_operator_version "${OCP_VER}"

if [ $? -ne 0 ]; then
    errorf "[ERROR] Failed to preload images using prepare-restricted-environment.sh"
    exit 1
fi

# Extract image details for further processing
infof "[INFO] Extracting bundle image information from IIB..."
catalogJson="/tmp/catalog.json"

# Using skopeo to inspect the image and extract the manifest details
skopeo inspect "docker://${UPSTREAM_IIB}" --config > "${catalogJson}"
if [[ ! -f "${catalogJson}" ]]; then
    errorf "[ERROR] Could not fetch catalog JSON for ${UPSTREAM_IIB}"
    exit 1
fi

# Extract image with SHA from catalog.json
bundle=$(jq -r '.config.Labels."operators.operatorframework.io.bundle.manifests"' < "${catalogJson}")
if [[ -z "${bundle}" ]]; then
    errorf "[ERROR] Could not extract bundle from ${UPSTREAM_IIB}"
    exit 1
fi
infof "[INFO] Bundle Version: ${bundle}"

imageWithSHA=$(jq -r '.config.Labels."operators.operatorframework.io.bundle.mediatype"' < "${catalogJson}")
if [[ -z "${imageWithSHA}" ]]; then
    errorf "[ERROR] Could not extract image with SHA from catalog JSON"
    exit 1
fi
infof "[INFO] Bundle Image SHA: ${imageWithSHA}"

# Logic from getTagForSHA.sh to resolve the image tag
resolve_image_tag() {
    local imageAndSHA="$1"
    local result=""
    
    if [[ $USE_QUAY == "true" ]]; then
        result=$(skopeo inspect "docker://quay.io/${imageAndSHA}" 2>/dev/null | jq -r '.Labels.version+"-"+.Labels.release')
    else
        result=$(skopeo inspect "docker://${imageAndSHA}" 2>/dev/null | jq -r '.Labels.version+"-"+.Labels.release')
    fi

    if [[ -n "$result" ]]; then
        local container=${result}
        container=$(echo "$container" | sed -r -e "s@:[0-9.]+:@:@")
        echo "$container"
    else
        errorf "[ERROR] Image with SHA not found: ${imageAndSHA}"
        exit 1
    fi
}

# Resolve the bundle image tag
bundleContainer=$(resolve_image_tag "${imageWithSHA}")
infof "[INFO] Resolved Bundle Image Tag: ${bundleContainer}"

# Call install-rhdh-catalog-source.sh to create the catalog source and install the operator
infof "[INFO] Installing CatalogSource and Operator..."
"${PWD}/install-rhdh-catalog-source.sh" --install-operator "${TO_INSTALL}" --latest

if [ $? -ne 0 ]; then
    errorf "[ERROR] Failed to install the operator."
    exit 1
fi

infof "[INFO] Installation completed successfully!"
