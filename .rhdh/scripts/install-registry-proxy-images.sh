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

# Define your registry-proxy URL (replace with your actual registry-proxy URL)
REGISTRY_PROXY_URL="registry-proxy.engineering.redhat.com"

# Define the target namespace in your cluster
NAMESPACE="my-namespace"

# List of image references (replace with your actual image references)
IMAGE_REFERENCES=(
  "quay.io/rhdh/iib:1.2-v4.14-x86_64"
  "quay.io/rhdh/rhdh-hub-rhel9@sha256:3dda237c066787dd3e04df006a1ad7ce56eda8e8c89c31bfec0e7d115ed08f54"
  "quay.io/rhdh/rhdh-rhel9-operator@sha256:1d7fa4a8e6f66d4bbd31fb8a8e910ddf69987c6cec50d83222cb4d6f39dae5e8"
  "registry-proxy.engineering.redhat.com/rh-osbs/rhdh-rhdh-operator-bundle@sha256:fe079e6ce4d08d300062b7c302ad15209b4984aeb3b4dcb7dcb73e29cdf75e81"
  "registry.redhat.io/openshift4/ose-kube-rbac-proxy@sha256:55a49d79b9d29757328762e8a4755013fba1ead5a2416cc737e8b06dd2a77eef"
  "registry.redhat.io/rhdh/rhdh-hub-rhel9@sha256:3dda237c066787dd3e04df006a1ad7ce56eda8e8c89c31bfec0e7d115ed08f54"
  "registry.redhat.io/rhdh/rhdh-rhel9-operator@sha256:1d7fa4a8e6f66d4bbd31fb8a8e910ddf69987c6cec50d83222cb4d6f39dae5e8"
  "registry.redhat.io/rhel9/postgresql-15@sha256:5c4cad6de1b8e2537c845ef43b588a11347a3297bfab5ea611c032f866a1cb4e"
)

# Pull images locally
for image_ref in "${IMAGE_REFERENCES[@]}"; do
  docker pull "$image_ref"
done

# Tag images for the local registry-proxy
for image_ref in "${IMAGE_REFERENCES[@]}"; do
  local_image_name="${image_ref##*/}"
  local_image_tag="${image_ref##*@}"
  local_image="${REGISTRY_PROXY_URL}/${local_image_name}@${local_image_tag}"
  docker tag "$image_ref" "$local_image"
done

# Push images into the cluster
for local_image in "${IMAGE_REFERENCES[@]}"; do
  oc image mirror "$local_image" "$local_image" --insecure=true -n "$NAMESPACE"
done

# Perform catalog source installation
oc apply -f catalog-source.yaml -n "$NAMESPACE"

echo "Registry-proxy images successfully pushed into the cluster!"
