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

# Variables
REMOTE_REGISTRY="quay.io/rhdh"
LOCAL_REGISTRY="image-registry.openshift-image-registry.svc:5000"
NAMESPACE="openshift-marketplace"
CATALOG_NAME="my-catalog-source"
IIB_IMAGE="registry-proxy.engineering.redhat.com/rh-osbs/rhdh-rhdh-hub-rhel9"
WORKDIR="/tmp/iib_extract"

mkdir -p "$WORKDIR"

# Step 1: Pull the IIB image and extract catalog information
echo "Pulling IIB image and extracting catalog information..."
skopeo copy --all "docker://$IIB_IMAGE" "oci:$WORKDIR/iib"

# Unpack the OCI image
cd "$WORKDIR"
echo "Listing contents of blobs/sha256/ for debugging..."gity
ls -R "$WORKDIR/iib/blobs/sha256/"

# Attempt to inspect index.json to locate manifests
echo "Inspecting index.json..."
indexJson="$WORKDIR/iib/index.json"
if [[ -f $indexJson ]]; then
    manifests=$(jq -r '.manifests[].digest' "$indexJson")
    echo "Found the following manifests: $manifests"
    
    # Check if we can find the catalog.json or similar file within the manifests
    for manifest in $manifests; do
        echo "Checking manifest: $manifest"
        manifestDir="$WORKDIR/iib/blobs/sha256/${manifest#*:}"
        
        # Find and extract any tar files associated with the manifest
        tar_files=$(find "$manifestDir" -type f -name "*.tar")
        for tar_file in $tar_files; do
            echo "Extracting $tar_file..."
            tar -xf "$tar_file" -C "$WORKDIR"
        done
        
        # Attempt to locate catalog.json or other relevant files
        potentialJson=$(find "$WORKDIR" -name "*.json" -print -quit)
        if [[ -n $potentialJson ]]; then
            echo "Found potential JSON file: $potentialJson"
            jq '.' "$potentialJson"
        fi
    done
else
    echo "[ERROR] Could not find index.json. Exiting."
    exit 1
fi

# Step 2: Extract image references from the unpacked catalog JSON
echo "Extracting image references from IIB..."

# Iterate over manifest digests in index.json
for manifest in $(jq -r '.manifests[].digest' "$WORKDIR/iib/index.json"); do
  # Construct the file path without the 'sha256:' prefix
  manifest_json="$WORKDIR/iib/blobs/sha256/${manifest#sha256:}"
  
  echo "Checking manifest: $manifest_json"
  
  # Check if the file exists before attempting to parse it
  if [[ -f "$manifest_json" ]]; then
    # Extract potential image references (adjust jq path based on manifest structure)
    image_references=$(jq -r '.. | objects | select(has("config") or has("layers")) | .config.digest // empty' "$manifest_json")
    
    if [[ -n $image_references ]]; then
      images+=($image_references)
    fi
  else
    echo "[WARNING] Manifest file $manifest_json does not exist. Skipping..."
  fi
done

# if [[ ${#images[@]} -eq 0 ]]; then
#     echo "[ERROR] No images found in IIB. Exiting."
#     exit 1
# fi


# Step 3: Convert image references to quay.io equivalents
echo "Converting image references to quay.io equivalents..."
quay_images=()
for image in "${images[@]}"; do
    quay_image="${REMOTE_REGISTRY}/$(echo $image | sed 's#.*rhdh/#rhdh/#')"
    quay_images+=("$quay_image")
done

# Step 4: Pull images from quay.io and push to local OpenShift registry
echo "Pulling and pushing images..."
for quay_image in "${quay_images[@]}"; do
    local_image="$LOCAL_REGISTRY/$NAMESPACE/$(echo $quay_image | cut -d'/' -f2-)"
    echo "Copying $quay_image to $local_image..."
    skopeo copy --all "docker://$quay_image" "docker://$local_image"
done

# Step 5: Create CatalogSource in OpenShift
echo "Creating CatalogSource..."
cat <<EOF | oc apply -f -
apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: $CATALOG_NAME
  namespace: $NAMESPACE
spec:
  sourceType: grpc
  image: $LOCAL_REGISTRY/$NAMESPACE/$(echo $IIB_IMAGE | sed 's#.*rhdh/#rhdh/#')
  displayName: Custom Catalog Source
  publisher: Red Hat
  updateStrategy:
    registryPoll:
      interval: 15m
EOF

echo "CatalogSource $CATALOG_NAME has been created and is using images from the local registry."
