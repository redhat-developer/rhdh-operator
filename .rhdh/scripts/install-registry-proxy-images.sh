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

# This script automates the installation of registry-proxy images onto an airgapped Kubernetes cluster. It pulls images locally, tags them for the local registry-proxy, pushes them into the cluster, and performs the catalog source installation.

set -e

# Define variables
REGISTRY_PROXY_URL=${1:-"registry-proxy.engineering.redhat.com"}
TARGET_NAMESPACE=${2:-"my-namespace"}
CATALOG_SOURCE_FILE=${3:-"catalog-source.yaml"}

# Function to pull image and get the SHA
pull_and_get_sha() {
  local image_ref=$1
  # Pull the image (using :latest or other tags if necessary)
  podman pull "$image_ref"
  # Get the SHA reference from the image
  podman inspect --format '{{index .RepoDigests 0}}' "$image_ref"
}

# Function to tag and push image
tag_and_push_image() {
  local image_ref=$1
  local sha_ref=$2

  # Extract the image name from the reference (e.g., quay.io/rhdh/rhdh-hub-rhel9)
  local image_name=$(basename "$image_ref")

  # Create a unique tag using a short SHA (12 characters)
  local short_sha=$(echo "$sha_ref" | cut -d':' -f2 | cut -c1-12)
  local tagged_image="${REGISTRY_PROXY_URL}/${image_name}:sha-${short_sha}"

  # Tag and push the image
  podman tag "$sha_ref" "$tagged_image"
  oc image mirror "$tagged_image" "$tagged_image" --insecure=true -n "$TARGET_NAMESPACE"
}

# List of images
IMAGE_REFERENCES=(
  "quay.io/rhdh/rhdh-hub-rhel9:latest"
  "quay.io/rhdh/rhdh-rhel9-operator:latest"
  "registry.redhat.io/openshift4/ose-kube-rbac-proxy:latest"
  "registry.redhat.io/rhel9/postgresql-15:latest"
)

# Pull, tag, and push images
for image_ref in "${IMAGE_REFERENCES[@]}"; do
  # Pull the image and get the full SHA reference
  sha_ref=$(pull_and_get_sha "$image_ref")

  # Tag and push the image to the registry proxy
  tag_and_push_image "$image_ref" "$sha_ref"
done

# Apply catalog source installation (use the provided or default catalog source file)
oc apply -f "$CATALOG_SOURCE_FILE" -n "$TARGET_NAMESPACE"

echo "Registry-proxy images successfully pushed into the cluster!"

