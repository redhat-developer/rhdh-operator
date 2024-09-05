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

# Set the default namespace and other configuration
NAMESPACE="openshift-marketplace"       # Namespace for the CatalogSource
OPERATOR_NAMESPACE="rhdh-operator"      # Namespace for the Subscription and OperatorGroup
OLM_CHANNEL="fast"
REMOTE_REGISTRY="quay.io/rhdh"
LOCAL_REGISTRY="image-registry.openshift-image-registry.svc:5000"
CATALOG_NAME="my-catalog-source"
WORKDIR="/tmp/iib_extract"
OPERATOR_NAME="rhdh"

# Dynamically get the OCP version and architecture
OCP_VER="v$(oc version -o json | jq -r '.openshiftVersion' | sed -r -e "s#([0-9]+\.[0-9]+)\..+#\1#")"
OCP_ARCH="$(oc version -o json | jq -r '.serverVersion.platform' | sed -r -e "s#linux/##")"

# Normalize architecture if necessary
if [[ $OCP_ARCH == "amd64" ]]; then
  OCP_ARCH="x86_64"
fi

# Construct the image tag dynamically based on the OCP version and architecture
IIB_IMAGE="quay.io/rhdh/iib:latest-${OCP_VER}-${OCP_ARCH}"

# Echo the image being used for logging/debugging purposes
echo "[INFO] Using iib from image $IIB_IMAGE"

# Minimum requirements check
if [[ ! $(command -v oc) ]]; then
  echo "Please install oc 4.10+ from an RPM or https://mirror.openshift.com/pub/openshift-v4/clients/ocp/"
  exit 1
fi
if [[ ! $(command -v jq) ]]; then
  echo "Please install jq 1.2+ from an RPM or https://pypi.org/project/jq/"
  exit 1
fi

# Check we're logged into a cluster
if ! oc whoami > /dev/null 2>&1; then
  echo "Not logged into an OpenShift cluster"
  exit 1
fi

# Create the working directory
mkdir -p "$WORKDIR"

# Step 1: Pull the IIB image and extract catalog information
echo "Pulling IIB image and extracting catalog information..."
skopeo copy --all "docker://$IIB_IMAGE" "oci:$WORKDIR/iib"

# Unpack the OCI image
cd "$WORKDIR"
echo "Listing contents of blobs/sha256/ for debugging..."
ls -R "$WORKDIR/iib/blobs/sha256/"

# Inspect index.json to locate manifests
indexJson="$WORKDIR/iib/index.json"
if [[ -f $indexJson ]]; then
    manifests=$(jq -r '.manifests[].digest' "$indexJson")
    echo "Found the following manifests: $manifests"
    
    images=()
    
    for manifest in $manifests; do
        echo "Checking manifest: $manifest"
        manifestDir="$WORKDIR/iib/blobs/sha256/${manifest#*:}"
        
        manifestJson="$manifestDir/manifest.json"
        if [[ -f $manifestJson ]]; then
            layers=$(jq -r '.[].layers[].digest' "$manifestJson")
            config=$(jq -r '.[].config.digest' "$manifestJson")
            images+=($layers $config)
        else
            echo "[WARNING] No manifest.json found for $manifest"
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
    quay_image="${REMOTE_REGISTRY}/$(basename ${image#sha256:})"
    quay_images+=("$quay_image")
done

# Step 3: Pull images from Quay.io and push to local OpenShift registry
for quay_image in "${quay_images[@]}"; do
    local_image="$LOCAL_REGISTRY/$NAMESPACE/$(basename $quay_image)"
    skopeo copy --all "docker://$quay_image" "docker://$local_image" || {
        echo "[ERROR] Failed to copy $quay_image. Exiting."
        exit 1
    }
done

# Step 4: Apply ImageContentSourcePolicy
echo "Creating ImageContentSourcePolicy..."
oc apply -f - <<EOF
apiVersion: operator.openshift.io/v1alpha1
kind: ImageContentSourcePolicy
metadata:
  name: quay-registry
spec:
  repositoryDigestMirrors:
  - mirrors:
    - ${REMOTE_REGISTRY}
    source: quay.io
EOF

# Step 5: Create CatalogSource in OpenShift
echo "Creating CatalogSource..."
oc apply -f - <<EOF
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

# Step 6: Create OperatorGroup and Subscription
if ! oc get namespace "$OPERATOR_NAMESPACE" > /dev/null 2>&1; then
    oc create namespace "$OPERATOR_NAMESPACE"
fi

echo "Creating OperatorGroup..."
oc apply -f - <<EOF
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: $OPERATOR_NAME-operator-group
  namespace: $OPERATOR_NAMESPACE
spec:
  targetNamespaces:
  - $OPERATOR_NAMESPACE
EOF

echo "Creating Subscription for the RHDH 1.3 CI Operator..."
oc apply -f - <<EOF
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: $OPERATOR_NAME
  namespace: $OPERATOR_NAMESPACE
spec:
  channel: $OLM_CHANNEL
  installPlanApproval: Automatic
  name: $OPERATOR_NAME
  source: $CATALOG_NAME
  sourceNamespace: $NAMESPACE
EOF

echo "RHDH 1.3 CI Operator has been deployed from the cached images."
