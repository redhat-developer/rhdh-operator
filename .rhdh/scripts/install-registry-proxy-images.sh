#!/bin/bash
# Fail on error
set -e

# Example usage:
# ./prepare-restricted-environment.sh \
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
declare prod_operator_index="${prod_operator_index:?Must set --prod_operator_index: for OCP 4.12+, use registry.redhat.io/redhat/redhat-operator-index:v4.14 or quay.io/rhdh/iib:latest-v4.14-x86_64}"
declare prod_operator_package_name="rhdh"
declare prod_operator_bundle_name="rhdh-operator"
declare prod_operator_version="${prod_operator_version:?Must set --prod_operator_version: use v1.3.x, v1.3.1, etc.}"

# Destination registry
declare my_operator_index_image_name_and_tag=${prod_operator_package_name}-index:${prod_operator_version}
declare helper_mirror_registry_storage=${helper_mirror_registry_storage:-"30Gi"}
declare my_catalog=${prod_operator_package_name}-disconnected-install
declare k8s_resource_name=${my_catalog}

# Check if logged into OpenShift cluster
if ! oc whoami > /dev/null 2>&1; then
  echo "[ERROR] Not logged into an OpenShift cluster."
  exit 1
fi

# Check OpenShift version and architecture
OCP_VER="$(oc version -o json | jq -r '.openshiftVersion' | sed -r -e "s#([0-9]+\.[0-9]+\.[0-9]+)-.+#\1#")"
OCP_VER_MAJOR="$(oc version -o json | jq -r '.openshiftVersion' | sed -r -e "s#([0-9]+)\..+#\1#")"
OCP_ARCH="$(oc version -o json | jq -r '.serverVersion.platform' | sed -r -e "s#linux/##")"
if [[ $OCP_ARCH == "amd64" ]]; then OCP_ARCH="x86_64"; fi

# Check for ROSA cluster
CLUSTER_TYPE=$(oc get infrastructure cluster -o jsonpath='{.status.platform}')
if [[ $CLUSTER_TYPE == "AWS" ]]; then
  echo "[INFO] Detected ROSA/AWS cluster. Ensuring compatibility with airgapped setup..."
  # Add ROSA-specific logic if needed
fi

function deploy_mirror_registry() {
    echo "[INFO] Deploying mirror registry..." >&2
    local namespace="airgap-helper-ns"
    local image="registry:2"
    local username="registryuser"
    local password=$(echo "$RANDOM" | base64 | head -c 20)

    if ! oc get namespace "${namespace}" &> /dev/null; then
      echo "  namespace ${namespace} does not exist - creating it..." >&2
      oc create namespace "${namespace}" >&2
    fi

    registry_htpasswd=$(htpasswd -Bbn "${username}" "${password}")
    echo "  generating auth secret for mirror registry. Those creds will be stored in 'airgap-registry-auth-creds' in ${namespace} ..." >&2
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

    cat <<EOF | oc apply -f - >&2
apiVersion: v1
kind: Secret
type: Opaque
metadata:
  name: airgap-registry-auth-creds
  namespace: "${namespace}"
  labels:
    app: airgap-registry
stringData:
  username: "${username}"
  password: "${password}"
EOF

    # Use default storage class if not provided
    if [ -z "$storage_class" ]; then
      storage_class=$(oc get storageclasses -o=jsonpath='{.items[?(@.metadata.annotations.storageclass\.kubernetes\.io/is-default-class=="true")].metadata.name}')
    fi

    echo "  creating PVC for mirror registry with storage class ${storage_class}: persistentvolumeclaim/airgap-registry-storage ..." >&2
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

    echo "  creating mirror registry Deployment: deployment/airgap-registry ..." >&2
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
          imagePullPolicy: IfNotPresent
          env:
            - name: REGISTRY_AUTH
              value: "htpasswd"
            - name: REGISTRY_AUTH_HTPASSWD_REALM
              value: "RHDH Private Registry"
            - name: REGISTRY_AUTH_HTPASSWD_PATH
              value: "/auth/htpasswd"
            - name: REGISTRY_STORAGE_DELETE_ENABLED
              value: "true"
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

    echo "  creating mirror registry Service: service/airgap-registry ..." >&2
    cat <<EOF | oc apply -f - >&2
apiVersion: v1
kind: Service
metadata:
  name: airgap-registry
  namespace: "${namespace}"
  labels:
    app: airgap-registry
spec:
  type: ClusterIP
  ports:
    - port: 5000
      protocol: TCP
      targetPort: 5000
  selector:
    app: airgap-registry
EOF

    echo "  creating Route to access mirror registry: route/airgap-registry ..." >&2
    oc -n "${namespace}" create route edge --service=airgap-registry --insecure-policy=Redirect --dry-run=client -o yaml \
      | oc -n "${namespace}" apply -f - >&2

    local registry_url=$(oc get route airgap-registry -n "${namespace}" --template='{{ .spec.host }}')
    echo "... done. Mirror registry should now be reachable at: ${registry_url} ..." >&2

    # Wait until url is ready
    echo "[INFO] Waiting for mirror registry to be ready and reachable ..." >&2
    curl --insecure -IL "${registry_url}" --retry 100 --retry-all-errors --retry-max-time 900 --fail &> /tmp/"${registry_url}.log" >&2

    echo "[INFO] Log into mirror registry to be able to push images to it..." >&2
    podman login -u="${username}" -p="${password}" "${registry_url}" --tls-verify=false >&2

    echo "[INFO] Marking mirror registry as insecure in the cluster ..." >&2
    oc patch image.config.openshift.io/cluster --patch '{"spec":{"registrySources":{"insecureRegistries":["'${registry_url}'"]}}}' --type=merge >&2

    echo "[INFO] Adding mirror registry creds to cluster global pull secret ..." >&2
    echo "  downloading global pull secret from the cluster ..." >&2
    oc get secret/pull-secret -n openshift-config --template='{{index .data ".dockerconfigjson" | base64decode}}' > /tmp/my-global-pull-secret-for-mirror-reg.yaml
    echo "   log into mirror registry and store creds into the pull secret downloaded..." >&2
    oc registry login \
      --insecure=true \
      --registry="${registry_url}" \
      --auth-basic="${username}:${password}" \
      --to=/tmp/my-global-pull-secret-for-mirror-reg.yaml \
       >&2
    echo "  writing updated pull secret into the cluster ..." >&2
    oc set data secret/pull-secret -n openshift-config --from-file=.dockerconfigjson=/tmp/my-global-pull-secret-for-mirror-reg.yaml >&2

    # Mirror OCP release images
    echo "[INFO] Mirroring OCP release images for airgapped environments..." >&2
    local ocp_product_repo='openshift-release-dev'
    local ocp_release_name="ocp-release"
    local ocp_local_repo="ocp/openshift"
    oc adm release mirror -a /tmp/my-global-pull-secret-for-mirror-reg.yaml \
      --from="quay.io/${ocp_product_repo}/${ocp_release_name}:${OCP_VER}-${OCP_ARCH}" \
      --to="${registry_url}/${ocp_local_repo}" \
      --to-release-image="${registry_url}/${ocp_local_repo}:${OCP_VER}-${OCP_ARCH}" \
      --insecure=true > /tmp/oc-adm-release-mirror__mirror-registry.out

    # Create ImageContentSourcePolicy for OCP release images
    echo "[INFO] Creating ImageContentSourcePolicy for OCP release images..." >&2
    cat <<EOF | oc apply -f - >&2
apiVersion: operator.openshift.io/v1alpha1
kind: ImageContentSourcePolicy
metadata:
  name: ocp-release
  labels:
    app: airgap-registry
spec:
  repositoryDigestMirrors:
  - mirrors:
    - "${registry_url}/${ocp_local_repo}"
    source: quay.io/openshift-release-dev/ocp-release
  - mirrors:
    - "${registry_url}/${ocp_local_repo}"
    source: "quay.io/openshift-release-dev/ocp-v${OCP_VER_MAJOR}.0-art-dev"
EOF

    echo "[INFO] Cleaning up temporary files..." >&2
    rm -f /tmp/my-global-pull-secret-for-mirror-reg.yaml /tmp/oc-adm-release-mirror__mirror-registry.out >&2

    echo "[INFO] Mirror registry ready: ${registry_url}" >&2
    echo "${registry_url}"
}

# Deploy or use existing mirror registry
declare my_registry="${use_existing_mirror_registry}"
if [ -z "${my_registry}" ]; then
  my_registry=$(deploy_mirror_registry)
fi

declare my_operator_index="${my_registry}/${prod_operator_package_name}/${my_operator_index_image_name_and_tag}"

# Create local directory for catalog
mkdir -p "${my_catalog}/${prod_operator_package_name}"

# Fetch metadata for the operator catalog
echo "[INFO] Fetching metadata for the ${prod_operator_package_name} operator catalog."
opm render "${my_operator_index}" \
  | jq "select \
    (\
      (.schema == \"olm.bundle\" and .name == \"${prod_operator_bundle_name}.${prod_operator_version}\") or \
      (.schema == \"olm.package\" and .name == \"${prod_operator_package_name}\") or \
      (.schema == \"olm.channel\" and .package == \"${prod_operator_package_name}\") \
    )" \
  | jq "select \
     (.schema == \"olm.channel\" and .package == \"${prod_operator_package_name}\").entries \
      |= [{name: \"${prod_operator_bundle_name}.${prod_operator_version}\"}]" \
  > "${my_catalog}/${prod_operator_package_name}/render.json"

echo "[INFO] Creating the catalog Dockerfile."
if [ -f "${my_catalog}.Dockerfile" ]; then
  rm -f "${my_catalog}.Dockerfile"
fi
opm generate dockerfile "./${my_catalog}"

# Build the catalog image locally
echo "[INFO] Building the catalog image locally."
podman build -t "${my_operator_index}" -f "./${my_catalog}.Dockerfile" --no-cache .

# Disable default Red Hat OperatorHub sources
echo "[INFO] Disabling the default Red Hat Ecosystem Catalog."
oc patch OperatorHub cluster --type json \
    --patch '[{"op": "add", "path": "/spec/disableAllDefaultSources", "value": true}]'

# Push the catalog image to the registry
echo "[INFO] Deploying catalog image to ${my_operator_index} registry."
skopeo copy --src-tls-verify=false --dest-tls-verify=false --all "containers-storage:$my_operator_index" "docker://$my_operator_index"

# Remove index image from mapping.txt for mirroring
oc adm catalog mirror "$my_operator_index" "$my_registry" --insecure --manifests-only | tee catalog_mirror.log
MANIFESTS_FOLDER=$(sed -n -e 's/^wrote mirroring manifests to \(.*\)$/\1/p' catalog_mirror.log |xargs)
sed -i -e "/${my_operator_index_image_name_and_tag}/d" "${MANIFESTS_FOLDER}/mapping.txt"

# Mirror related images to the registry
echo "[INFO] Mirroring related images to the ${my_registry} registry."
while IFS= read -r line; do
  public_image=$(echo "${line}" | cut -d '=' -f1)
  if [[ "$prod_operator_index" != registry.redhat.io/redhat/redhat-operator-index* ]] && [[ "$public_image" == registry.redhat.io/rhdh/* ]]; then
    if ! skopeo inspect "docker://$public_image" &> /dev/null; then
      echo "  Replacing non-public CI image $public_image ..."
      public_image=${public_image/registry.redhat.io\/rhdh/quay.io\/rhdh}
      echo "    => $public_image"
    fi
  fi
  private_image=$(echo "${line}" | cut -d '=' -f2)
  echo "[INFO] Mirroring ${public_image}"
  skopeo copy --dest-tls-verify=false --preserve-digests --all "docker://$public_image" "docker://$private_image"
done < "${MANIFESTS_FOLDER}/mapping.txt"

# Create CatalogSource and ImageContentSourcePolicy
echo "[INFO] Creating CatalogSource and ImageContentSourcePolicy"
cat "${MANIFESTS_FOLDER}/catalogSource.yaml" | sed 's|name: .*|name: '${k8s_resource_name}'|' | oc apply -f -
cat "${MANIFESTS_FOLDER}/imageContentSourcePolicy.yaml" | sed 's|name: .*|name: '${k8s_resource_name}'|' | oc apply -f -

echo "[INFO] Catalog $my_operator_index deployed to the $my_registry registry."
