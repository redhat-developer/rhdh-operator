#!/bin/bash
#
# Script to streamline installing the official RHDH Catalog Source in a disconnected OpenShift or Kubernetes cluster.
#
# Requires: oc (OCP) or kubectl (K8s), jq, yq, umoci, base64, opm, skopeo

set -euo pipefail

SCRIPT_PATH=$(realpath "$0")

NC='\033[0m'

IS_OPENSHIFT=""

NAMESPACE_SUBSCRIPTION="rhdh-operator"
NAMESPACE_OPERATOR="rhdh-operator"
OLM_CHANNEL="fast"
INDEX_IMAGE="registry.redhat.io/redhat/redhat-operator-index:v4.18"
FILTERED_VERSIONS=(1.4 1.5)

# assume mikefarah version of yq is already available on the path; if 1, then install the version shown
INSTALL_YQ=0 
YQ_VERSION=v4.45.1

function logf() {
  set -euo pipefail

  local prefix=$1
  local color=$2
  local msg=$3
  local fullMsg="[${prefix}] ${msg}"

  if [[ "$TERM" == *"color"* ]]; then
    echo -e "${color}${fullMsg}${NC}"
  else
    echo -e "${fullMsg}"
  fi
}

function infof() {
  logf "INFO" "${NC}" "$1"
}

function warnf() {
  logf "WARN" "\033[0;33m" "$1"
}

function debugf() {
  logf "DEBUG" "\033[0;90m" "$1"
}

function errorf() {
  logf "ERROR" "\033[0;31m" "$1"
}

function check_tool() {
  if ! command -v "$1" >/dev/null; then
      errorf "Error: Required tool '$1' is not installed."
      exit 1
  fi
}

function usage() {
  FILTERED_VERSIONS_CSV="${FILTERED_VERSIONS[*]}"; FILTERED_VERSIONS_CSV="${FILTERED_VERSIONS_CSV// /,}"
  echo "
This script streamlines the installation of the Red Hat Developer Hub Operator in a disconnected OpenShift or Kubernetes cluster.
It supports partially disconnected as well as fully disconnected environments.
In a partially disconnected environment, the host from which this script is executed has access to the Internet and the Red Hat ecosystem catalog,
and can push the images directly to the mirror registry and the cluster.
In a fully disconnected environment however, everything needs to be mirrored to disk first, then transferred to the
disconnected environment (usually via a bastion host), from where we can connect to the mirror registry and the cluster.

Usage:
  $0 [OPTIONS]

Options:
  --index-image <operator-index-image>   : Operator index image (default: $INDEX_IMAGE)
  --ci-index <true|false>                : Indicates that the index image is a CI build. Unsupported.
                                            Setting this to 'true' causes the script to replace all references to the internal RH registries
                                            with quay.io when mirroring images. Relevant only if '--use-oc-mirror' is 'false'. Default: false
  --filter-versions <list>               : Comma-separated list of operator minor versions to keep in the catalog (default: $FILTERED_VERSIONS_CSV).
                                            Specify '*' to disable version filtering and include all channels and all versions.
                                            Useful for CI index images for example.
  --to-registry <registry_url>           : Mirror the images into the specified registry, assuming you are already logged into it.
                                            If this is not set and --to-dir is not set, it will attempt to use the builtin OCP registry
                                            if the target cluster is OCP. Otherwise, it will error out.
                                            It also assumes you are logged into the target cluster as well.
  --to-dir </absolute/path/to/dir>       : Mirror images into the specified directory. Needs to be an absolute path.
                                            This is useful if you are working in a fully disconnected environment and
                                            you must manually transfer the images to your network.
                                            From there, you will be able to re-run this script with '--from-dir' to push
                                            the images to your private registry.
  --from-dir </absolute/path/to/dir>     : Load images from the specified directory. Needs to be an absolute path.
                                            This is useful if you are working in a fully disconnected environment.
                                            In this case, you would use '--to-dir' first to mirror images to a specified directory,
                                            then transfer this dir over to your disconnected network.
                                            From there, you will be able to re-run this script with '--from-dir' to push
                                            the images to your private registry.
  --install-operator <true|false>        : Install the RHDH operator right after creating the CatalogSource (default: true)
  --extra-images <list>                  : Comma-separated list of extra images to mirror
  --use-oc-mirror <true|false>           : Whether to use the 'oc-mirror' tool (default: false).
                                            This is the recommended way for mirroring on regular OpenShift clusters.
                                            Bear in mind however that this relies on resources like ImageContentSourcePolicy,
                                            which don't seem to work well on ROSA clusters or clusters with hosted control
                                            planes (like HyperShift or Red Hat OpenShift on IBM Cloud).
  --oc-mirror-path <path>                : Path to the oc-mirror binary (default: 'oc-mirror').
  --oc-mirror-flags <string>             : Additional flags to pass to all oc-mirror commands.
  --install-yq                           : Install yq $YQ_VERSION from https://github.com/mikefarah/yq (not the jq python wrapper)

Examples:

  # Install the Catalog Source by pushing the images to the internal OCP mirror registry,
  #   because it detected that it is connected to an OCP cluster.
  $0

  # Install the Catalog Source by pushing the images to the specified mirror registry, assuming the user is already logged into it.
  $0 \\
    --to-registry registry.example.com

  # Extract all the images needed into the specified directory.
  $0 \\
    --to-dir  /path/to/my/dir

  # From a bastion host connected to the disconnected network,
  # install the operator by using the images from the specified directory.
  $0 \\
    --from-dir  /path/to/my/dir \\
    --to-registry registry.example.com

  # Install the Catalog Source from a CI index image by pushing the images to the internal OCP mirror registry,
  #   because it detected that it is connected to an OCP cluster.
  # It will automatically replace all references to the internal RH registries with quay.io
  $0 \\
    --ci-index true
"
}

OPERATOR_NAME="rhdh-operator"
IS_CI_INDEX_IMAGE="false"

TO_REGISTRY=""
INSTALL_OPERATOR="true"
TO_DIR=""
FROM_DIR=""
EXTRA_IMAGES=()
USE_OC_MIRROR="false"
OC_MIRROR_PATH="oc-mirror"
OC_MIRROR_FLAGS=""

NO_VERSION_FILTER="false"
RELATED_IMAGES=()

# example usage:
# ./prepare-restricted-environment.sh \
# [ --filter-versions "1.a,1.b" ]
# --from-dir /path/to/dir (to support mirroring from a bastion host)
# --to-dir /path/to/dir (to support exporting images to a dir, which can be transferred to the bastion host)
# --to-registry "$MY_MIRROR_REGISTRY" (either this or to-dir needs to specified, both can be specified)
# --install-operator "true"
# --use-oc-mirror "false"

while [[ "$#" -gt 0 ]]; do
  case $1 in
    # Legacy options. Deprecated but kept for backward compatibility
    '--prod_operator_index') INDEX_IMAGE="$2"; shift 1;;
    '--prod_operator_package_name') debugf "--prod_operator_package_name ($2) is no longer used"; shift 1;;
    '--prod_operator_bundle_name') debugf "--prod_operator_bundle_name ($2) is no longer used"; shift 1;;
    '--helper_mirror_registry_storage')
      debugf "--helper_mirror_registry_storage is no longer used. This script assumes you already have a mirror registry in your disconnected environment";
      shift 1;;
    '--use_existing_mirror_registry')
      debugf "--use_existing_mirror_registry is no longer used. This script assumes you already have a mirror registry in your disconnected environment";
      shift 1;;
    '--prod_operator_version')
      input="${2#v}"
      IFS='.' read -ra parts <<< "$input"
      length=${#parts[@]}
      if [ $length -ge 2 ]; then
       FILTERED_VERSIONS=(${parts[0]}.${parts[1]})
      else
       FILTERED_VERSIONS=(${parts[*]})
      fi
      debugf "FILTERED_VERSIONS=${FILTERED_VERSIONS[@]}"
      shift 1;;

    # New options
    '--index-image') INDEX_IMAGE="$2"; shift 1;;
    '--ci-index') IS_CI_INDEX_IMAGE="$2"; shift 1;;
    '--filter-versions')
      if [[ "$2" == "*" ]]; then
        NO_VERSION_FILTER="true"
      else
        IFS=',' read -r -a FILTERED_VERSIONS <<< "$2"
      fi
      shift 1;;
    '--extra-images') IFS=',' read -r -a EXTRA_IMAGES <<< "$2"; shift 1;;
    '--to-registry') TO_REGISTRY="$2"; shift 1;;
    '--to-dir') TO_DIR=$(realpath "$2"); shift 1;;
    '--from-dir') FROM_DIR="$2"; shift 1;;
    '--install-operator') INSTALL_OPERATOR="$2"; shift 1;;
    '--use-oc-mirror') USE_OC_MIRROR="$2"; shift 1;;
    '--oc-mirror-path') OC_MIRROR_PATH="$2"; shift 1;;
    '--oc-mirror-flags') OC_MIRROR_FLAGS="$2"; shift 1;;
    '--install-yq') INSTALL_YQ=1;;
    '-h'|'--help') usage; exit 0;;
    *) errorf "Unknown parameter is used: $1."; usage; exit 1;;
  esac
  shift 1
done

function is_openshift() {
  set -euo pipefail

  oc get routes.route.openshift.io &> /dev/null || kubectl get routes.route.openshift.io &> /dev/null
}

function detect_ocp_and_set_env_var() {
  set -euo pipefail

  if [[ "${IS_OPENSHIFT}" = "" ]]; then
    IS_OPENSHIFT=$(is_openshift && echo 'true' || echo 'false')
  fi
}

# Wrapper function to call kubectl or oc
function invoke_cluster_cli() {
  set -euo pipefail

  local command=$1
  shift

  detect_ocp_and_set_env_var
  if [[ "${IS_OPENSHIFT}" = "true" ]]; then
    if command -v oc &> /dev/null; then
      oc "$command" "$@"
    else
      kubectl "$command" "$@"
    fi
  else
    kubectl "$command" "$@"
  fi
}

##########################################################################################
# Script start
##########################################################################################

# Pre-checks
if [[ "${USE_OC_MIRROR}" = "true" ]]; then
  if [[ "${OC_MIRROR_PATH}" == "oc-mirror" ]]; then
    check_tool "oc-mirror"
  else
    if [ ! -f "${OC_MIRROR_PATH}" ] || [ ! -x "${OC_MIRROR_PATH}" ]; then
      if ! command -v "$1" >/dev/null; then
        errorf "oc-mirror binary not found or not executable: ${OC_MIRROR_PATH}"
        exit 1
      fi
    fi
    debugf "Using oc-mirror path: ${OC_MIRROR_PATH}"
  fi
fi

if [[ -n "${FROM_DIR}" && -n "${TO_DIR}" ]]; then
  errorf "--from-dir and --to-dir are mutually exclusive. Please specify only one of them."
  exit 1
fi
if [[ -n "${TO_REGISTRY}" && -n "${TO_DIR}" ]]; then
  errorf "--to-registry and --to-dir are mutually exclusive. Please specify only one of them."
  exit 1
fi
if [[ -z "${TO_REGISTRY}" && -z "${TO_DIR}" ]]; then
  # If we know that we are connected to OCP, let's use the internal OCP registry
  isOcp=$(is_openshift && echo 'true' || echo 'false')
  if [[ "$isOcp" != "true" ]]; then
    if [[ -n "${FROM_DIR}" ]]; then
      errorf "--to-registry is needed when --from-dir is specified."
    else
      errorf "Please specify either --to-registry or --to-dir (not both). Or log into your OCP cluster to automatically use its integrated image registry."
    fi
    exit 1
  fi
  debugf "--to-registry not specified but detected an OCP cluster => will try to use the internal OCP cluster registry as mirror registry"
  TO_REGISTRY="OCP_INTERNAL"
fi
if [[ -n "${FROM_DIR}" && -z "${TO_REGISTRY}" ]]; then
  errorf "--to-registry is needed when --from-dir is specified."
  exit 1
fi

if [[ -n "${TO_DIR}" ]]; then
  mkdir -p "${TO_DIR}"
  TMPDIR="${TO_DIR}"
else
  TMPDIR=$(mktemp -d)
  ## shellcheck disable=SC2064
  trap "rm -fr $TMPDIR || true" EXIT
fi
pushd "${TMPDIR}" > /dev/null
debugf ">>> WORKING DIR: $TMPDIR <<<"

if [[ $INSTALL_YQ ]]; then
  YQ=$HOME/.local/bin/yq_mf
  YQ_BINARY=yq_linux_amd64
  curl -sSLo- https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/${YQ_BINARY}.tar.gz | tar xz && mv -f ${YQ_BINARY} "${YQ}"
  debugf "mikefarah yq $YQ_VERSION installed to $YQ"
else
  YQ=$(which yq)
fi

function merge_registry_auth() {
  set -euo pipefail

  currentRegistryAuthFile="${REGISTRY_AUTH_FILE:-${XDG_RUNTIME_DIR:-/run/user/$(id -u)}/containers/auth.json}"
  debugf "currentRegistryAuthFile: $currentRegistryAuthFile"
  if [ ! -f "${currentRegistryAuthFile}" ]; then
    debugf "Missing registry auth file. Will proceed without any existing auth against the registry hosting the index image: $INDEX_IMAGE"
    return
  fi
  # TODO(rm3l): Overriding XDG_RUNTIME_DIR so it can work with "oc-mirror v1", which does not work with REGISTRY_AUTH_FILE.
  # Remove this when oc-mirror v2 is out of TP.
  export XDG_RUNTIME_DIR=$(mktemp -d)
  mkdir -p "${XDG_RUNTIME_DIR}/containers"
  ## shellcheck disable=SC2064
  trap "rm -fr $XDG_RUNTIME_DIR || true" EXIT
  # Using the current working dir, otherwise tools like 'skopeo login' will attempt to write to /run, which
  # might be restricted in CI environments.
  # This also ensures that the credentials don't conflict with any existing creds for the same registry
  export REGISTRY_AUTH_FILE="${XDG_RUNTIME_DIR}/containers/auth.json"
  debugf "REGISTRY_AUTH_FILE: $REGISTRY_AUTH_FILE"

  # Merge existing authentication from currentRegistryAuthFile into REGISTRY_AUTH_FILE
  images=("${INDEX_IMAGE}")
  if [[ -n "${TO_REGISTRY}" ]]; then
    images+=("$(buildRegistryUrl)")
  fi
  registries=("registry.redhat.io" "quay.io")
  for img in "${images[@]}"; do
    reg=$(echo "$img" | cut -d'/' -f1)
    [[ " ${registries[*]} " =~ " $reg " ]] || registries+=("$reg")
  done
  tmpFile=$(mktemp)
  ## shellcheck disable=SC2064
  trap "rm -f $tmpFile || true" EXIT
  echo '{"auths": {' > "$tmpFile"
  for reg in "${registries[@]}"; do
    echo "  \"$reg\": .auths.\"$reg\"," >> "$tmpFile"
  done
  sed -i '$ s/,$//' "$tmpFile"
  echo '}}' >> "$tmpFile"
  debugf "yq filter: $(cat "$tmpFile")"
  "$YQ" -o=json "$(cat "$tmpFile")" "${currentRegistryAuthFile}" > "${REGISTRY_AUTH_FILE}"
}

function ocp_prepare_internal_registry() {
  set -euo pipefail

  debugf "Exposing cluster registry..." >&2
  internal_registry_url="image-registry.openshift-image-registry.svc:5000"
  oc patch configs.imageregistry.operator.openshift.io/cluster --patch '{"spec":{"defaultRoute":true}}' --type=merge >&2
  # https://access.redhat.com/solutions/6022011
  oc patch configs.imageregistry.operator.openshift.io/cluster --patch '{"spec":{"disableRedirect":true}}' --type=merge >&2
  my_registry=$(oc get route default-route -n openshift-image-registry --template='{{ .spec.host }}')
  skopeo login -u kubeadmin -p "$(oc whoami -t)" --tls-verify=false "$my_registry" >&2
  podman login -u kubeadmin -p "$(oc whoami -t)" --tls-verify=false "$my_registry" >&2
  for ns in rhdh-operator openshift4 rhdh rhel9 oc-mirror; do
    # To be able to push images under this scope in the internal image registry
    if ! oc get namespace "$ns" &> /dev/null; then
      oc create namespace "$ns" >&2
    fi
    oc adm policy add-cluster-role-to-user system:image-signer system:serviceaccount:${ns}:default >&2 || true
  done
  for ns in rhdh-operator openshift-marketplace; do
    if oc -n ${ns} get secret internal-reg-ext-auth-for-rhdh &> /dev/null; then
      oc -n ${ns} delete secret internal-reg-ext-auth-for-rhdh >&2
    fi
    oc -n ${ns} create secret docker-registry internal-reg-ext-auth-for-rhdh \
      --docker-server="${my_registry}" \
      --docker-username=kubeadmin \
      --docker-password="$(oc whoami -t)" \
      --docker-email="admin@internal-registry-ext.example.com" >&2
    if oc -n ${ns} get secret internal-reg-auth-for-rhdh &> /dev/null; then
      oc -n ${ns} delete secret internal-reg-auth-for-rhdh >&2
    fi
    oc -n ${ns} create secret docker-registry internal-reg-auth-for-rhdh \
      --docker-server="${internal_registry_url}" \
      --docker-username=kubeadmin \
      --docker-password="$(oc whoami -t)" \
      --docker-email="admin@internal-registry.example.com" >&2
    oc adm policy add-cluster-role-to-user system:image-signer system:serviceaccount:${ns}:default >&2 || true
  done
  oc policy add-role-to-user system:image-puller system:serviceaccount:openshift-marketplace:default -n openshift-marketplace >&2 || true
  oc policy add-role-to-user system:image-puller system:serviceaccount:rhdh-operator:default -n rhdh-operator >&2 || true
  oc policy add-role-to-user system:image-puller system:serviceaccount:rhdh-operator:rhdh-operator -n rhdh-operator >&2 || true
}

function buildRegistryUrl() {
  set -euo pipefail

  detect_ocp_and_set_env_var
  local input="${1:-external}"
  if [[ "${IS_OPENSHIFT}" = "true" && "${TO_REGISTRY}" = "OCP_INTERNAL" ]]; then
    if [[ "${input}" == "internal" ]]; then
      echo "image-registry.openshift-image-registry.svc:5000"
    else
      echo $(oc get route default-route -n openshift-image-registry --template='{{ .spec.host }}')
    fi
  else
    echo "${TO_REGISTRY}"
  fi
}

function buildCatalogImageUrl() {
  if [[ -n "$TO_REGISTRY" ]]; then
    tag=${INDEX_IMAGE##*:}; [[ "$INDEX_IMAGE" == "$tag" ]] && tag="latest"
    echo "$(buildRegistryUrl "${1:-external}")/${2:-rhdh/index}:${tag}"
  else
    echo ""
  fi
}

function render_index() {
  set -euo pipefail

  mkdir -p "${TMPDIR}/rhdh/rhdh"
  local_index_file="${TMPDIR}/rhdh/rhdh/render.yaml"

  debugf "Rendering index image $INDEX_IMAGE as a local file: $local_index_file..."

  prod_operator_package_name="rhdh"
  prod_operator_name="${prod_operator_package_name}-operator"
  debugf "Fetching metadata for the ${prod_operator_package_name} operator catalog channel, packages, and bundles."

  # Filtering out to keep only the elements related to RHDH and to the versions selected
  if [[ "${NO_VERSION_FILTER}" == "true" ]]; then
    opm render "${INDEX_IMAGE}" --output=yaml > "${local_index_file}"
  else
    chanFilterList=""
    bundleFilterList=""
    chanEntriesFilterList=""
    for v in "${FILTERED_VERSIONS[@]}"; do
      chanFilterList+='(.schema == "olm.channel" and .package == "'${prod_operator_package_name}'" and .name == "fast-'$v'") or '
      bundleFilterList+='(.schema == "olm.bundle" and .name == "'$prod_operator_name'.v'$v'*") or '
      chanEntriesFilterList+='.name == "'$prod_operator_name'.v'$v'*" or '
    done
    chanFilterList="${chanFilterList%"or "}"
    bundleFilterList="${bundleFilterList%"or "}"
    chanEntriesFilterList="${chanEntriesFilterList%"or "}"
    debugf "chanFilterList=$chanFilterList"
    debugf "chanEntriesFilterList=$chanEntriesFilterList"
    debugf "bundleFilterList=$bundleFilterList"

    opm render "${INDEX_IMAGE}" --output=yaml | \
      "$YQ" 'select(
          (.schema == "olm.package" and .name == "'${prod_operator_package_name}'")
          or
          (.schema == "olm.channel" and .package == "'${prod_operator_package_name}'" and .name == "fast")
          or
          '"$chanFilterList"'
          or
          '"$bundleFilterList"'
        )' | "$YQ" '.entries |= map(select('"$chanEntriesFilterList"'))' \
        > "${local_index_file}"
  fi

  debugf "Got $(cat "${local_index_file}" | wc -l) lines of JSON from the index!"

  if [ ! -s "${local_index_file}" ]; then
    errorf "[ERROR] 'opm render $INDEX_IMAGE' returned an empty output, which likely means that this index Image does not contain the rhdh operator."
    return 1
  fi
}

function extract_last_two_elements() {
  local input="$1"
  local IFS='/'

  read -ra parts <<< "$input"

  local length=${#parts[@]}
  if [ $length -ge 2 ]; then
   echo "${parts[-2]}/${parts[-1]}"
  else
   echo "${parts[*]}"
  fi
}

function mirror_extra_images() {
  debugf "Extra images: ${EXTRA_IMAGES[@]}..."
  for img in "${EXTRA_IMAGES[@]}"; do
    if [[ "$img" == *"@sha256:"* ]]; then
      imgDigest="${img##*@sha256:}"
      imgDir="./extraImages/${img%@*}/sha256_$imgDigest"
      lastTwo=$(extract_last_two_elements "${img%@*}")
      targetImg="$(buildRegistryUrl)/${lastTwo}:$imgDigest"
    elif [[ "$img" == *":"* ]]; then
      imgDir="./extraImages/${img%:*}/tag_$imgTag"
      imgTag="${img##*:}"
      lastTwo=$(extract_last_two_elements "${img%:*}")
      targetImg="$(buildRegistryUrl)/${lastTwo}:$imgTag"
    else
      imgDir="./extraImages/${img}/tag_latest"
      lastTwo=$(extract_last_two_elements "${img}")
      targetImg="$(buildRegistryUrl)/${lastTwo}:latest"
    fi

    if [[ -n "$TO_REGISTRY" ]]; then
      if [[ "${IS_OPENSHIFT}" = "true" && "${TO_REGISTRY}" = "OCP_INTERNAL" ]]; then
        # Create the corresponding project if it doesn't exist
        projectNameForOcpReg=${lastTwo%%/*}
        oc get namespace "${projectNameForOcpReg}" &>/dev/null || oc create namespace "${projectNameForOcpReg}"
      fi
      mirror_image_to_registry "$img" "$targetImg"
    else
      if [ ! -d "$imgDir" ]; then
        mkdir -p "${imgDir}"
        mirror_image_to_archive "$img" "$imgDir"
      fi
    fi
  done
  debugf "... done."
}

function mirror_extra_images_from_dir() {
  BASE_DIR="${FROM_DIR}/extraImages"
  debugf "Extra images from ${BASE_DIR}..."
  if [ -d "${BASE_DIR}" ]; then
    # Iterate over all directories named "sha256_*"
    find "$BASE_DIR" -type d -name "sha256_*" | while read -r sha256_dir; do
      relative_path=${sha256_dir#"$BASE_DIR/"}
      sha256_hash=${sha256_dir##*/sha256_}
      parent_path=$(dirname "$relative_path")
      debugf "parent_path: $parent_path"
      lastTwo=$(extract_last_two_elements "${parent_path}")
      extraImg="${lastTwo}:${sha256_hash}"
      debugf "Extra-image: $extraImg"
      if [[ -n "$TO_REGISTRY" ]]; then
        if [[ "${IS_OPENSHIFT}" = "true" && "${TO_REGISTRY}" = "OCP_INTERNAL" ]]; then
          # Create the corresponding project if it doesn't exist
          projectNameForOcpReg=${lastTwo%%/*}
          oc get namespace "${projectNameForOcpReg}" &>/dev/null || oc create namespace "${projectNameForOcpReg}"
        fi
        targetImg="$(buildRegistryUrl)/${extraImg%@*}"
        push_image_from_archive "$sha256_dir" "$targetImg"
      fi
    done

    # Iterate over all directories named "tag_*"
    find "$BASE_DIR" -type d -name "tag_*" | while read -r tag_dir; do
      relative_path=${tag_dir#"$BASE_DIR/"}
      tag_hash=${tag_dir##*/tag_}
      parent_path=$(dirname "$relative_path")
      debugf "parent_path: $parent_path"
      lastTwo=$(extract_last_two_elements "${parent_path}")
      extraImg="${lastTwo}:${tag_hash}"
      debugf "Extra-image: $extraImg"
      if [[ -n "$TO_REGISTRY" ]]; then
        if [[ "${IS_OPENSHIFT}" = "true" && "${TO_REGISTRY}" = "OCP_INTERNAL" ]]; then
          # Create the corresponding project if it doesn't exist
          projectNameForOcpReg=${lastTwo%%/*}
          oc get namespace "${projectNameForOcpReg}" &>/dev/null || oc create namespace "${projectNameForOcpReg}"
        fi
        targetImg="$(buildRegistryUrl)/${extraImg%:*}"
        push_image_from_archive "$tag_dir" "$targetImg"
      fi
    done
  fi
}

function replaceInternalRegIfNeeded() {
  img="$1"
  if [[ "${IS_CI_INDEX_IMAGE}" == "true" ]]; then
    replacement="${2:-quay.io}"

    for reg in registry.stage.redhat.io registry.redhat.io; do
      img="${img/$reg\/rhdh/$replacement\/rhdh}"
    done
    img="${img/registry-proxy.engineering.redhat.com\/rh-osbs\/rhdh-/$replacement\/rhdh\/}"
  fi
  echo "$img"
}

function process_bundles() {

  for bundleImg in $(grep -E '^image: .*operator-bundle' "${TMPDIR}/rhdh/rhdh/render.yaml" | awk '{print $2}' | uniq); do
    debugf "bundleImg=$bundleImg"
    originalBundleImg="$bundleImg"
    bundleImg=$(replaceInternalRegIfNeeded "$bundleImg")
    digest="${bundleImg##*@sha256:}"
    if skopeo inspect "docker://$bundleImg" &> /dev/null; then
      mkdir -p "bundles/$digest"
      debugf "\t copying and unpacking image $bundleImg locally..."
      if [ ! -d "./bundles/${digest}/src" ]; then
        skopeo copy --remove-signatures "docker://$bundleImg" "oci:./bundles/${digest}/src:latest"
      fi
      if [ ! -d "./bundles/${digest}/unpacked" ]; then
        umoci unpack --image "./bundles/${digest}/src:latest" "./bundles/${digest}/unpacked" --rootless
      fi

      debugf "\t inspecting related images referenced in bundle image $bundleImg..."
      for file in "./bundles/${digest}/unpacked/rootfs/manifests"/*; do
        if [[ "$file" == *.clusterserviceversion.yaml || "$file" == *.csv.yaml ]]; then
          debugf "\t Adding imagePullSecrets to the CSV file so we can pull from private registries"
          "$YQ" eval '
            (.spec.install.spec.deployments[] | select(.name == "rhdh-operator").spec.template.spec) +=
              {"imagePullSecrets": [{"name": "internal-reg-auth-for-rhdh"},{"name": "internal-reg-ext-auth-for-rhdh"},{"name": "reg-pull-secret"}]}
          ' -i "$file"

          all_related_images=()
          debugf "\t finding related images in $file to mirror..."
          images=$(grep -E 'image: ' "$file" | awk -F ': ' '{print $2}' | uniq)
          if [[ -n "$images" ]]; then
            all_related_images+=($images)
          fi
          # TODO(rm3l): we should use spec.relatedImages instead, but it seems to be incomplete in some bundles
          related_images=$("$YQ" '.spec.install.spec.deployments[].spec.template.spec.containers[].env[] | select(.name | test("^RELATED_IMAGE_")).value' "$file" || true)
          if [[ -n "$related_images" ]]; then
            all_related_images+=($related_images)
          fi
          for relatedImage in "${all_related_images[@]}"; do
            imgDir="./images/"
            if [[ "$relatedImage" == *"@sha256:"* ]]; then
              relatedImageDigest="${relatedImage##*@sha256:}"
              imgDir+="${relatedImage%@*}/sha256_$relatedImageDigest"
              lastTwo=$(extract_last_two_elements "${relatedImage%@*}")
              targetImg="$(buildRegistryUrl)/${lastTwo}:$relatedImageDigest"
              internalTargetImg="$(buildRegistryUrl "internal")/${lastTwo}:$relatedImageDigest"
            elif [[ "$relatedImage" == *":"* ]]; then
              relatedImageTag="${relatedImage##*:}"
              imgDir+="${relatedImage%:*}/tag_$relatedImageTag"
              lastTwo=$(extract_last_two_elements "${relatedImage%:*}")
              targetImg="$(buildRegistryUrl)/${lastTwo}:$relatedImageTag"
              internalTargetImg="$(buildRegistryUrl "internal")/${lastTwo}:$relatedImageTag"
            else
              imgDir+="${relatedImage}/tag_latest"
              lastTwo=$(extract_last_two_elements "${relatedImage}")
              targetImg="$(buildRegistryUrl)/${lastTwo}:latest"
              internalTargetImg="$(buildRegistryUrl "internal")/${lastTwo}:latest"
            fi

            if [[ -n "$TO_REGISTRY" ]]; then
              mirror_image_to_registry "$relatedImage" "$targetImg"
              debugf "replacing $relatedImage in file '${file}' => $internalTargetImg"
              sed -i 's#'$relatedImage'#'$internalTargetImg'#g' "$file"
            else
              if [ ! -d "$imgDir" ]; then
                mkdir -p "${imgDir}"
                mirror_image_to_archive "$relatedImage" "$imgDir"
              fi
            fi
          done
        fi
      done

      if [[ -n "$TO_REGISTRY" ]]; then
          # repack the image with the changes
          debugf "\t Repacking image ./bundles/${digest}/src => ./bundles/${digest}/unpacked..."
          umoci repack --image "./bundles/${digest}/src:latest" "./bundles/${digest}/unpacked"

          # Push the bundle to the mirror registry
          newBundleImage="$(buildRegistryUrl)/$(extract_last_two_elements "${bundleImg%@*}"):${digest}"
          newBundleImageInternal="$(buildRegistryUrl "internal")/$(extract_last_two_elements "${bundleImg%@*}"):${digest}"
          debugf "\t Pushing updated bundle image: ./bundles/${digest}/src => ${newBundleImage}..."
          skopeo copy --remove-signatures --dest-tls-verify=false "oci:./bundles/${digest}/src:latest" "docker://${newBundleImage}"

          sed -i "s#${originalBundleImg}#${newBundleImageInternal}#g" "./rhdh/rhdh/render.yaml"
      fi
    fi
  done

  if [ ! -f "rhdh/rhdh.Dockerfile" ]; then
    debugf "\t Regenerating Dockerfile so the index can be rebuilt..."
    opm generate dockerfile rhdh/rhdh

    if [[ -n "$TO_REGISTRY" ]]; then
      infof "Building the catalog image locally."
      pushd "rhdh"
      my_operator_index="$(buildCatalogImageUrl)"
      podman build -t "$my_operator_index" -f "./rhdh.Dockerfile" --no-cache .

      infof "Deploying your catalog image to the $my_operator_index registry."
      skopeo copy --remove-signatures --src-tls-verify=false --dest-tls-verify=false --all "containers-storage:$my_operator_index" "docker://$my_operator_index"
      popd
    fi
  fi
}

function process_bundles_from_dir() {

  if [ ! -f "${FROM_DIR}/rhdh/rhdh.Dockerfile" ]; then
    errorf "Missing ${FROM_DIR}/rhdh/rhdh.Dockerfile file. I don't known how to rebuild the index image."
    return 1
  fi

  for d in bundles rhdh; do
    cp -r "${FROM_DIR}/${d}" "${TMPDIR}/${d}"
  done

  for bundleImg in $(grep -E '^image: .*operator-bundle' "${FROM_DIR}/rhdh/rhdh/render.yaml" | awk '{print $2}' | uniq); do
    debugf "bundleImg=$bundleImg"
    digest="${bundleImg##*@sha256:}"
    if [ ! -d "${FROM_DIR}/bundles/${digest}/src" ]; then
      warnf "missing src image for bundle digest: ${FROM_DIR}/bundles/${digest}/src"
      continue
    fi
    if [ ! -d "${FROM_DIR}/bundles/${digest}/unpacked" ]; then
      warnf "missing unpacked image for bundle digest: ${FROM_DIR}/bundles/${digest}/unpacked"
      continue
    fi

    debugf "Handling bundle image from ${FROM_DIR}/bundles/${digest}..."

    debugf "\t inspecting related images referenced in bundle image $bundleImg..."
    for file in "${TMPDIR}/bundles/${digest}/unpacked/rootfs/manifests"/*; do
      if [[ "$file" == *.clusterserviceversion.yaml || "$file" == *.csv.yaml ]]; then
        all_related_images=()
        debugf "\t finding related images in $file to mirror..."
        images=$(grep -E 'image: ' "$file" | awk -F ': ' '{print $2}' | uniq)
        if [[ -n "$images" ]]; then
          all_related_images+=($images)
        fi
        # TODO(rm3l): we should use spec.relatedImages instead, but it seems to be incomplete in some bundles
        related_images=$("$YQ" '.spec.install.spec.deployments[].spec.template.spec.containers[].env[] | select(.name | test("^RELATED_IMAGE_")).value' "$file" || true)
        if [[ -n "$related_images" ]]; then
          all_related_images+=($related_images)
        fi
        for relatedImage in "${all_related_images[@]}"; do
          imgDir="${FROM_DIR}/images/"
          if [[ "$relatedImage" == *"@sha256:"* ]]; then
            relatedImageDigest="${relatedImage##*@sha256:}"
            imgDir+="${relatedImage%@*}/sha256_$relatedImageDigest"
            targetImg="$(buildRegistryUrl)/$(extract_last_two_elements "${relatedImage%@*}"):$relatedImageDigest"
            targetImgInternal="$(buildRegistryUrl "internal")/$(extract_last_two_elements "${relatedImage%@*}"):$relatedImageDigest"
          elif [[ "$relatedImage" == *":"* ]]; then
            relatedImageTag="${relatedImage##*:}"
            imgDir+="${relatedImage%:*}/tag_$relatedImageTag"
            targetImg="$(buildRegistryUrl)/$(extract_last_two_elements "${relatedImage%:*}"):$relatedImageTag"
            targetImgInternal="$(buildRegistryUrl "internal")/$(extract_last_two_elements "${relatedImage%:*}"):$relatedImageTag"
          else
            imgDir+="${relatedImage}/tag_latest"
            targetImg="$(buildRegistryUrl)/$(extract_last_two_elements "${relatedImage}"):latest"
            targetImgInternal="$(buildRegistryUrl "internal")/$(extract_last_two_elements "${relatedImage}"):latest"
          fi
          if [ ! -d "$imgDir" ]; then
            warnf "Skipping related image $relatedImage not found mirrored in dir: $FROM_DIR/images"
            continue
          fi
          if [[ -n "$TO_REGISTRY" ]]; then
            push_image_from_archive "$imgDir" "$targetImg"
            debugf "replacing $relatedImage in file '${file}' => $targetImgInternal"
            sed -i 's#'$relatedImage'#'$targetImgInternal'#g' "$file"
          fi
        done
      fi
    done

    if [[ -n "$TO_REGISTRY" ]]; then
        # repack the image with the changes
        debugf "\t Repacking image ./bundles/${digest}/src => ./bundles/${digest}/unpacked..."
        umoci repack --image "${TMPDIR}/bundles/${digest}/src:latest" "${TMPDIR}/bundles/${digest}/unpacked"

        # Push the bundle to the mirror registry
        newBundleImage="$(buildRegistryUrl)/$(extract_last_two_elements "${bundleImg%@*}"):${digest}"
        newBundleImageInternal="$(buildRegistryUrl "internal")/$(extract_last_two_elements "${bundleImg%@*}"):${digest}"
        debugf "\t Pushing updated bundle image: ./bundles/${digest}/src => ${newBundleImage}..."
        skopeo copy --preserve-digests --remove-signatures --dest-tls-verify=false "oci:${TMPDIR}/bundles/${digest}/src:latest" "docker://${newBundleImage}"

        sed -i "s#${bundleImg}#${newBundleImageInternal}#g" "${TMPDIR}/rhdh/rhdh/render.yaml"
    fi
  done

  if [[ -n "$TO_REGISTRY" ]]; then
    pushd "${TMPDIR}/rhdh"
    my_operator_index="$(buildCatalogImageUrl)"
    debugf "Building the catalog image locally: $my_operator_index"
    podman build -t "$my_operator_index" -f "./rhdh.Dockerfile" --no-cache .

    debugf "Deploying your catalog image to the $my_operator_index registry."
    skopeo copy --preserve-digests --remove-signatures --src-tls-verify=false --dest-tls-verify=false --all "containers-storage:$my_operator_index" "docker://$my_operator_index"
    popd
  fi
}

function mirror_image_to_registry() {
  local src_image
  src_image=$(replaceInternalRegIfNeeded "$1")
  local dest_image
  dest_image=$2
  echo "Mirroring $src_image to $dest_image..."
  skopeo copy --preserve-digests --remove-signatures --all --dest-tls-verify=false docker://"$src_image" docker://"$dest_image"
}

function mirror_image_to_archive() {
  local src_image
  dest_image=$(replaceInternalRegIfNeeded "$1")
  local archive_path
  archive_path=$2
  debugf "Saving $src_image to $archive_path..."
  skopeo copy --preserve-digests --remove-signatures --all --preserve-digests --dest-tls-verify=false docker://"$src_image" dir:"$archive_path"
}

function push_image_from_archive() {
  local archive_path=$1
  local dest_image=$2
  echo "Pushing $archive_path to $dest_image..."
  skopeo copy --preserve-digests --remove-signatures --all --dest-tls-verify=false dir:"$archive_path" docker://"$dest_image"
}

check_tool "yq"
check_tool "umoci"
check_tool "skopeo"
if [[ -n "$TO_REGISTRY" ]]; then
  check_tool "podman"
fi

if [[ -n "${TO_DIR}" ]]; then
  cp -f "${SCRIPT_PATH}" "${TO_DIR}/install.sh"
fi

detect_ocp_and_set_env_var
if [[ "${IS_OPENSHIFT}" = "true" && "${TO_REGISTRY}" = "OCP_INTERNAL" ]]; then
  ocp_prepare_internal_registry
fi

merge_registry_auth

manifestsTargetDir="${TMPDIR}"
if [[ -n "${FROM_DIR}" ]]; then
  manifestsTargetDir="${FROM_DIR}"
fi

if [[ "${USE_OC_MIRROR}" = "true" ]]; then
  # TODO(rm3l): oc-mirror v1 always loads the docker creds first:
  # https://github.com/openshift/oc-mirror/blob/main/pkg/image/credentials.go
  # But we want to use our own credentials file, which is not possible until oc-mirror v2 (currently tech preview)
  if [ -f ~/.docker/config.json ]; then
    debugf "Temporarily moving ~/.docker/config.json to ~/.docker/config.json.bak, so as to work with oc-mirror v1"
    mv -f ~/.docker/config.json ~/.docker/config.json.bak || true
    trap "mv -f ~/.docker/config.json.bak ~/.docker/config.json || true" EXIT
  fi

  NAMESPACE_CATALOGSOURCE="openshift-marketplace"
  ocMirrorLogFile="${TMPDIR}/oc-mirror.log.txt"
  if [[ -z "${FROM_DIR}" ]]; then
    # Direct to registry
    cat <<EOF > "${TMPDIR}/imageset-config.yaml"
apiVersion: mirror.openshift.io/v1alpha2
kind: ImageSetConfiguration
storageConfig:
  local:
    # Do not delete or modify metadata generated by the oc-mirror plugin,
    # use the same storage backend every time run the oc-mirror plugin for the same mirror
    path: ./metadata
mirror:
  operators:
  - catalog: ${INDEX_IMAGE}
    full: false
    targetCatalog: rhdh-catalog
    packages:
      - name: rhdh
EOF
    if [[ "${NO_VERSION_FILTER}" != "true" ]]; then
      cat <<EOF >> "${TMPDIR}/imageset-config.yaml"
        channels:
          - name: fast
EOF
      for v in "${FILTERED_VERSIONS[@]}"; do
        cat <<EOF >> "${TMPDIR}/imageset-config.yaml"
          - name: fast-${v}
EOF
      done

    fi
    nbExtraImgs=${#EXTRA_IMAGES[@]}
    if [ $nbExtraImgs -ge 1 ]; then
      cat <<EOF >> "${TMPDIR}/imageset-config.yaml"
      additionalImages:
EOF
      for extraImg in "${EXTRA_IMAGES[@]}"; do
        cat <<EOF >> "${TMPDIR}/imageset-config.yaml"
      - name: "$extraImg"
EOF
      done
    fi

    if [[ -n "${TO_DIR}" ]]; then
      "${OC_MIRROR_PATH}" \
        --config="${TMPDIR}/imageset-config.yaml" \
        file://"${TO_DIR}" \
        --skip-missing \
        --dest-skip-tls \
        --continue-on-error \
        --max-nested-paths=1 \
        $OC_MIRROR_FLAGS \
        | tee "${ocMirrorLogFile}"
      if [[ "${TO_DIR}" != "${TMPDIR}" ]]; then
        cp -f "${TMPDIR}/imageset-config.yaml" "${TO_DIR}/imageset-config.yaml"
      fi
      # targetCatalog needs to exist in the target registry. Copying a fake image..
      mirror_image_to_archive "registry.redhat.io/ubi9/ubi:latest" "${TO_DIR}/rhdh-catalog"
    fi
    if [[ -n "$TO_REGISTRY" ]]; then
      registryUrl=$(buildRegistryUrl)
      if [[ "${TO_REGISTRY}" == "OCP_INTERNAL" ]]; then
        registryUrl+="/oc-mirror"
      fi
      # targetCatalog needs to exist in the target registry. Copying a fake image..
      catalog_reg_path="rhdh-catalog"
      if [[ "${TO_REGISTRY}" == "OCP_INTERNAL" ]]; then
        catalog_reg_path="oc-mirror/rhdh-catalog"
      fi
      my_operator_index="$(buildCatalogImageUrl external "${catalog_reg_path}")"
      mirror_image_to_registry "registry.redhat.io/ubi9/ubi:latest" "${my_operator_index}-tmp"

      "${OC_MIRROR_PATH}" \
        --config="${TMPDIR}/imageset-config.yaml" \
        "docker://${registryUrl}" \
        --skip-missing \
        --dest-skip-tls \
        --continue-on-error \
        --max-nested-paths=2 \
        $OC_MIRROR_FLAGS \
        | tee "${ocMirrorLogFile}"
    fi

  else
    # from dir
    if [ ! -d "${FROM_DIR}" ]; then
      errorf "Directory not found: ${FROM_DIR}"
      exit 1
    fi
    if [[ -n "${TO_REGISTRY}" ]]; then
      registryUrl=$(buildRegistryUrl)
      if [[ "${TO_REGISTRY}" == "OCP_INTERNAL" ]]; then
        registryUrl+="/oc-mirror"
      fi

      # Rendering index, so as to manually build and push the targetCatalog (defined in the imageset).
      # The target catalog needs to exist in the target registry.
      catalog_reg_path="rhdh-catalog"
      if [[ "${TO_REGISTRY}" == "OCP_INTERNAL" ]]; then
        catalog_reg_path="oc-mirror/rhdh-catalog"
      fi
      my_operator_index="$(buildCatalogImageUrl external "${catalog_reg_path}")"
      push_image_from_archive "${FROM_DIR}/rhdh-catalog" "${my_operator_index}-tmp"

      "${OC_MIRROR_PATH}" \
        --config="${FROM_DIR}/imageset-config.yaml" \
        --from "${FROM_DIR}" \
        "docker://${registryUrl}" \
        --skip-missing \
        --dest-skip-tls \
        --continue-on-error \
        $OC_MIRROR_FLAGS \
        | tee "${ocMirrorLogFile}"
    fi
  fi

  if [ -f "${ocMirrorLogFile}" ]; then
    # The xargs here is to trim whitespaces
    catalogSourceLocation=$(sed -n -e 's/^Writing CatalogSource manifests to \(.*\)$/\1/p' "${ocMirrorLogFile}" |xargs)
    icspLocation=$(sed -n -e 's/^Writing ICSP manifests to \(.*\)$/\1/p' "${ocMirrorLogFile}" |xargs)
    if [[ -n "${icspLocation}" ]]; then
      debugf "ICSP parent location: ${TMPDIR}/${icspLocation}"
      if [[ -n "${TO_REGISTRY}" ]]; then
        invoke_cluster_cli apply -f ${TMPDIR}/${icspLocation}/imageContentSourcePolicy.yaml
      fi
    fi
    if [[ -n "${catalogSourceLocation}" ]]; then
      # Replace some metadata and add the default list of secrets
      debugf "catalogSource parent location: ${TMPDIR}/${catalogSourceLocation}"
      "$YQ" -i '.metadata.name = "rhdh-catalog"' -i ${TMPDIR}/${catalogSourceLocation}/catalogSource-*.yaml
      "$YQ" -i '.spec.displayName = "Red Hat Developer Hub Catalog (Airgapped)"' -i ${TMPDIR}/${catalogSourceLocation}/catalogSource-*.yaml
      "$YQ" -i '.spec.secrets = (.spec.secrets // []) + ["internal-reg-auth-for-rhdh", "internal-reg-ext-auth-for-rhdh", "reg-pull-secret"]' \
        ${TMPDIR}/${catalogSourceLocation}/catalogSource-*.yaml
      if [[ -n "${TO_REGISTRY}" ]]; then
        invoke_cluster_cli apply -f ${TMPDIR}/${catalogSourceLocation}/catalogSource-*.yaml
      fi
    fi
  fi
else
  if [[ -z "${FROM_DIR}" ]]; then
    render_index
    process_bundles
  else
    process_bundles_from_dir
    mirror_extra_images_from_dir
  fi
  mirror_extra_images

  # create OLM resources
  manifestsTargetDir="${TMPDIR}"
  if [[ -n "${FROM_DIR}" ]]; then
    manifestsTargetDir="${FROM_DIR}"
  fi

  NAMESPACE_CATALOGSOURCE='$NAMESPACE_CATALOGSOURCE'
  my_operator_index='$CATALOG_IMAGE'
  if [[ -n "${TO_REGISTRY}" ]]; then
    # It assumes that the user is also connected to a cluster
    detect_ocp_and_set_env_var
    if [[ "${IS_OPENSHIFT}" = "true" ]]; then
      debugf "Detected an OpenShift cluster"
      if ! command -v oc &> /dev/null; then
        errorf "Please install oc 4.10+ from an RPM or https://mirror.openshift.com/pub/openshift-v4/clients/ocp/"
        exit 1
      fi
      # Check we're logged into a cluster
      if ! oc whoami &> /dev/null; then
        errorf "Not logged into an OpenShift cluster"
        exit 1
      fi
    else
      if ! command -v oc &> /dev/null && ! command -v kubectl &> /dev/null; then
        errorf "Please install kubectl or oc 4.10+ (from an RPM or https://mirror.openshift.com/pub/openshift-v4/clients/ocp/)"
        exit 1
      fi
      debugf "Falling back to a standard K8s cluster"
      # Check that OLM is installed
      if ! invoke_cluster_cli get crd catalogsources.operators.coreos.com &> /dev/null; then
        errorf "
    OLM not installed (CatalogSource CRD not found) or you don't have enough permissions.
    Check that you are correctly logged into the cluster and that OLM is installed.
    See https://olm.operatorframework.io/docs/getting-started/#installing-olm-in-your-cluster to install OLM."
        exit 1
      fi
    fi

    if [[ "${IS_OPENSHIFT}" = "true" ]]; then
      NAMESPACE_CATALOGSOURCE="openshift-marketplace"
    else
      NAMESPACE_CATALOGSOURCE="olm"
    fi
    my_operator_index="$(buildCatalogImageUrl "internal")"
  fi

  cat <<EOF > "${manifestsTargetDir}/catalogSource.yaml"
  apiVersion: operators.coreos.com/v1alpha1
  kind: CatalogSource
  metadata:
    name: rhdh-catalog
    namespace: ${NAMESPACE_CATALOGSOURCE}
  spec:
    sourceType: grpc
    image: ${my_operator_index}
    secrets:
    - internal-reg-auth-for-rhdh
    - internal-reg-ext-auth-for-rhdh
    # Create this image pull secret if your mirror registry requires auth
    - reg-pull-secret
    publisher: "Red Hat"
    displayName: "Red Hat Developer Hub (Airgapped)"
EOF

  if [[ -n "${TO_REGISTRY}" ]]; then
    # IDMS will only work on regular OCP clusters. It doesn't work on ROSA or clusters with hosted control planes like on IBM Cloud.
    registry_url_internal=$(buildRegistryUrl)
    cat <<EOF > "${manifestsTargetDir}/imageDigestMirrorSet.yaml"
  apiVersion: config.openshift.io/v1
  kind: ImageDigestMirrorSet
  metadata:
    name: rhdh-idms
  spec:
    imageDigestMirrors:
    - mirrors:
      - ${registry_url_internal}/rhel9/postgresql-15
      source: registry.redhat.io/rhel9/postgresql-15
    - mirrors:
      - ${registry_url_internal}/rhdh
      source: registry.redhat.io/rhdh
    - mirrors:
      - ${registry_url_internal}/openshift4/ose-kube-rbac-proxy
      source: registry.redhat.io/openshift4/ose-kube-rbac-proxy
EOF
    # Also include mirrors to extra-images
    if [[ -n "${FROM_DIR}" ]]; then
      BASE_DIR="${FROM_DIR}/extraImages"
      if [ -d "${BASE_DIR}" ]; then
        # Iterate over all directories named "sha256_*"
        find "$BASE_DIR" -type d -name "sha256_*" | while read -r sha256_dir; do
          relative_path=${sha256_dir#"$BASE_DIR/"}
          sha256_hash=${sha256_dir##*/sha256_}
          parent_path=$(dirname "$relative_path")
          extraImg="${parent_path}"
          targetImg="${extraImg%@*}"
          targetImgLastTwo=$(extract_last_two_elements "$targetImg")
          cat <<EOF >> "${manifestsTargetDir}/imageDigestMirrorSet.yaml"
    - mirrors:
      - ${registry_url_internal}/${targetImgLastTwo}
      source: ${targetImg}
EOF
        done

        # Iterate over all directories named "tag_*"
        find "$BASE_DIR" -type d -name "tag_*" | while read -r tag_dir; do
          relative_path=${tag_dir#"$BASE_DIR/"}
          tag_hash=${tag_dir##*/tag_}
          parent_path=$(dirname "$relative_path")
          extraImg="${parent_path}"
          targetImg="${extraImg%:*}"
          targetImgLastTwo=$(extract_last_two_elements "$targetImg")
          cat <<EOF >> "${manifestsTargetDir}/imageDigestMirrorSet.yaml"
    - mirrors:
      - ${registry_url_internal}/${targetImgLastTwo}
      source: ${targetImg}
EOF
        done
      fi
    fi
    # Iterate from the --extra-images passed on the CLI
    debugf "Extra images from CLI: ${EXTRA_IMAGES[@]}..."
    for img in "${EXTRA_IMAGES[@]}"; do
      if [[ "$img" == *"@sha256:"* ]]; then
        targetImg="${img%@*}"
      elif [[ "$img" == *":"* ]]; then
        targetImg="${img%:*}"
      else
        targetImg="${img}"
      fi
      targetImgLastTwo=$(extract_last_two_elements "$targetImg")
      cat <<EOF >> "${manifestsTargetDir}/imageDigestMirrorSet.yaml"
    - mirrors:
      - ${registry_url_internal}/${targetImgLastTwo}
      source: ${targetImg}
EOF
    done

    # Create the IDMS (OCP-specific) and CatalogSource
    if [[ "${IS_OPENSHIFT}" = "true" ]]; then
       invoke_cluster_cli apply -f "${manifestsTargetDir}/imageDigestMirrorSet.yaml"
    fi
    debugf "Adding the internal cluster creds as pull secrets to be able to pull images from this internal registry by default"
    invoke_cluster_cli apply -f "${manifestsTargetDir}/catalogSource.yaml"
  fi
fi

if [[ -n "${TO_REGISTRY}" && "${IS_OPENSHIFT}" = "true" ]]; then
  infof "Disabling the default Red Hat Ecosystem Catalog."
  invoke_cluster_cli patch OperatorHub cluster --type json \
      --patch '[{"op": "add", "path": "/spec/disableAllDefaultSources", "value": true}]'
fi

cat <<EOF > "${manifestsTargetDir}/namespace.yaml"
apiVersion: v1
kind: Namespace
metadata:
  name: ${NAMESPACE_OPERATOR}
EOF

cat <<EOF > "${manifestsTargetDir}/operatorGroup.yaml"
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: rhdh-operator-group
  namespace: ${NAMESPACE_OPERATOR}
EOF

cat <<EOF > "${manifestsTargetDir}/subscription.yaml"
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: rhdh-operator
  namespace: ${NAMESPACE_OPERATOR}
spec:
  channel: fast
  installPlanApproval: Automatic
  name: rhdh
  source: rhdh-catalog
  sourceNamespace: ${NAMESPACE_CATALOGSOURCE}
EOF

if [[ "$INSTALL_OPERATOR" != "true" ]]; then
  echo
  echo "Done. "
  if [[ -n "${TO_DIR}" ]]; then
    echo "
${TO_DIR} should now contain all the images and resources needed to install the Red Hat Developer Hub operator. Next steps:

1. Transfer ${TO_DIR} over to your disconnected environment
2. In your disconnected environment, run the 'install.sh' script (located in your export dir) with the '--from-dir' and '--to-registry' options, like so:

# Make sure you are connected to the mirror registry and the target cluster
/path/to/export-dir/install.sh \
  --from-dir /path/to/export-dir \
  --to-registry \$mirror_registry_url \
  --install-operator <true|false>
    "
  fi
  if [[ -n "${TO_REGISTRY}" ]]; then
    if [[ "${IS_OPENSHIFT}" = "true" ]]; then
      echo "Now log into the OCP web console as an admin, then go to Operators > OperatorHub, search for Red Hat Developer Hub, and install the Red Hat Developer Hub Operator."
    else
      echo "To install the operator, you will need to create an OperatorGroup and a Subscription. You can do so with the following commands:

      kubectl -n ${NAMESPACE_OPERATOR} apply -f ${manifestsTargetDir}/namespace.yaml
      kubectl -n ${NAMESPACE_OPERATOR} apply -f ${manifestsTargetDir}/operatorGroup.yaml
      kubectl -n ${NAMESPACE_OPERATOR} apply -f ${manifestsTargetDir}/subscription.yaml
      "
    fi
  fi
  exit 0
fi

if [[ -n "${TO_REGISTRY}" ]]; then

  # Install the operator
  for manifest in namespace operatorGroup subscription; do
    invoke_cluster_cli apply -f "${manifestsTargetDir}/${manifest}.yaml"
  done

  invoke_cluster_cli -n ${NAMESPACE_OPERATOR} patch serviceaccount default \
    -p '{"imagePullSecrets": [{"name": "internal-reg-auth-for-rhdh"},{"name": "internal-reg-ext-auth-for-rhdh"},{"name": "reg-pull-secret"}]}'

  if [[ "${IS_OPENSHIFT}" = "true" ]]; then
    OCP_CONSOLE_ROUTE_HOST=$(invoke_cluster_cli get route console -n openshift-console -o=jsonpath='{.spec.host}')
    CLUSTER_ROUTER_BASE=$(invoke_cluster_cli get ingress.config.openshift.io/cluster '-o=jsonpath={.spec.domain}')
    echo -n "

  To install, go to:
  https://${OCP_CONSOLE_ROUTE_HOST}/catalog/ns/${NAMESPACE_OPERATOR}?catalogType=OperatorBackedService

  Or "
  else
    echo -n "

  To install on Kubernetes: "
  fi

  CLI_TOOL="kubectl"
  if [[ "${IS_OPENSHIFT}" = "true" ]]; then
    CLI_TOOL="oc"
  fi
  CR_EXAMPLE="
  cat <<EOF | ${CLI_TOOL} -n ${NAMESPACE_OPERATOR} apply -f -
  apiVersion: rhdh.redhat.com/v1alpha3
  kind: Backstage
  metadata:
    name: developer-hub
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
  EOF"

  echo "run this to create an RHDH instance:
  ${CR_EXAMPLE}

Note that if you are creating the CR above in a different namespace, you will probably need to add the right pull secrets to be able to
be able to pull the images from your mirror registry. You can do so by patching the default service account in your namespace, like so:

${CLI_TOOL} -n \$YOUR_NAMESPACE patch serviceaccount default -p '{\"imagePullSecrets\": [{\"name\": \"\$YOUR_PULL_SECRET_NAME\"}]}'

More details about image pull secrets in https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/
  "

  if [[ "${IS_OPENSHIFT}" = "true" ]]; then
    echo "
  Once deployed, Developer Hub will be available at
  https://backstage-developer-hub-${NAMESPACE_OPERATOR}.${CLUSTER_ROUTER_BASE}
  "
  fi
fi
