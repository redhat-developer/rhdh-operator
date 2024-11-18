#!/bin/bash
#
# Script to streamline installing an IIB image in an OpenShift cluster for testing.
#
# Requires: oc, jq

set -e

RED='\033[0;31m'
NC='\033[0m'

NAMESPACE_CATALOGSOURCE="openshift-marketplace"
NAMESPACE_SUBSCRIPTION="rhdh-operator"
OLM_CHANNEL="fast"

errorf() {
  echo -e "${RED}$1${NC}"
}

usage() {
echo "
This script streamlines testing IIB images by configuring an OpenShift cluster to enable it to use the specified IIB image
as a catalog source. The CatalogSource is created in the openshift-marketplace namespace,
and is named 'operatorName-channelName', eg., rhdh-fast

If IIB installation fails, see https://docs.engineering.redhat.com/display/CFC/Test and
follow steps in section 'Adding Brew Pull Secret'

Usage:
  $0 [OPTIONS]

Options:
  -v 1.y                       : Install from iib quay.io/rhdh/iib:1.y-\$OCP_VER-\$OCP_ARCH (eg., 1.4-v4.14-x86_64)
  --latest                     : Install from iib quay.io/rhdh/iib:latest-\$OCP_VER-\$OCP_ARCH (eg., latest-v4.14-x86_64) [default]
  --next                       : Install from iib quay.io/rhdh/iib:next-\$OCP_VER-\$OCP_ARCH (eg., next-v4.14-x86_64)
  --install-operator <NAME>    : Install operator named \$NAME after creating CatalogSource

Examples:
  $0 \\
    --install-operator rhdh          # RC release in progess (from latest tag and stable branch )

  $0 \\
    --next --install-operator rhdh   # CI future release (from next tag and upstream main branch)
"
}

if [[ "$#" -lt 1 ]]; then usage; exit 0; fi

# minimum requirements
if [[ ! $(command -v oc) ]]; then
  errorf "Please install oc 4.10+ from an RPM or https://mirror.openshift.com/pub/openshift-v4/clients/ocp/"
  exit 1
fi
if [[ ! $(command -v jq) ]]; then
  errorf "Please install jq 1.2+ from an RPM or https://pypi.org/project/jq/"
  exit 1
fi

# Check we're logged into a cluster
if ! oc whoami > /dev/null 2>&1; then
  errorf "Not logged into an OpenShift cluster"
  exit 1
fi

# log into your OCP cluster before running this or you'll get null values for OCP vars!
OCP_VER="v$(oc version -o json | jq -r '.openshiftVersion' | sed -r -e "s#([0-9]+\.[0-9]+)\..+#\1#")"
OCP_ARCH="$(oc version -o json | jq -r '.serverVersion.platform' | sed -r -e "s#linux/##")"
if [[ $OCP_ARCH == "amd64" ]]; then OCP_ARCH="x86_64"; fi
# if logged in, this should return something like latest-v4.12-x86_64
IIB_TAG="latest-${OCP_VER}-${OCP_ARCH}"

while [[ "$#" -gt 0 ]]; do
  case $1 in
    '--install-operator')
      # Create project if necessary
      if ! oc get project "$NAMESPACE_SUBSCRIPTION" > /dev/null 2>&1; then
        echo "Project $NAMESPACE_SUBSCRIPTION does not exist; creating it"
        oc create namespace "$NAMESPACE_SUBSCRIPTION"
      fi
      TO_INSTALL="$2"; shift 1;;
    '--next'|'--latest')
      # if logged in, this should return something like latest-v4.12-x86_64 or next-v4.12-x86_64
      IIB_TAG="${1/--/}-${OCP_VER}-$OCP_ARCH";;
    '-v')
      IIB_TAG="${2}-${OCP_VER}-$OCP_ARCH";
      OLM_CHANNEL="fast-${2}"
      shift 1;;
    '-h'|'--help') usage; exit 0;;
    *) echo "[ERROR] Unknown parameter is used: $1."; usage; exit 1;;
  esac
  shift 1
done

# check if the IIB we're going to install as a catalog source exists before trying to install it
if [[ ! $(command -v skopeo) ]]; then
  errorf "Please install skopeo 1.11+"
  exit 1
fi

UPSTREAM_IIB="quay.io/rhdh/iib:${IIB_TAG}";

# shellcheck disable=SC2086
UPSTREAM_IIB_MANIFEST="$(skopeo inspect docker://${UPSTREAM_IIB} --raw || exit 2)"
# echo "Got: $UPSTREAM_IIB_MANIFEST"
if [[ $UPSTREAM_IIB_MANIFEST == *"Error parsing image name "* ]] || [[ $UPSTREAM_IIB_MANIFEST == *"manifest unknown"* ]]; then
  echo "$UPSTREAM_IIB_MANIFEST"; exit 3
else
  echo "[INFO] Using iib from image $UPSTREAM_IIB"
  IIB_IMAGE="${UPSTREAM_IIB}"
fi

TMPDIR=$(mktemp -d)
# shellcheck disable=SC2064
trap "rm -fr $TMPDIR" EXIT

CATALOGSOURCE_NAME="${TO_INSTALL}-${OLM_CHANNEL}"
DISPLAY_NAME_SUFFIX="${TO_INSTALL}"

# Add CatalogSource for the IIB
if [ -z "$TO_INSTALL" ]; then
  IIB_NAME="${UPSTREAM_IIB##*:}"
  IIB_NAME="${IIB_NAME//_/-}"
  IIB_NAME="${IIB_NAME//./-}"
  IIB_NAME="$(echo "$IIB_NAME" | tr '[:upper:]' '[:lower:]')"
  CATALOGSOURCE_NAME="rhdh-iib-${IIB_NAME}-${OLM_CHANNEL}"
  DISPLAY_NAME_SUFFIX="${IIB_NAME}"
fi

function install_regular_cluster() {
  # A regular cluster should support ImageContentSourcePolicy/ImageDigestMirrorSet resources
  ICSP_URL="quay.io/rhdh/"
  ICSP_URL_PRE=${ICSP_URL%%/*}

  # for 1.4+, use IDMS instead of ICSP
  # TODO https://issues.redhat.com/browse/RHIDP-4188 if we onboard 1.3 to Konflux, use IDMS for latest too
  if [[ "$IIB_IMAGE" == *"next"* ]]; then
    echo "[INFO] Adding ImageDigestMirrorSet to resolve unreleased images on registry.redhat.io from quay.io" >&2
    echo "---
apiVersion: config.openshift.io/v1
kind: ImageDigestMirrorSet
metadata:
  name: ${ICSP_URL_PRE//./-}
spec:
  imageDigestMirrors:
  - source: registry.redhat.io/rhdh/rhdh-hub-rhel9
    mirrors:
      - ${ICSP_URL}rhdh-hub-rhel9
  - source: registry.redhat.io/rhdh/rhdh-rhel9-operator
    mirrors:
      - ${ICSP_URL}rhdh-rhel9-operator
  - source: registry-proxy.engineering.redhat.com/rh-osbs/rhdh-rhdh-operator-bundle
    mirrors:
      - ${ICSP_URL}rhdh-operator-bundle
  " > "$TMPDIR/ImageDigestMirrorSet_${ICSP_URL_PRE}.yml" && oc apply -f "$TMPDIR/ImageDigestMirrorSet_${ICSP_URL_PRE}.yml" >&2
  else
    echo "[INFO] Adding ImageContentSourcePolicy to resolve references to images not on quay.io as if from quay.io" >&2
    # echo "[DEBUG] ${ICSP_URL_PRE}, ${ICSP_URL_PRE//./-}, ${ICSP_URL}"
    echo "---
apiVersion: operator.openshift.io/v1alpha1
kind: ImageContentSourcePolicy
metadata:
  name: ${ICSP_URL_PRE//./-}
spec:
  repositoryDigestMirrors:
  ## 1. add mappings for Developer Hub bundle, operator, hub
  - mirrors:
    - ${ICSP_URL}rhdh-operator-bundle
    source: registry.redhat.io/rhdh/rhdh-operator-bundle
  - mirrors:
    - ${ICSP_URL}rhdh-operator-bundle
    source: registry.stage.redhat.io/rhdh/rhdh-operator-bundle
  - mirrors:
    - ${ICSP_URL}rhdh-operator-bundle
    source: registry-proxy.engineering.redhat.com/rh-osbs/rhdh-rhdh-operator-bundle

  - mirrors:
    - ${ICSP_URL}rhdh-rhel9-operator
    source: registry.redhat.io/rhdh/rhdh-rhel9-operator
  - mirrors:
    - ${ICSP_URL}rhdh-rhel9-operator
    source: registry.stage.redhat.io/rhdh/rhdh-rhel9-operator
  - mirrors:
    - ${ICSP_URL}rhdh-rhel9-operator
    source: registry-proxy.engineering.redhat.com/rh-osbs/rhdh-rhdh-rhel9-operator

  - mirrors:
    - ${ICSP_URL}rhdh-hub-rhel9
    source: registry.redhat.io/rhdh/rhdh-hub-rhel9
  - mirrors:
    - ${ICSP_URL}rhdh-hub-rhel9
    source: registry.stage.redhat.io/rhdh/rhdh-hub-rhel9
  - mirrors:
    - ${ICSP_URL}rhdh-hub-rhel9
    source: registry-proxy.engineering.redhat.com/rh-osbs/rhdh-rhdh-hub-rhel9

  ## 2. general repo mappings
  - mirrors:
    - ${ICSP_URL_PRE}
    source: registry.redhat.io
  - mirrors:
    - ${ICSP_URL_PRE}
    source: registry.stage.redhat.io
  - mirrors:
    - ${ICSP_URL_PRE}
    source: registry-proxy.engineering.redhat.com

  ### now add mappings to resolve internal references
  - mirrors:
    - registry.redhat.io
    source: registry.stage.redhat.io
  - mirrors:
    - registry.stage.redhat.io
    source: registry-proxy.engineering.redhat.com
  - mirrors:
    - registry.redhat.io
    source: registry-proxy.engineering.redhat.com
  " > "$TMPDIR/ImageContentSourcePolicy_${ICSP_URL_PRE}.yml" && oc apply -f "$TMPDIR/ImageContentSourcePolicy_${ICSP_URL_PRE}.yml" >&2
  fi

  printf "%s" "${IIB_IMAGE}"
}

function install_hosted_control_plane_cluster() {
  # Clusters with an hosted control plane do not propagate ImageContentSourcePolicy/ImageDigestMirrorSet resources
  # to the underlying nodes, causing an issue mirroring internal images effectively.
  # This function works around this by locally modifying the bundles (replacing all refs to the internal registries
  # with their mirrors on quay.io), rebuilding and pushing the images to the internal cluster registry.
  if [[ ! $(command -v umoci) ]]; then
    errorf "Please install umoci 0.4+. See https://github.com/opencontainers/umoci"
    exit 1
  fi

  mkdir -p "${TMPDIR}/rhdh/rhdh" >&2
  echo "[DEBUG] Rendering IIB $UPSTREAM_IIB as a local file..." >&2
  opm render "$UPSTREAM_IIB" --output=yaml > "${TMPDIR}/rhdh/rhdh/render.yaml"
  if [ ! -s "${TMPDIR}/rhdh/rhdh/render.yaml" ]; then
    errorf "[ERROR] 'opm render $UPSTREAM_IIB' returned an empty output, which likely means that this IIB Image does not contain any operators in it. Please reach out to the RHDH Productization team." >&2
    exit 1
  fi

  # 1. Expose the internal cluster registry if not done already
  echo "[DEBUG] Exposing cluster registry..." >&2
  internal_registry_url="image-registry.openshift-image-registry.svc:5000"
  oc patch configs.imageregistry.operator.openshift.io/cluster --patch '{"spec":{"defaultRoute":true}}' --type=merge >&2
  my_registry=$(oc get route default-route -n openshift-image-registry --template='{{ .spec.host }}')
  skopeo login -u kubeadmin -p $(oc whoami -t) --tls-verify=false $my_registry >&2
  if oc -n openshift-marketplace get secret internal-reg-auth-for-rhdh &> /dev/null; then
    oc -n openshift-marketplace delete secret internal-reg-auth-for-rhdh >&2
  fi
  if oc -n openshift-marketplace get secret internal-reg-ext-auth-for-rhdh &> /dev/null; then
    oc -n openshift-marketplace delete secret internal-reg-ext-auth-for-rhdh >&2
  fi
  oc -n openshift-marketplace create secret docker-registry internal-reg-ext-auth-for-rhdh \
    --docker-server=${my_registry} \
    --docker-username=kubeadmin \
    --docker-password=$(oc whoami -t) \
    --docker-email="admin@internal-registry-ext.example.com" >&2
  oc -n openshift-marketplace create secret docker-registry internal-reg-auth-for-rhdh \
    --docker-server=${internal_registry_url} \
    --docker-username=kubeadmin \
    --docker-password=$(oc whoami -t) \
    --docker-email="admin@internal-registry.example.com" >&2
  oc registry login --registry="$my_registry" --auth-basic="kubeadmin:$(oc whoami -t)" >&2
  for ns in rhdh-operator rhdh; do
    # To be able to push images under this scope in the internal image registry
    if ! oc get namespace "$ns" > /dev/null; then
      oc create namespace "$ns" >&2
    fi
    oc adm policy add-cluster-role-to-user system:image-signer system:serviceaccount:${ns}:default >&2 || true
  done
  oc policy add-role-to-user system:image-puller system:serviceaccount:openshift-marketplace:default -n openshift-marketplace >&2 || true
  oc policy add-role-to-user system:image-puller system:serviceaccount:rhdh-operator:default -n rhdh-operator >&2 || true

  # 2. Render the IIB locally, modify any references to the internal registries with their mirrors on Quay
  # and push the updates to the internal cluster registry
  for bundleImg in $(cat "${TMPDIR}/rhdh/rhdh/render.yaml" | grep -E '^image: .*operator-bundle' | awk '{print $2}' | uniq); do
    originalBundleImg="$bundleImg"
    digest="${originalBundleImg##*@sha256:}"
    bundleImg="${bundleImg/registry.stage.redhat.io/quay.io}"
    bundleImg="${bundleImg/registry.redhat.io/quay.io}"
    bundleImg="${bundleImg/registry-proxy.engineering.redhat.com\/rh-osbs\/rhdh-/quay.io\/rhdh\/}"
    echo "[DEBUG] $originalBundleImg => $bundleImg" >&2
    if skopeo inspect "docker://$bundleImg" &> /dev/null; then
      newBundleImage="${my_registry}/rhdh/rhdh-operator-bundle:${digest}"
      newBundleImageAsInt="${internal_registry_url}/rhdh/rhdh-operator-bundle:${digest}"
      mkdir -p "bundles/$digest" >&2

      echo "[DEBUG] Copying and unpacking image $bundleImg locally..." >&2
      skopeo copy "docker://$bundleImg" "oci:./bundles/${digest}/src:latest" >&2
      umoci unpack --image "./bundles/${digest}/src:latest" "./bundles/${digest}/unpacked" --rootless >&2

      # Replace the occurrences in the .csv.yaml or .clusterserviceversion.yaml files
      echo "[DEBUG] Replacing refs to internal registry in bundle image $bundleImg..." >&2
      for folder in manifests metadata; do
        for file in "./bundles/${digest}/unpacked/rootfs/${folder}"/*; do
          if [ -f "$file" ]; then
            echo "[DEBUG] replacing refs to internal registries in file '${file}'" >&2
            sed -i 's#registry.redhat.io/rhdh#quay.io/rhdh#g' "$file" >&2
            sed -i 's#registry.stage.redhat.io/rhdh#quay.io/rhdh#g' "$file" >&2
            sed -i 's#registry-proxy.engineering.redhat.com/rh-osbs/rhdh-#quay.io/rhdh/#g' "$file" >&2
          fi
        done
      done

      # repack the image with the changes
      echo "[DEBUG] Repacking image ./bundles/${digest}/src => ./bundles/${digest}/unpacked..." >&2
      umoci repack --image "./bundles/${digest}/src:latest" "./bundles/${digest}/unpacked" >&2

      # Push the bundle to the internal cluster registry
      echo "[DEBUG] Pushing updated image: ./bundles/${digest}/src => ${newBundleImage}..." >&2
      skopeo copy --dest-tls-verify=false "oci:./bundles/${digest}/src:latest" "docker://${newBundleImage}" >&2

      sed -i "s#${originalBundleImg}#${newBundleImageAsInt}#g" "${TMPDIR}/rhdh/rhdh/render.yaml" >&2
    fi
  done

  # 3. Regenerate the IIB image with the local changes to the render.yaml file and build and push it from within the cluster
  echo "[DEBUG] Regenerating IIB Dockerfile with updated refs..." >&2
  opm generate dockerfile rhdh/rhdh >&2

  echo "[DEBUG] Submitting in-cluster build request for the updated IIB..." >&2
  if ! oc -n rhdh get buildconfig.build.openshift.io/iib >& /dev/null; then
    oc -n rhdh new-build --strategy docker --binary --name iib >&2
    oc -n rhdh patch buildconfig.build.openshift.io/iib -p '{"spec": {"strategy": {"dockerStrategy": {"dockerfilePath": "rhdh.Dockerfile"}}}}' >&2
  fi
  oc -n rhdh start-build iib --wait --follow --from-dir=rhdh >&2
  local imageStreamWithTag="rhdh/iib:${IIB_TAG}"
  oc tag rhdh/iib:latest "${imageStreamWithTag}" >&2

  local result="${internal_registry_url}/${imageStreamWithTag}"
  echo "[DEBUG] IIB built and pushed to internal cluster registry: $result..." >&2
  printf "%s" "${result}"
}

pushd "${TMPDIR}"
echo ">>> WORKING DIR: $TMPDIR <<<"

# Defaulting to the hosted control plane behavior which has more chances to work
CONTROL_PLANE_TECH=$(oc get infrastructure cluster -o jsonpath='{.status.controlPlaneTopology}' || \
  (echo '[WARN] Could not determine the cluster type => defaulting to the hosted control plane behavior' >&2 && echo 'External'))
IS_HOSTED_CONTROL_PLANE="false"
if [[ "${CONTROL_PLANE_TECH}" == "External" ]]; then
  # 'External' indicates that the control plane is hosted externally to the cluster
  # and that its components are not visible within the cluster.
  IS_HOSTED_CONTROL_PLANE="true"
fi

newIIBImage=${IIB_IMAGE}
if [[ "${IS_HOSTED_CONTROL_PLANE}" = "true" ]]; then
  echo "[INFO] Detected a cluster with a hosted control plane"
  newIIBImage=$(install_hosted_control_plane_cluster)
else
  newIIBImage=$(install_regular_cluster)
fi

echo "[DEBUG] newIIBImage=${newIIBImage}"

echo "apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: ${CATALOGSOURCE_NAME}
  namespace: ${NAMESPACE_CATALOGSOURCE}
spec:
  sourceType: grpc
  image: ${newIIBImage}
  secrets:
  - internal-reg-auth-for-rhdh
  - internal-reg-ext-auth-for-rhdh
  publisher: IIB testing ${DISPLAY_NAME_SUFFIX}
  displayName: IIB testing catalog ${DISPLAY_NAME_SUFFIX}
" > "$TMPDIR"/CatalogSource.yml && oc apply -f "$TMPDIR"/CatalogSource.yml

if [ -z "$TO_INSTALL" ]; then
  echo "Done. Now log into the OCP web console as an admin, then go to Operators > OperatorHub, search for Red Hat Developer Hub, and install the Red Hat Developer Hub Operator."
  exit 0
fi

# Create OperatorGroup to allow installing all-namespaces operators in $NAMESPACE_SUBSCRIPTION
echo "Creating OperatorGroup to allow all-namespaces operators to be installed"
echo "apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: rhdh-operator-group
  namespace: ${NAMESPACE_SUBSCRIPTION}
" > "$TMPDIR"/OperatorGroup.yml && oc apply -f "$TMPDIR"/OperatorGroup.yml

# Create subscription for operator
echo "apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: $TO_INSTALL
  namespace: ${NAMESPACE_SUBSCRIPTION}
spec:
  channel: $OLM_CHANNEL
  installPlanApproval: Automatic
  name: $TO_INSTALL
  source: ${CATALOGSOURCE_NAME}
  sourceNamespace: ${NAMESPACE_CATALOGSOURCE}
" > "$TMPDIR"/Subscription.yml && oc apply -f "$TMPDIR"/Subscription.yml

OCP_CONSOLE_ROUTE_HOST=$(oc get route console -n openshift-console -o=jsonpath='{.spec.host}')
CLUSTER_ROUTER_BASE=$(oc get ingress.config.openshift.io/cluster '-o=jsonpath={.spec.domain}')
echo "

To install, go to:
https://${OCP_CONSOLE_ROUTE_HOST}/catalog/ns/${NAMESPACE_SUBSCRIPTION}?catalogType=OperatorBackedService

Or run this:

echo \"apiVersion: rhdh.redhat.com/v1alpha3
kind: Backstage
metadata:
  name: developer-hub
  namespace: ${NAMESPACE_SUBSCRIPTION}
spec:
  application:
    appConfig:
      mountPath: /opt/app-root/src
    extraFiles:
      mountPath: /opt/app-root/src
    replicas: 1
    route:
      enabled: true
  database:
    enableLocalDb: true
\" | oc apply -f-

Once deployed, Developer Hub will be available at
https://backstage-developer-hub-${NAMESPACE_SUBSCRIPTION}.${CLUSTER_ROUTER_BASE}
"
