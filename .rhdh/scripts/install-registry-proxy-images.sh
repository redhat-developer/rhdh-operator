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
IIB_IMAGE="quay.io/rhdh/iib:latest-v4.14-x86_64"
WORKDIR="/tmp/iib_extract"

mkdir -p "$WORKDIR"

# Step 1: Pull the IIB image and extract catalog information
echo "Pulling IIB image and extracting catalog information..."
skopeo copy --all "docker://$IIB_IMAGE" "oci:$WORKDIR/iib"

# Unpack the OCI image
cd "$WORKDIR"
echo "Listing contents of blobs/sha256/ for debugging..."
ls -R "$WORKDIR/iib/blobs/sha256/"

# Attempt to inspect index.json to locate manifests
echo "Inspecting index.json..."
indexJson="$WORKDIR/iib/index.json"
if [[ -f $indexJson ]]; then
    manifests=$(jq -r '.manifests[].digest' "$indexJson")
    echo "Found the following manifests: $manifests"
    
    # Initialize an array to hold image references
    images=()
    
    # Check each manifest for image references
    for manifest in $manifests; do
        echo "Checking manifest: $manifest"
        manifestDir="$WORKDIR/iib/blobs/sha256/${manifest#*:}"
        
        # Extract tar files associated with the manifest
        tar_files=$(find "$manifestDir" -type f -name "*.tar")
        for tar_file in $tar_files; do
            echo "Extracting $tar_file..."
            tar -xf "$tar_file" -C "$WORKDIR"
        done
        
        # Attempt to locate and extract image references
        potentialJson=$(find "$WORKDIR" -name "*.json" -print -quit)
        if [[ -n $potentialJson ]]; then
            echo "Found potential JSON file: $potentialJson"
            image_references=$(jq -r '.. | objects | select(has("config") or has("layers")) | .config.digest // empty' "$potentialJson")
            if [[ -n $image_references ]]; then
                images+=($image_references)
            fi
        fi
    done
else
    echo "[ERROR] Could not find index.json. Exiting."
    exit 1
fi

# Step 2: Convert image references to Quay.io equivalents
echo "Converting image references to Quay.io equivalents..."
quay_images=()
for image in "${images[@]}"; do
    # Construct a valid Quay.io image path
    # Ensure the format is correct for the Quay.io repository
    quay_image="${REMOTE_REGISTRY}/rhdh/${image#sha256:}"
    echo "Mapping to Quay.io image: $quay_image"
    quay_images+=("$quay_image")
done

# Step 3: Pull images from Quay.io and push to local OpenShift registry
echo "Pulling and pushing images..."
for quay_image in "${quay_images[@]}"; do
    local_image="$LOCAL_REGISTRY/$NAMESPACE/$(basename $quay_image)"
    echo "Copying $quay_image to $local_image..."
    skopeo copy --all "docker://$quay_image" "docker://$local_image" || {
        echo "[ERROR] Failed to copy $quay_image. Exiting."
        exit 1
    }
done

# Step 4: Create CatalogSource in OpenShift
echo "Creating CatalogSource..."
cat <<EOF | oc apply -f -
apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: $CATALOG_NAME
  namespace: $NAMESPACE
spec:
  sourceType: grpc
  image: $LOCAL_REGISTRY/$NAMESPACE/$(basename $IIB_IMAGE)
  displayName: Custom Catalog Source
  publisher: Red Hat
  updateStrategy:
    registryPoll:
      interval: 15m
EOF

echo "CatalogSource $CATALOG_NAME has been created and is using images from the local registry."