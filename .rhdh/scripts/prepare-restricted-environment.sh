#!/bin/bash
#
# Script to streamline installing the official RHDH Catalog Source in a disconnected OpenShift or Kubernetes cluster.
# Can be instructed to use oc-mirror v2 for image mirroring (v1 is deprecated in OCP 4.18+).
#
# Requires: oc (OCP) or kubectl (K8s), jq, yq, umoci, base64, opm, skopeo, oc-mirror v2

set -euo pipefail

SCRIPT_PATH=$(realpath "$0")

NC='\033[0m'

IS_OPENSHIFT=""
IS_HOSTED_CONTROL_PLANE=""

NAMESPACE_OPERATOR="rhdh-operator"
INDEX_IMAGE="registry.redhat.io/redhat/redhat-operator-index:v4.18"
FILTERED_VERSIONS=(*)

MAX_PARALLEL="${MAX_PARALLEL:-10}"
if ! [[ "$MAX_PARALLEL" =~ ^[0-9]+$ ]] || [[ "$MAX_PARALLEL" -lt 1 ]]; then
  echo "[ERROR] MAX_PARALLEL must be a positive integer, got: '$MAX_PARALLEL'" >&2
  exit 1
fi

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

SEM_FD=""

function sem_init() {
  local fifo
  fifo=$(mktemp -u)
  mkfifo "$fifo"
  exec {SEM_FD}<>"$fifo"
  rm -f "$fifo"
  for ((i = 0; i < MAX_PARALLEL; i++)); do
    printf '\n' >&"${SEM_FD}"
  done
}

function sem_acquire() {
  IFS= read -r -u "${SEM_FD}"
}

function sem_release() {
  printf '\n' >&"${SEM_FD}"
}

function wait_for_pids() {
  local -n __wfp_pids=$1
  local context="${2:-background job}"
  local failed=0
  for pid in ${__wfp_pids[@]+"${__wfp_pids[@]}"}; do
    if ! wait "$pid"; then
      failed=$((failed + 1))
    fi
  done
  __wfp_pids=()
  if [[ $failed -gt 0 ]]; then
    errorf "${failed} ${context}(s) failed"
    return 1
  fi
}

function check_tool() {
  if ! command -v "$1" >/dev/null; then
    errorf "Error: Required tool '$1' is not installed."
    exit 1
  fi
}

function usage() {
  FILTERED_VERSIONS_CSV="${FILTERED_VERSIONS[*]}"
  FILTERED_VERSIONS_CSV="${FILTERED_VERSIONS_CSV// /,}"
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
                                            with quay.io when mirroring images. Relevant only if '--use-oc-mirror' is 'false'. 
                                            When set to 'true', --filter-versions must be explicitly specified. Default: false
  --filter-versions <list>               : Comma-separated list of operator minor versions to keep in the catalog (default: * (all versions)).
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
  --use-oc-mirror <true|false>           : Whether to use the 'oc-mirror' tool v2 (default: false).
                                            This is the recommended way for mirroring on regular OpenShift clusters.
                                            oc-mirror v2 generates ImageDigestMirrorSet and ImageTagMirrorSet resources
                                            instead of ImageContentSourcePolicy. Bear in mind however that ImageDigestMirrorSet
                                            and ImageTagMirrorSet don't seem to work well on ROSA clusters or clusters with hosted control
                                            planes (like HyperShift or Red Hat OpenShift on IBM Cloud).
                                            IMPORTANT: When using --to-dir (mirrorToDisk workflow), oc-mirror v2 only copies images
                                            to disk and does NOT create cluster resources (CatalogSource, IDMS, ITMS). These resources
                                            are only generated when pushing to a registry (mirrorToMirror workflow or diskToMirror workflow) using --from-dir and
                                            --to-registry together.
  --oc-mirror-path <path>                : Path to the oc-mirror binary (default: 'oc-mirror').
  --oc-mirror-flags <string>             : Additional flags to pass to all oc-mirror commands.
  --max-parallel <N>                     : Maximum number of parallel image operations (default: 10, env: MAX_PARALLEL).
                                            Lower this value if you are running low on disk space, as fewer concurrent
                                            downloads will reduce peak disk usage.
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
    --ci-index true \\
    --filter-versions '1.4,1.5'

  # WORKFLOW with oc-mirror v2 for fully disconnected environments:
  # (on connected host): Export images to disk
  $0 \\
    --use-oc-mirror true \\
    --to-dir /path/to/export
  
  # (on disconnected bastion): Push images to registry and create cluster resources
  $0 \\
    --use-oc-mirror true \\
    --from-dir /path/to/export \\
    --to-registry registry.example.com
"
}

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
FILTER_VERSIONS_PROVIDED="false"

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
  '--prod_operator_index')
    INDEX_IMAGE="$2"
    shift 1
    ;;
  '--prod_operator_package_name')
    debugf "--prod_operator_package_name ($2) is no longer used"
    shift 1
    ;;
  '--prod_operator_bundle_name')
    debugf "--prod_operator_bundle_name ($2) is no longer used"
    shift 1
    ;;
  '--helper_mirror_registry_storage')
    debugf "--helper_mirror_registry_storage is no longer used. This script assumes you already have a mirror registry in your disconnected environment"
    shift 1
    ;;
  '--use_existing_mirror_registry')
    debugf "--use_existing_mirror_registry is no longer used. This script assumes you already have a mirror registry in your disconnected environment"
    shift 1
    ;;
  '--prod_operator_version')
    FILTER_VERSIONS_PROVIDED="true"
    input="${2#v}"
    IFS='.' read -ra parts <<<"$input"
    length=${#parts[@]}
    if [ "$length" -ge 2 ]; then
      FILTERED_VERSIONS=("${parts[0]}"."${parts[1]}")
    else
      FILTERED_VERSIONS=("${parts[*]}")
    fi
    debugf "${FILTERED_VERSIONS[@]}"
    shift 1
    ;;

  # New options
  '--index-image')
    INDEX_IMAGE="$2"
    shift 1
    ;;
  '--ci-index')
    IS_CI_INDEX_IMAGE="$2"
    shift 1
    ;;
  '--filter-versions')
    FILTER_VERSIONS_PROVIDED="true"
    if [[ "$2" == "*" ]]; then
      NO_VERSION_FILTER="true"
    else
      IFS=',' read -r -a FILTERED_VERSIONS <<<"$2"
    fi
    shift 1
    ;;
  '--extra-images')
    IFS=',' read -r -a EXTRA_IMAGES <<<"$2"
    shift 1
    ;;
  '--to-registry')
    TO_REGISTRY="$2"
    shift 1
    ;;
  '--to-dir')
    TO_DIR=$(realpath "$2")
    shift 1
    ;;
  '--from-dir')
    FROM_DIR="$2"
    shift 1
    ;;
  '--install-operator')
    INSTALL_OPERATOR="$2"
    shift 1
    ;;
  '--use-oc-mirror')
    USE_OC_MIRROR="$2"
    shift 1
    ;;
  '--oc-mirror-path')
    OC_MIRROR_PATH="$2"
    shift 1
    ;;
  '--oc-mirror-flags')
    OC_MIRROR_FLAGS="$2"
    shift 1
    ;;
  '--max-parallel')
    MAX_PARALLEL="$2"
    if ! [[ "$MAX_PARALLEL" =~ ^[0-9]+$ ]] || [[ "$MAX_PARALLEL" -lt 1 ]]; then
      errorf "MAX_PARALLEL must be a positive integer, got: '$MAX_PARALLEL'"
      exit 1
    fi
    shift 1
    ;;
  '--install-yq')
    INSTALL_YQ=1 ;;
  '-h' | '--help')
    usage
    exit 0
    ;;
  *)
    errorf "Unknown parameter is used: $1."
    usage
    exit 1
    ;;
  esac
  shift 1
done

function is_openshift() {
  set -euo pipefail

  oc get routes.route.openshift.io &>/dev/null || kubectl get routes.route.openshift.io &>/dev/null
}

function detect_ocp_and_set_env_var() {
  set -euo pipefail

  if [[ -z "${IS_OPENSHIFT}" ]]; then
    IS_OPENSHIFT=$(is_openshift && echo 'true' || echo 'false')
    debugf "IS_OPENSHIFT: ${IS_OPENSHIFT}"
  fi
  if [[ "${IS_OPENSHIFT}" == "true" ]] && [[ -z "${IS_HOSTED_CONTROL_PLANE}" ]]; then
    local cpTech
    cpTech=$(oc get infrastructure cluster -o jsonpath='{.status.controlPlaneTopology}' ||
      (warnf 'Could not determine the cluster type => defaulting to the hosted control plane behavior' >&2 && echo 'External'))
    if [[ "${cpTech}" == "External" ]]; then
      # 'External' indicates that the control plane is hosted externally to the cluster
      # and that its components are not visible within the cluster.
      IS_HOSTED_CONTROL_PLANE="true"
    else
      IS_HOSTED_CONTROL_PLANE="false"
    fi
    debugf "IS_HOSTED_CONTROL_PLANE: ${IS_HOSTED_CONTROL_PLANE}"
  fi
}

# Wrapper function to call kubectl or oc
function invoke_cluster_cli() {
  set -euo pipefail

  local command=$1
  shift

  detect_ocp_and_set_env_var
  if [[ "${IS_OPENSHIFT}" = "true" ]]; then
    if command -v oc &>/dev/null; then
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
if [[ "${IS_CI_INDEX_IMAGE}" == "true" ]]; then
  if [[ "${FILTER_VERSIONS_PROVIDED}" == "false" ]]; then
    errorf "When --ci-index is true, --filter-versions must be specified."
    exit 1
  fi
fi

if [[ -n "${TO_DIR}" ]]; then
  mkdir -p "${TO_DIR}"
  TMPDIR="${TO_DIR}"
  trap '{ jobs -p | xargs -r kill 2>/dev/null; wait 2>/dev/null; } || true' EXIT
  trap 'exit 1' INT TERM
else
  TMPDIR=$(mktemp -d)
  # shellcheck disable=SC2064
  trap "rm -fr \"$TMPDIR\" || true; { jobs -p | xargs -r kill 2>/dev/null; wait 2>/dev/null; } || true" EXIT
  trap 'exit 1' INT TERM
fi
pushd "${TMPDIR}" >/dev/null
debugf ">>> WORKING DIR: $TMPDIR <<<"

sem_init

if (( INSTALL_YQ )); then
  YQ=$HOME/.local/bin/yq_mf
  YQ_BINARY=yq_linux_amd64
  mkdir -p "$HOME/.local/bin"
  curl -sSLo- https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/${YQ_BINARY}.tar.gz | tar xz && mv -f ${YQ_BINARY} "${YQ}"
  debugf "mikefarah yq $YQ_VERSION installed to $YQ"
else
  YQ=$(command -v yq)
fi


function ocp_prepare_internal_registry() {
  set -euo pipefail

  debugf "Exposing cluster registry..." >&2
  internal_registry_url="image-registry.openshift-image-registry.svc:5000"
  oc patch configs.imageregistry.operator.openshift.io/cluster --patch '{"spec":{"defaultRoute":true}}' --type=merge >&2
  # https://access.redhat.com/solutions/6022011
  oc patch configs.imageregistry.operator.openshift.io/cluster --patch '{"spec":{"disableRedirect":true}}' --type=merge >&2
  my_registry=$(oc get route default-route -n openshift-image-registry --template='{{ .spec.host }}')

  skopeo login -u kubeadmin -p "$(oc whoami -t)" --tls-verify=false "$my_registry" >&2 &
  local skopeo_login_pid=$!
  podman login -u kubeadmin -p "$(oc whoami -t)" --tls-verify=false "$my_registry" >&2 &
  local podman_login_pid=$!

  local ns_pids=()
  for ns in rhdh-operator openshift4 rhdh rhel9 oc-mirror; do
    (
      if ! oc get namespace "$ns" &>/dev/null; then
        oc create namespace "$ns" >&2
      fi
      oc adm policy add-cluster-role-to-user system:image-signer "system:serviceaccount:${ns}:default" >&2 || true
    ) &
    ns_pids+=($!)
  done
  wait_for_pids ns_pids "namespace setup"

  if ! wait "$skopeo_login_pid"; then
    errorf "skopeo login failed"
    return 1
  fi
  if ! wait "$podman_login_pid"; then
    errorf "podman login failed"
    return 1
  fi

  local secret_pids=()
  for ns in rhdh-operator openshift-marketplace; do
    (
      if oc -n "${ns}" get secret internal-reg-ext-auth-for-rhdh &>/dev/null; then
        oc -n "${ns}" delete secret internal-reg-ext-auth-for-rhdh >&2
      fi
      oc -n "${ns}" create secret docker-registry internal-reg-ext-auth-for-rhdh \
        --docker-server="${my_registry}" \
        --docker-username=kubeadmin \
        --docker-password="$(oc whoami -t)" \
        --docker-email="admin@internal-registry-ext.example.com" >&2
      if oc -n "${ns}" get secret internal-reg-auth-for-rhdh &>/dev/null; then
        oc -n "${ns}" delete secret internal-reg-auth-for-rhdh >&2
      fi
      oc -n "${ns}" create secret docker-registry internal-reg-auth-for-rhdh \
        --docker-server="${internal_registry_url}" \
        --docker-username=kubeadmin \
        --docker-password="$(oc whoami -t)" \
        --docker-email="admin@internal-registry.example.com" >&2
      oc adm policy add-cluster-role-to-user system:image-signer "system:serviceaccount:${ns}:default" >&2 || true
    ) &
    secret_pids+=($!)
  done
  wait_for_pids secret_pids "secret setup"

  local policy_pids=()
  oc policy add-role-to-user system:image-puller system:serviceaccount:openshift-marketplace:default -n openshift-marketplace >&2 &
  policy_pids+=($!)
  oc policy add-role-to-user system:image-puller system:serviceaccount:rhdh-operator:default -n rhdh-operator >&2 &
  policy_pids+=($!)
  oc policy add-role-to-user system:image-puller system:serviceaccount:rhdh-operator:rhdh-operator -n rhdh-operator >&2 &
  policy_pids+=($!)
  wait_for_pids policy_pids "policy setup"
}

function buildRegistryUrl() {
  set -euo pipefail

  detect_ocp_and_set_env_var
  local input="${1:-external}"
  if [[ "${IS_OPENSHIFT}" = "true" && "${TO_REGISTRY}" = "OCP_INTERNAL" ]]; then
    if [[ "${input}" == "internal" ]]; then
      echo "image-registry.openshift-image-registry.svc:5000"
    else
      oc get route default-route -n openshift-image-registry --template='{{ .spec.host }}'
    fi
  else
    echo "${TO_REGISTRY}"
  fi
}

CACHED_REGISTRY_URL=""
CACHED_REGISTRY_URL_INTERNAL=""

function cache_registry_urls() {
  if [[ -n "$TO_REGISTRY" ]]; then
    CACHED_REGISTRY_URL=$(buildRegistryUrl)
    CACHED_REGISTRY_URL_INTERNAL=$(buildRegistryUrl "internal")
  fi
}

function buildCatalogImageUrl() {
  if [[ -n "$TO_REGISTRY" ]]; then
    tag=${INDEX_IMAGE##*:}
    [[ "$INDEX_IMAGE" == "$tag" ]] && tag="latest"
    local reg_url
    if [[ "${1:-external}" == "internal" ]]; then
      reg_url="${CACHED_REGISTRY_URL_INTERNAL}"
    else
      reg_url="${CACHED_REGISTRY_URL}"
    fi
    echo "${reg_url}/${2:-rhdh/index}:${tag}"
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
    opm render "${INDEX_IMAGE}" --output=yaml >"${local_index_file}"
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

    opm render "${INDEX_IMAGE}" --output=yaml |
      "$YQ" 'select(
          (.schema == "olm.package" and .name == "'${prod_operator_package_name}'")
          or
          (.schema == "olm.channel" and .package == "'${prod_operator_package_name}'" and .name == "fast")
          or
          '"$chanFilterList"'
          or
          '"$bundleFilterList"'
        )' | "$YQ" '.entries |= map(select('"$chanEntriesFilterList"'))' \
      >"${local_index_file}"
  fi

  debugf "Got $(wc -l < "${local_index_file}") lines of JSON from the index!"

  if [ ! -s "${local_index_file}" ]; then
    errorf "[ERROR] 'opm render $INDEX_IMAGE' returned an empty output, which likely means that this index Image does not contain the rhdh operator."
    return 1
  fi
}

function extract_last_two_elements() {
  local input="$1"
  local IFS='/'

  read -ra parts <<<"$input"

  local length=${#parts[@]}
  if [ "$length" -ge 2 ]; then
    echo "${parts[-2]}/${parts[-1]}"
  else
    echo "${parts[*]}"
  fi
}

function _last_two() {
  local -n __lt_result=$1
  local __lt_input="$2"
  if [[ "$__lt_input" == */*/* ]]; then
    local __lt_last="${__lt_input##*/}"
    local __lt_rest="${__lt_input%/*}"
    __lt_result="${__lt_rest##*/}/${__lt_last}"
  else
    __lt_result="$__lt_input"
  fi
}

function mirror_extra_images() {
  if [[ ${#EXTRA_IMAGES[@]} -eq 0 ]]; then
    return
  fi
  debugf "Extra images (${#EXTRA_IMAGES[@]}, max ${MAX_PARALLEL} parallel): ${EXTRA_IMAGES[*]}"

  if [[ -n "$TO_REGISTRY" ]] && [[ "${IS_OPENSHIFT}" = "true" && "${TO_REGISTRY}" = "OCP_INTERNAL" ]]; then
    for img in "${EXTRA_IMAGES[@]}"; do
      local lastTwo=""
      if [[ "$img" == *"@sha256:"* ]]; then
        _last_two lastTwo "${img%@*}"
      elif [[ "$img" == *":"* ]]; then
        _last_two lastTwo "${img%:*}"
      else
        _last_two lastTwo "${img}"
      fi
      local projectNameForOcpReg=${lastTwo%%/*}
      oc get namespace "${projectNameForOcpReg}" &>/dev/null || oc create namespace "${projectNameForOcpReg}"
    done
  fi

  local pids=()
  for img in "${EXTRA_IMAGES[@]}"; do
    local imgDir imgTag lastTwo="" targetImg
    if [[ "$img" == *"@sha256:"* ]]; then
      local imgDigest="${img##*@sha256:}"
      imgDir="./extraImages/${img%@*}/sha256_$imgDigest"
      _last_two lastTwo "${img%@*}"
      targetImg="${CACHED_REGISTRY_URL}/${lastTwo}:$imgDigest"
    elif [[ "$img" == *":"* ]]; then
      imgTag="${img##*:}"
      imgDir="./extraImages/${img%:*}/tag_$imgTag"
      _last_two lastTwo "${img%:*}"
      targetImg="${CACHED_REGISTRY_URL}/${lastTwo}:$imgTag"
    else
      imgDir="./extraImages/${img}/tag_latest"
      _last_two lastTwo "${img}"
      targetImg="${CACHED_REGISTRY_URL}/${lastTwo}:latest"
    fi

    if [[ -n "$TO_REGISTRY" ]]; then
      mirror_image_to_registry "$img" "$targetImg" &
      pids+=($!)
    else
      if [ ! -d "$imgDir" ]; then
        mkdir -p "${imgDir}"
        mirror_image_to_archive "$img" "$imgDir" &
        pids+=($!)
      fi
    fi
  done
  wait_for_pids pids "extra image"
  debugf "... done."
}

function mirror_extra_images_from_dir() {
  local BASE_DIR="${FROM_DIR}/extraImages"
  debugf "Extra images from ${BASE_DIR}..."
  if [ ! -d "${BASE_DIR}" ]; then
    return
  fi

  local sha256_dirs=()
  mapfile -t sha256_dirs < <(find "$BASE_DIR" -type d -name "sha256_*" 2>/dev/null)
  local tag_dirs=()
  mapfile -t tag_dirs < <(find "$BASE_DIR" -type d -name "tag_*" 2>/dev/null)

  if [[ ${#sha256_dirs[@]} -eq 0 ]] && [[ ${#tag_dirs[@]} -eq 0 ]]; then
    return
  fi

  infof "Pushing ${#sha256_dirs[@]} digest + ${#tag_dirs[@]} tag extra images from dir (max ${MAX_PARALLEL} parallel)..."

  if [[ -n "$TO_REGISTRY" ]] && [[ "${IS_OPENSHIFT}" = "true" && "${TO_REGISTRY}" = "OCP_INTERNAL" ]]; then
    local ns_set=()
    for sha256_dir in "${sha256_dirs[@]}"; do
      local relative_path=${sha256_dir#"$BASE_DIR/"}
      local parent_path
      parent_path=$(dirname "$relative_path")
      local lastTwo=""
      _last_two lastTwo "${parent_path}"
      ns_set+=("${lastTwo%%/*}")
    done
    for tag_dir in "${tag_dirs[@]}"; do
      local relative_path=${tag_dir#"$BASE_DIR/"}
      local parent_path
      parent_path=$(dirname "$relative_path")
      local lastTwo=""
      _last_two lastTwo "${parent_path}"
      ns_set+=("${lastTwo%%/*}")
    done
    local unique_ns
    unique_ns=$(printf '%s\n' "${ns_set[@]}" | sort -u)
    while IFS= read -r ns; do
      if [[ -n "$ns" ]] && ! oc get namespace "$ns" &>/dev/null; then
        oc create namespace "$ns"
      fi
    done <<<"$unique_ns"
  fi

  local pids=()

  for sha256_dir in "${sha256_dirs[@]}"; do
    local relative_path=${sha256_dir#"$BASE_DIR/"}
    local sha256_hash=${sha256_dir##*/sha256_}
    local parent_path
    parent_path=$(dirname "$relative_path")
    local lastTwo=""
    _last_two lastTwo "${parent_path}"
    local extraImg="${lastTwo}:${sha256_hash}"
    if [[ -n "$TO_REGISTRY" ]]; then
      local targetImg
      targetImg="${CACHED_REGISTRY_URL}/${extraImg%@*}"
      push_image_from_archive "$sha256_dir" "$targetImg" &
      pids+=($!)
    fi
  done

  for tag_dir in "${tag_dirs[@]}"; do
    local relative_path=${tag_dir#"$BASE_DIR/"}
    local tag_hash=${tag_dir##*/tag_}
    local parent_path
    parent_path=$(dirname "$relative_path")
    local lastTwo=""
    _last_two lastTwo "${parent_path}"
    local extraImg="${lastTwo}:${tag_hash}"
    if [[ -n "$TO_REGISTRY" ]]; then
      local targetImg
      targetImg="${CACHED_REGISTRY_URL}/${extraImg}"
      push_image_from_archive "$tag_dir" "$targetImg" &
      pids+=($!)
    fi
  done

  wait_for_pids pids "extra image from dir"
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

function process_single_bundle() {
  set -euo pipefail

  local bundleImg="$1"
  local originalBundleImg="$2"
  local digest="$3"
  local sed_commands_dir="$4"
  local bundle_id="$5"

  local bundle_dir="bundles/${digest}"
  mkdir -p "${bundle_dir}"

  sem_acquire
  if ! skopeo copy --remove-signatures "docker://$bundleImg" "oci:./${bundle_dir}/src:latest" 2>"${bundle_dir}/copy.err"; then
    sem_release
    debugf "bundle #${bundle_id}: skopeo copy failed, skipping (see ${bundle_dir}/copy.err)" >&2
    return 0
  fi
  sem_release
  debugf "bundle #${bundle_id}: pulled ${bundleImg}" >&2

  umoci unpack --image "./${bundle_dir}/src:latest" "./${bundle_dir}/unpacked" --rootless

  for file in "./${bundle_dir}/unpacked/rootfs/manifests"/*; do
    if [[ "$file" == *.clusterserviceversion.yaml || "$file" == *.csv.yaml ]]; then
      "$YQ" eval '
        (.spec.install.spec.deployments[] | select(.name == "rhdh-operator").spec.template.spec) +=
          {"imagePullSecrets": [{"name": "internal-reg-auth-for-rhdh"},{"name": "internal-reg-ext-auth-for-rhdh"},{"name": "reg-pull-secret"}]}
      ' -i "$file"

      local all_related_images=()
      local images
      mapfile -t images < <(grep -E 'image: ' "$file" | awk -F ': ' '{print $2}' | uniq)
      if ((${#images[@]})); then
        all_related_images+=("${images[@]}")
      fi
      local related_images
      related_images=$("$YQ" '.spec.install.spec.deployments[].spec.template.spec.containers[].env[] | select(.name | test("^RELATED_IMAGE_")).value' "$file" || true)
      if [[ -n "$related_images" ]]; then
        while IFS= read -r img; do
          [[ -n "$img" ]] && all_related_images+=("$img")
        done <<<"$related_images"
      fi

      local csv_sed_file="${sed_commands_dir}/${digest}_csv.sed"
      : > "$csv_sed_file"
      local inner_pids=()

      for relatedImage in "${all_related_images[@]}"; do
        local imgDir="./images/"
        local lastTwo="" targetImg internalTargetImg
        if [[ "$relatedImage" == *"@sha256:"* ]]; then
          local relatedImageDigest="${relatedImage##*@sha256:}"
          imgDir+="${relatedImage%@*}/sha256_$relatedImageDigest"
          _last_two lastTwo "${relatedImage%@*}"
          targetImg="${CACHED_REGISTRY_URL}/${lastTwo}:$relatedImageDigest"
          internalTargetImg="${CACHED_REGISTRY_URL_INTERNAL}/${lastTwo}:$relatedImageDigest"
        elif [[ "$relatedImage" == *":"* ]]; then
          local relatedImageTag="${relatedImage##*:}"
          imgDir+="${relatedImage%:*}/tag_$relatedImageTag"
          _last_two lastTwo "${relatedImage%:*}"
          targetImg="${CACHED_REGISTRY_URL}/${lastTwo}:$relatedImageTag"
          internalTargetImg="${CACHED_REGISTRY_URL_INTERNAL}/${lastTwo}:$relatedImageTag"
        else
          imgDir+="${relatedImage}/tag_latest"
          _last_two lastTwo "${relatedImage}"
          targetImg="${CACHED_REGISTRY_URL}/${lastTwo}:latest"
          internalTargetImg="${CACHED_REGISTRY_URL_INTERNAL}/${lastTwo}:latest"
        fi

        if [[ -n "$TO_REGISTRY" ]]; then
          echo "s#${relatedImage}#${internalTargetImg}#g" >> "$csv_sed_file"
          mirror_image_to_registry "$relatedImage" "$targetImg" &
          inner_pids+=($!)
        else
          if [ ! -d "$imgDir" ]; then
            mkdir -p "${imgDir}"
            mirror_image_to_archive "$relatedImage" "$imgDir" &
            inner_pids+=($!)
          fi
        fi
      done

      wait_for_pids inner_pids "related image mirror"

      if [[ -s "$csv_sed_file" ]]; then
        sed -i -f "$csv_sed_file" "$file"
      fi
    fi
  done

  if [[ -n "$TO_REGISTRY" ]]; then
    umoci repack --image "./${bundle_dir}/src:latest" "./${bundle_dir}/unpacked"

    local bundleLastTwo=""
    _last_two bundleLastTwo "${bundleImg%@*}"
    local newBundleImage="${CACHED_REGISTRY_URL}/${bundleLastTwo}:${digest}"
    local newBundleImageInternal="${CACHED_REGISTRY_URL_INTERNAL}/${bundleLastTwo}:${digest}"
    debugf "bundle #${bundle_id}: pushing ${newBundleImage}" >&2
    sem_acquire
    skopeo copy --remove-signatures --dest-tls-verify=false "oci:./${bundle_dir}/src:latest" "docker://${newBundleImage}" || { sem_release; return 1; }
    sem_release

    echo "s#${originalBundleImg}#${newBundleImageInternal}#g" > "${sed_commands_dir}/${digest}.sed"
  fi
}

function process_bundles() {

  local bundle_images
  bundle_images=$(grep -E '^image: .*operator-bundle' "${TMPDIR}/rhdh/rhdh/render.yaml" | awk '{print $2}' | uniq)

  local total_bundles
  total_bundles=$(echo "$bundle_images" | wc -l | tr -d ' ')
  infof "Processing ${total_bundles} bundles (max ${MAX_PARALLEL} parallel)..."

  local sed_commands_dir="${TMPDIR}/sed_commands_bundles"
  mkdir -p "$sed_commands_dir"

  local bundle_count=0
  local pids=()

  for bundleImg in $bundle_images; do
    bundle_count=$((bundle_count + 1))
    local originalBundleImg="$bundleImg"
    bundleImg=$(replaceInternalRegIfNeeded "$bundleImg")
    local digest="${bundleImg##*@sha256:}"
    debugf "bundle #${bundle_count}/${total_bundles}: $originalBundleImg => $bundleImg"

    process_single_bundle "$bundleImg" "$originalBundleImg" "$digest" "$sed_commands_dir" "$bundle_count" &
    pids+=($!)
  done

  wait_for_pids pids "bundle"

  local sed_files
  sed_files=$(find "$sed_commands_dir" -maxdepth 1 -name '*.sed' ! -name '*_csv.sed' 2>/dev/null || true)
  if [[ -n "$sed_files" ]]; then
    local combined_sed="${TMPDIR}/combined_bundle_sed.txt"
    find "$sed_commands_dir" -maxdepth 1 -name '*.sed' ! -name '*_csv.sed' -exec cat {} + > "$combined_sed"
    local replacement_count
    replacement_count=$(wc -l < "$combined_sed" | tr -d ' ')
    infof "Applying ${replacement_count} image ref replacements to render.yaml..."
    sed -i -f "$combined_sed" "./rhdh/rhdh/render.yaml"
  fi

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

function process_single_bundle_from_dir() {
  set -euo pipefail

  local bundleImg="$1"
  local digest="$2"
  local sed_commands_dir="$3"
  local bundle_id="$4"

  if [ ! -d "${FROM_DIR}/bundles/${digest}/src" ]; then
    warnf "missing src image for bundle digest: ${FROM_DIR}/bundles/${digest}/src" >&2
    return 0
  fi
  if [ ! -d "${FROM_DIR}/bundles/${digest}/unpacked" ]; then
    warnf "missing unpacked image for bundle digest: ${FROM_DIR}/bundles/${digest}/unpacked" >&2
    return 0
  fi

  debugf "bundle #${bundle_id}: handling from ${FROM_DIR}/bundles/${digest}..." >&2

  for file in "${TMPDIR}/bundles/${digest}/unpacked/rootfs/manifests"/*; do
    if [[ "$file" == *.clusterserviceversion.yaml || "$file" == *.csv.yaml ]]; then
      local all_related_images=()
      local images
      mapfile -t images < <(grep -E 'image: ' "$file" | awk -F ': ' '{print $2}' | uniq)
      if ((${#images[@]})); then
        all_related_images+=("${images[@]}")
      fi
      local related_images
      related_images=$("$YQ" '.spec.install.spec.deployments[].spec.template.spec.containers[].env[] | select(.name | test("^RELATED_IMAGE_")).value' "$file" || true)
      if [[ -n "$related_images" ]]; then
        while IFS= read -r img; do
          [[ -n "$img" ]] && all_related_images+=("$img")
        done <<<"$related_images"
      fi

      local csv_sed_file="${sed_commands_dir}/${digest}_csv.sed"
      : > "$csv_sed_file"
      local inner_pids=()

      for relatedImage in "${all_related_images[@]}"; do
        local imgDir="${FROM_DIR}/images/"
        local lastTwo="" targetImg targetImgInternal
        if [[ "$relatedImage" == *"@sha256:"* ]]; then
          local relatedImageDigest="${relatedImage##*@sha256:}"
          imgDir+="${relatedImage%@*}/sha256_$relatedImageDigest"
          _last_two lastTwo "${relatedImage%@*}"
          targetImg="${CACHED_REGISTRY_URL}/${lastTwo}:$relatedImageDigest"
          targetImgInternal="${CACHED_REGISTRY_URL_INTERNAL}/${lastTwo}:$relatedImageDigest"
        elif [[ "$relatedImage" == *":"* ]]; then
          local relatedImageTag="${relatedImage##*:}"
          imgDir+="${relatedImage%:*}/tag_$relatedImageTag"
          _last_two lastTwo "${relatedImage%:*}"
          targetImg="${CACHED_REGISTRY_URL}/${lastTwo}:$relatedImageTag"
          targetImgInternal="${CACHED_REGISTRY_URL_INTERNAL}/${lastTwo}:$relatedImageTag"
        else
          imgDir+="${relatedImage}/tag_latest"
          _last_two lastTwo "${relatedImage}"
          targetImg="${CACHED_REGISTRY_URL}/${lastTwo}:latest"
          targetImgInternal="${CACHED_REGISTRY_URL_INTERNAL}/${lastTwo}:latest"
        fi
        if [ ! -d "$imgDir" ]; then
          warnf "Skipping related image $relatedImage not found mirrored in dir: $FROM_DIR/images" >&2
          continue
        fi
        if [[ -n "$TO_REGISTRY" ]]; then
          echo "s#${relatedImage}#${targetImgInternal}#g" >> "$csv_sed_file"
          push_image_from_archive "$imgDir" "$targetImg" &
          inner_pids+=($!)
        fi
      done

      wait_for_pids inner_pids "related image push"

      if [[ -s "$csv_sed_file" ]]; then
        sed -i -f "$csv_sed_file" "$file"
      fi
    fi
  done

  if [[ -n "$TO_REGISTRY" ]]; then
    umoci repack --image "${TMPDIR}/bundles/${digest}/src:latest" "${TMPDIR}/bundles/${digest}/unpacked"

    local bundleLastTwo=""
    _last_two bundleLastTwo "${bundleImg%@*}"
    local newBundleImage="${CACHED_REGISTRY_URL}/${bundleLastTwo}:${digest}"
    local newBundleImageInternal="${CACHED_REGISTRY_URL_INTERNAL}/${bundleLastTwo}:${digest}"
    debugf "bundle #${bundle_id}: pushing ${newBundleImage}" >&2
    sem_acquire
    skopeo copy --preserve-digests --remove-signatures --dest-tls-verify=false "oci:${TMPDIR}/bundles/${digest}/src:latest" "docker://${newBundleImage}" || { sem_release; return 1; }
    sem_release

    echo "s#${bundleImg}#${newBundleImageInternal}#g" > "${sed_commands_dir}/${digest}.sed"
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

  local bundle_images
  bundle_images=$(grep -E '^image: .*operator-bundle' "${FROM_DIR}/rhdh/rhdh/render.yaml" | awk '{print $2}' | uniq)

  local total_bundles
  total_bundles=$(echo "$bundle_images" | wc -l | tr -d ' ')
  infof "Processing ${total_bundles} bundles from dir (max ${MAX_PARALLEL} parallel)..."

  local sed_commands_dir="${TMPDIR}/sed_commands_bundles_from_dir"
  mkdir -p "$sed_commands_dir"

  local bundle_count=0
  local pids=()

  for bundleImg in $bundle_images; do
    bundle_count=$((bundle_count + 1))
    local digest="${bundleImg##*@sha256:}"
    debugf "bundle #${bundle_count}/${total_bundles}: $bundleImg"

    process_single_bundle_from_dir "$bundleImg" "$digest" "$sed_commands_dir" "$bundle_count" &
    pids+=($!)
  done

  wait_for_pids pids "bundle from dir"

  local sed_files
  sed_files=$(find "$sed_commands_dir" -maxdepth 1 -name '*.sed' ! -name '*_csv.sed' 2>/dev/null || true)
  if [[ -n "$sed_files" ]]; then
    local combined_sed="${TMPDIR}/combined_bundle_from_dir_sed.txt"
    find "$sed_commands_dir" -maxdepth 1 -name '*.sed' ! -name '*_csv.sed' -exec cat {} + > "$combined_sed"
    local replacement_count
    replacement_count=$(wc -l < "$combined_sed" | tr -d ' ')
    infof "Applying ${replacement_count} image ref replacements to render.yaml..."
    sed -i -f "$combined_sed" "${TMPDIR}/rhdh/rhdh/render.yaml"
  fi

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
  sem_acquire
  local rc=0
  local src_image="$1"
  if [[ "${IS_CI_INDEX_IMAGE}" == "true" ]]; then
    for reg in registry.stage.redhat.io registry.redhat.io; do
      src_image="${src_image/$reg\/rhdh/quay.io\/rhdh}"
    done
    src_image="${src_image/registry-proxy.engineering.redhat.com\/rh-osbs\/rhdh-/quay.io\/rhdh\/}"
  fi
  local dest_image=$2

  echo "Mirroring $src_image to $dest_image..."
  skopeo copy --preserve-digests --remove-signatures --all --dest-tls-verify=false docker://"$src_image" docker://"$dest_image" || rc=$?
  sem_release
  return $rc
}

function mirror_image_to_archive() {
  sem_acquire
  local rc=0
  local src_image="$1"
  if [[ "${IS_CI_INDEX_IMAGE}" == "true" ]]; then
    for reg in registry.stage.redhat.io registry.redhat.io; do
      src_image="${src_image/$reg\/rhdh/quay.io\/rhdh}"
    done
    src_image="${src_image/registry-proxy.engineering.redhat.com\/rh-osbs\/rhdh-/quay.io\/rhdh\/}"
  fi
  local archive_path="$2"

  debugf "Saving $src_image to $archive_path..."
  skopeo copy --preserve-digests --remove-signatures --all --preserve-digests --dest-tls-verify=false docker://"$src_image" dir:"$archive_path" || rc=$?
  sem_release
  return $rc
}

function push_image_from_archive() {
  sem_acquire
  local rc=0
  local archive_path=$1
  local dest_image=$2
  echo "Pushing $archive_path to $dest_image..."
  skopeo copy --preserve-digests --remove-signatures --all --dest-tls-verify=false dir:"$archive_path" docker://"$dest_image" || rc=$?
  sem_release
  return $rc
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
cache_registry_urls

manifestsTargetDir="${TMPDIR}"
if [[ -n "${FROM_DIR}" ]]; then
  manifestsTargetDir="${FROM_DIR}"
fi

if [[ "${USE_OC_MIRROR}" = "true" ]]; then
  # oc-mirror v2 uses ${XDG_RUNTIME_DIR}/containers/auth.json by default for authentication

  NAMESPACE_CATALOGSOURCE="openshift-marketplace"
  ocMirrorLogFile="${TMPDIR}/oc-mirror.log.txt"
  
  if [[ -z "${FROM_DIR}" ]]; then
    # Direct to registry
    cat <<EOF >"${TMPDIR}/imageset-config.yaml"
apiVersion: mirror.openshift.io/v2alpha1
kind: ImageSetConfiguration
mirror:
  operators:
  - catalog: ${INDEX_IMAGE}
    full: false
    targetCatalog: rhdh-catalog
    packages:
      - name: rhdh
EOF
    if [[ "${NO_VERSION_FILTER}" != "true" ]]; then
      cat <<EOF >>"${TMPDIR}/imageset-config.yaml"
        channels:
          - name: fast
EOF
      for v in "${FILTERED_VERSIONS[@]}"; do
        cat <<EOF >>"${TMPDIR}/imageset-config.yaml"
          - name: fast-${v}
EOF
      done
    fi
    nbExtraImgs=${#EXTRA_IMAGES[@]}
    if [ "$nbExtraImgs" -ge 1 ]; then
      cat <<EOF >>"${TMPDIR}/imageset-config.yaml"
  additionalImages:
EOF
      for extraImg in "${EXTRA_IMAGES[@]}"; do
        cat <<EOF >>"${TMPDIR}/imageset-config.yaml"
  - name: "$extraImg"
EOF
      done
    fi

    if [[ -n "${TO_DIR}" ]]; then
      "${OC_MIRROR_PATH}" \
        --config="${TMPDIR}/imageset-config.yaml" \
        file://"${TO_DIR}" \
        --dest-tls-verify=false \
        --max-nested-paths=2 \
        "$OC_MIRROR_FLAGS" \
        --v2 |
        tee "${ocMirrorLogFile}"
      if [[ "${TO_DIR}" != "${TMPDIR}" ]]; then
        cp -f "${TMPDIR}/imageset-config.yaml" "${TO_DIR}/imageset-config.yaml"
      fi
      # targetCatalog needs to exist in the target registry. Copying a fake image..
      mirror_image_to_archive "registry.redhat.io/ubi9/ubi:latest" "${TO_DIR}/rhdh-catalog"
    fi
    if [[ -n "$TO_REGISTRY" ]]; then
      registryUrl="${CACHED_REGISTRY_URL}"
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
        --workspace file://"${TMPDIR}" \
        "docker://${registryUrl}" \
        --dest-tls-verify=false \
        --max-nested-paths=2 \
        "$OC_MIRROR_FLAGS" \
        --v2 |
        tee "${ocMirrorLogFile}"
    fi

  else
    # from dir
    if [ ! -d "${FROM_DIR}" ]; then
      errorf "Directory not found: ${FROM_DIR}"
      exit 1
    fi
    if [[ -n "${TO_REGISTRY}" ]]; then
      registryUrl="${CACHED_REGISTRY_URL}"
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
        --from file://"${FROM_DIR}" \
        "docker://${registryUrl}" \
        --dest-tls-verify=false \
        --max-nested-paths=2 \
        "$OC_MIRROR_FLAGS" \
        --v2 |
        tee "${ocMirrorLogFile}"
    fi
  fi

  # oc-mirror v2 generates manifests in working-dir/cluster-resources/
  # These are only generated during diskToMirror operations (when TO_REGISTRY is set)
  if [[ -n "${TO_REGISTRY}" ]]; then
    clusterResourcesDir="${TMPDIR}/working-dir/cluster-resources"
    if [[ -n "${FROM_DIR}" ]]; then
      clusterResourcesDir="${FROM_DIR}/working-dir/cluster-resources"
    fi
    
    if [ -d "${clusterResourcesDir}" ]; then
      debugf "Processing cluster resources from: ${clusterResourcesDir}"
      
      # Apply ImageDigestMirrorSet and ImageTagMirrorSet resources
      foundResources=false
      # oc-mirror v2 generates files with patterns: idms-*.yaml, itms-*.yaml
      
      for manifest in "${clusterResourcesDir}"/idms-*.yaml "${clusterResourcesDir}"/imageDigestMirrorSet*.yaml \
                      "${clusterResourcesDir}"/itms-*.yaml "${clusterResourcesDir}"/imageTagMirrorSet*.yaml; do
        if [[ -f "${manifest}" ]]; then
          debugf "Applying manifest: ${manifest}"
          if ! invoke_cluster_cli apply -f "${manifest}" 2>&1; then
            warnf "Failed to apply manifest: ${manifest}"
            if [[ "${IS_HOSTED_CONTROL_PLANE}" = "true" ]]; then
              warnf "This is expected on Hosted Control Plane clusters (ROSA HCP, HyperShift, etc.)."
              warnf "On HCP clusters, image mirrors must be configured in the HostedCluster resource on the management cluster."
              warnf "Please work with your cluster administrator to configure image mirroring."
            fi
          fi
          foundResources=true
        fi
      done
      
      # Process CatalogSource resources
      # oc-mirror v2 generates files with pattern: cs-*.yaml
      for manifest in "${clusterResourcesDir}"/cs-*.yaml "${clusterResourcesDir}"/catalogSource*.yaml; do
        if [[ -f "${manifest}" ]]; then
          debugf "Processing CatalogSource: ${manifest}"
          # Replace some metadata and add the default list of secrets
          "$YQ" -i '.metadata.name = "rhdh-catalog"' "${manifest}"
          "$YQ" -i '.spec.displayName = "Red Hat Developer Hub Catalog (Airgapped)"' "${manifest}"
          "$YQ" -i '.spec.secrets = (.spec.secrets // []) + ["internal-reg-auth-for-rhdh", "internal-reg-ext-auth-for-rhdh"]' "${manifest}"
          "$YQ" -i '.spec.image |= sub("default-route-openshift-image-registry\.apps\.[^/]+", "image-registry.openshift-image-registry.svc:5000")' "${manifest}"
          invoke_cluster_cli apply -f "${manifest}"
          foundResources=true
        fi
      done
      
      if [[ "${foundResources}" != "true" ]]; then
        warnf "No cluster resources found in ${clusterResourcesDir}. This may indicate that oc-mirror v2 did not generate them."
      fi
    else
      warnf "Cluster resources directory not found: ${clusterResourcesDir}. This may indicate that oc-mirror v2 did not generate cluster resources."
    fi
  fi
else
  if [[ -z "${FROM_DIR}" ]]; then
    mirror_extra_images &
    extra_mirror_pid=$!

    render_index
    process_bundles

    if ! wait "$extra_mirror_pid"; then
      errorf "mirror_extra_images failed"
      exit 1
    fi
  else
    mirror_extra_images_from_dir &
    extra_from_dir_pid=$!

    mirror_extra_images &
    extra_mirror_pid=$!

    process_bundles_from_dir

    if ! wait "$extra_from_dir_pid"; then
      errorf "mirror_extra_images_from_dir failed"
      exit 1
    fi
    if ! wait "$extra_mirror_pid"; then
      errorf "mirror_extra_images failed"
      exit 1
    fi
  fi

  # create OLM resources
  manifestsTargetDir="${TMPDIR}"
  if [[ -n "${FROM_DIR}" ]]; then
    manifestsTargetDir="${FROM_DIR}"
  fi

  # shellcheck disable=SC2016
  NAMESPACE_CATALOGSOURCE='$NAMESPACE_CATALOGSOURCE'
  # shellcheck disable=SC2016
  my_operator_index='$CATALOG_IMAGE'
  if [[ -n "${TO_REGISTRY}" ]]; then
      # It assumes that the user is also connected to a cluster
    detect_ocp_and_set_env_var
    if [[ "${IS_OPENSHIFT}" = "true" ]]; then
      debugf "Detected an OpenShift cluster"
      if ! command -v oc &>/dev/null; then
        errorf "Please install oc 4.10+ from an RPM or https://mirror.openshift.com/pub/openshift-v4/clients/ocp/"
        exit 1
      fi
      # Check we're logged into a cluster
      if ! oc whoami &>/dev/null; then
        errorf "Not logged into an OpenShift cluster"
        exit 1
      fi
    else
      if ! command -v oc &>/dev/null && ! command -v kubectl &>/dev/null; then
        errorf "Please install kubectl or oc 4.10+ (from an RPM or https://mirror.openshift.com/pub/openshift-v4/clients/ocp/)"
        exit 1
      fi
      debugf "Falling back to a standard K8s cluster"
      # Check that OLM is installed
      if ! invoke_cluster_cli get crd catalogsources.operators.coreos.com &>/dev/null; then
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

  cat <<EOF >"${manifestsTargetDir}/catalogSource.yaml"
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
    registry_url_internal="${CACHED_REGISTRY_URL}"
    cat <<EOF >"${manifestsTargetDir}/imageDigestMirrorSet.yaml"
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
          cat <<EOF >>"${manifestsTargetDir}/imageDigestMirrorSet.yaml"
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
          cat <<EOF >>"${manifestsTargetDir}/imageDigestMirrorSet.yaml"
    - mirrors:
      - ${registry_url_internal}/${targetImgLastTwo}
      source: ${targetImg}
EOF
        done
      fi
    fi
    # Iterate from the --extra-images passed on the CLI
    debugf "Extra images from CLI: " "${EXTRA_IMAGES[@]}"
    for img in "${EXTRA_IMAGES[@]}"; do
      if [[ "$img" == *"@sha256:"* ]]; then
        targetImg="${img%@*}"
      elif [[ "$img" == *":"* ]]; then
        targetImg="${img%:*}"
      else
        targetImg="${img}"
      fi
      targetImgLastTwo=$(extract_last_two_elements "$targetImg")
      cat <<EOF >>"${manifestsTargetDir}/imageDigestMirrorSet.yaml"
    - mirrors:
      - ${registry_url_internal}/${targetImgLastTwo}
      source: ${targetImg}
EOF
    done

    # Create the IDMS (OCP-specific) and CatalogSource.
    # IDMS resources never really worked on clusters with hosted control planes, and it looks like it is no longer
    # possible to create them in ROSA/HyperShift 4.18. So skipping the IDMS creation for such clusters.
    # More details in https://issues.redhat.com/browse/RHIDP-6684
    if [[ "${IS_OPENSHIFT}" == "true" ]] && [[ "${IS_HOSTED_CONTROL_PLANE}" != "true" ]]; then
      invoke_cluster_cli apply -f "${manifestsTargetDir}/imageDigestMirrorSet.yaml"
    fi
    debugf "Adding the internal cluster creds as pull secrets to be able to pull images from this internal registry by default"
    invoke_cluster_cli apply -f "${manifestsTargetDir}/catalogSource.yaml"
  fi
fi

# No longer possible to patch the OperatorHub resource on clusters with hosted control planes (ROSA 4.18).
# More details in https://issues.redhat.com/browse/OCPBUGS-43431?focusedId=26463911&page=com.atlassian.jira.plugin.system.issuetabpanels%3Acomment-tabpanel#comment-26463911
if [[ -n "${TO_REGISTRY}" ]] && [[ "${IS_OPENSHIFT}" == "true" ]] && [[ "${IS_HOSTED_CONTROL_PLANE}" != "true" ]]; then
  infof "Disabling the default Red Hat Ecosystem Catalog."
  invoke_cluster_cli patch OperatorHub cluster --type json \
    --patch '[{"op": "add", "path": "/spec/disableAllDefaultSources", "value": true}]'
fi

cat <<EOF >"${manifestsTargetDir}/namespace.yaml"
apiVersion: v1
kind: Namespace
metadata:
  name: ${NAMESPACE_OPERATOR}
EOF

cat <<EOF >"${manifestsTargetDir}/operatorGroup.yaml"
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: rhdh-operator-group
  namespace: ${NAMESPACE_OPERATOR}
EOF

cat <<EOF >"${manifestsTargetDir}/subscription.yaml"
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
  apiVersion: rhdh.redhat.com/v1alpha5
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
