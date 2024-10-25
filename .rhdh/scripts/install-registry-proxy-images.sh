#!/bin/bash
# Fail on error
set -e

# Example usage:
# ./install-registry-proxy-images.sh \
#   --prod_operator_index "registry.redhat.io/redhat/redhat-operator-index:v4.14" \
#   --prod_operator_package_name "rhdh" \
#   --prod_operator_bundle_name "rhdh-operator" \
#   --prod_operator_version "v1.3.0" \
#   --helper_mirror_registry_storage "30Gi" \
#   --use_existing_mirror_registry "$MY_MIRROR_REGISTRY"

# Parse input arguments
while [ $# -gt 0 ]; do
  if [[ $1 == *"--"* ]]; then
    param="${1/--/}"
    declare "$param"="$2"
  fi
  shift
done

# Operators
declare prod_operator_index="${prod_operator_index:?Must set --prod_operator_index}"
declare prod_operator_package_name="rhdh"
declare prod_operator_bundle_name="rhdh-operator"
declare prod_operator_version="${prod_operator_version:?Must set --prod_operator_version}"

# Destination registry and configurations
declare my_operator_index_image_name_and_tag=${prod_operator_package_name}-index:${prod_operator_version}
declare helper_mirror_registry_storage=${helper_mirror_registry_storage:-"30Gi"}
declare my_catalog=${prod_operator_package_name}-disconnected-install
declare k8s_resource_name=${my_catalog}

# Check if logged into OpenShift cluster
if ! oc whoami > /dev/null 2>&1; then
  echo "[ERROR] Not logged into an OpenShift cluster."
  exit 1
fi

# Check for hosted control plane
CONTROL_PLANE_TECH=$(oc get infrastructure cluster -o jsonpath='{.status.controlPlaneTopology}')
IS_HOSTED_CONTROL_PLANE="false"
if [[ "${CONTROL_PLANE_TECH}" == "External" ]]; then
  IS_HOSTED_CONTROL_PLANE="true"
  echo "[INFO] Detected a hosted control plane. Adjusting strategy for airgapped setup..."
fi

# Deploy mirror registry if needed
function deploy_mirror_registry() {
    echo "[INFO] Deploying mirror registry..." >&2
    local namespace="airgap-helper-ns"
    local image="registry:2"
    local username="registryuser"
    local password=$(echo "$RANDOM" | base64 | head -c 20)

    if ! oc get namespace "${namespace}" &> /dev/null; then
      echo "  Creating namespace ${namespace}..." >&2
      oc create namespace "${namespace}" >&2
    fi

    registry_htpasswd=$(htpasswd -Bbn "${username}" "${password}")
    cat <<EOF | oc apply -f - >&2
apiVersion: v1
kind: Secret
type: Opaque
metadata:
  name: airgap-registry-auth
  namespace: "${namespace}"
  labels:
    app: airgap-registry
stringData:
  htpasswd: "${registry_htpasswd}"
EOF

    # PVC, Deployment, and Service setup for mirror registry
    storage_class=$(oc get storageclasses -o=jsonpath='{.items[?(@.metadata.annotations.storageclass\.kubernetes\.io/is-default-class=="true")].metadata.name}')
    cat <<EOF | oc apply -f - >&2
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: airgap-registry-storage
  namespace: "${namespace}"
spec:
  resources:
    requests:
      storage: "${helper_mirror_registry_storage}"
  storageClassName: "${storage_class}"
  accessModes:
    - ReadWriteOnce
EOF

    cat <<EOF | oc replace --force -f - >&2
apiVersion: apps/v1
kind: Deployment
metadata:
  name: airgap-registry
  namespace: "${namespace}"
  labels:
    app: airgap-registry
spec:
  replicas: 1
  selector:
    matchLabels:
      app: airgap-registry
  template:
    metadata:
      labels:
        app: airgap-registry
    spec:
      containers:
        - image: "${image}"
          name: airgap-registry
          ports:
            - containerPort: 5000
          volumeMounts:
            - name: registry-vol
              mountPath: /var/lib/registry
            - name: auth-vol
              mountPath: "/auth"
              readOnly: true
      volumes:
        - name: registry-vol
          persistentVolumeClaim:
            claimName: airgap-registry-storage
        - name: auth-vol
          secret:
            secretName: airgap-registry-auth
EOF

    cat <<EOF | oc apply -f - >&2
apiVersion: v1
kind: Service
metadata:
  name: airgap-registry
  namespace: "${namespace}"
spec:
  ports:
    - port: 5000
      protocol: TCP
      targetPort: 5000
  selector:
    app: airgap-registry
EOF

    oc -n "${namespace}" create route edge --service=airgap-registry --insecure-policy=Redirect >&2
    registry_url=$(oc get route airgap-registry -n "${namespace}" --template='{{ .spec.host }}')
    podman login -u="${username}" -p="${password}" "${registry_url}" --tls-verify=false >&2
    echo "[INFO] Mirror registry ready at: ${registry_url}" >&2
    echo "${registry_url}"
}

# Deploy or use existing mirror registry
declare my_registry="${use_existing_mirror_registry}"
if [ -z "${my_registry}" ]; then
  my_registry=$(deploy_mirror_registry)
fi

# Function to transform operator bundle
function transform_operator_bundle() {
  local bundle_image="$1"
  local my_registry="$2"
  
  digest="${bundle_image##*@sha256:}"
  transformed_image="${my_registry}/rhdh/rhdh-operator-bundle:${digest}"
  container_id=$(podman create "$bundle_image")

  podman cp "${container_id}:/manifests" "./bundles/${digest}/manifests"
  podman rm -f "${container_id}"

  # Update references in manifests
  for file in ./bundles/${digest}/manifests/*; do
    sed -i 's#registry.redhat.io/rhdh#quay.io/rhdh#g' "$file"
    sed -i 's#registry.stage.redhat.io/rhdh#quay.io/rhdh#g' "$file"
  done

  podman build -t "${transformed_image}" -f ./bundles/${digest}/bundle.Dockerfile .
  podman push "${transformed_image}" --tls-verify=false
}

# Fetch and transform operator bundle if on a hosted control plane
if [[ "${IS_HOSTED_CONTROL_PLANE}" == "true" ]]; then
  echo "[INFO] Using hosted control plane strategy..."
  transform_operator_bundle "${prod_operator_index}" "${my_registry}"
fi

# Set up the CatalogSource with updated images
echo "apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: ${prod_operator_package_name}-catalog
  namespace: openshift-marketplace
spec:
  sourceType: grpc
  image: ${transformed_image}
  displayName: RHDH Catalog Source
  publisher: Red Hat
" | oc apply -f -

echo "[INFO] ${prod_operator_package_name} CatalogSource deployed."
