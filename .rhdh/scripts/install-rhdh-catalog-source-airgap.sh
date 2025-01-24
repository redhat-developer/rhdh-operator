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
OLM_CHANNEL="fast"

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

# example usage:
# ./install-rhdh-catalog-source-airgap.sh \
#  [ --prod_operator_index "registry.redhat.io/redhat/redhat-operator-index:v4.14" \]
#  [ --prod_operator_package_name "rhdh" \ ]
#  [ --prod_operator_bundle_name "rhdh-operator" \ ]
#  [   --prod_operator_version "v1.1.0" \ ]
# [ --filter-versions "1.3,1.4" ]
# --from-dir /path/to/dir (to support mirroring from a bastion host)
# --to-dir /path/to/dir (to support exporting images to a dir, which can be transferred to the bastion host)
# --to-registry "$MY_MIRROR_REGISTRY" (either this or to-dir needs to specified, both can be specified)
# --install-operator <NAME>

function usage() {
  echo "
This script streamlines the installation of the RHDH Operator in a disconnected OpenShift or Kubernetes cluster.
The CatalogSource is created in the 'openshift-marketplace' namespace on OpenShift or 'olm' namespace on Kubernetes,
and is named 'operatorName-channelName', eg., rhdh-catalog

Usage:
  $0 [OPTIONS]

Options:
  --index-image <operator-index-image>   : Operator index image (default: registry.redhat.io/redhat/redhat-operator-index:v4.14)
  --filter-versions <list>               : Comma-separated list of operator minor versions to keep in the catalog (default: 1.3,1.4,1.5)
  --to-registry <registry_url>           : Mirror the images into the specified registry, assuming you are already logged into it. If this is not set and --to-dir is not set, it will attempt to use the builtin OCP registry if the target cluster is OCP. Otherwise, it will error out. It also assumes you are logged into the target disconnected cluster as well.
  --to-dir </path/to/dir>                : Mirror images into the specified directory. Needs to be an absolute path. This is useful if you are working in a fully disconnected environment and you must manually transfer the images to your network. From there, you will be able to re-run this script with '--from-dir' to push the images to your private registry.
  --from-dir </path/to/dir/with/images>  : Load images from the specified directory. Needs to be an absolute path. This is useful if you are working in a fully disconnected environment. In this case, you would use '--to-dir' first to mirror images to a specified directory, then transfer this dir over to your disconnected network. From there, you will be able to re-run this script with '--from-dir' to push the images to your private registry.
  --install-operator <true|false>        : Install the RHDH operator right after creating CatalogSource (default: false)
  --extra-images <list>                  : Comma-separated list of extra images to mirror

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
    --to-registry registry.example.com \\
    --install-operator true
"
}

INDEX_IMAGE="registry.redhat.io/redhat/redhat-operator-index:v4.17"
OPERATOR_NAME="rhdh-operator"

TO_REGISTRY=""
INSTALL_OPERATOR="false"
TO_DIR=""
FROM_DIR=""
FILTERED_VERSIONS=(1.3 1.4 1.5)
EXTRA_IMAGES=()

RELATED_IMAGES=()

while [[ "$#" -gt 0 ]]; do
  case $1 in
    '--index-image') INDEX_IMAGE="$2"; shift 1;;
    '--filter-versions') IFS=',' read -r -a FILTERED_VERSIONS <<< "$2"; shift 1;;
    '--extra-images') IFS=',' read -r -a EXTRA_IMAGES <<< "$2"; shift 1;;
    '--to-registry') TO_REGISTRY="$2"; shift 1;;
    '--to-dir') TO_DIR=$(realpath "$2"); shift 1;;
    '--from-dir') FROM_DIR="$2"; shift 1;;
    '--install-operator') INSTALL_OPERATOR="$2"; shift 1;;
    '-h'|'--help') usage; exit 0;;
    *) errorf "Unknown parameter is used: $1."; usage; exit 1;;
  esac
  shift 1
done

if [[ "$INSTALL_OPERATOR" != "true" && "$INSTALL_OPERATOR" != "false" ]]; then
  errorf "Invalid argument for --install-operator. Must be 'true' or 'false'".
  exit 1
fi

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
if [[ -n "${TO_REGISTRY}" && -n "${TO_DIR}" ]]; then
  errorf "--to-registry and --to-dir are mutually exclusive. Please specify only one of them."
  exit 1
fi
if [[ -z "${TO_REGISTRY}" && -z "${TO_DIR}" ]]; then
  errorf "Please specify either --to-registry or --to-dir (not both)."
  exit 1
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
  ## trap "rm -fr $TMPDIR || true" EXIT
fi
pushd "${TMPDIR}" > /dev/null
debugf ">>> WORKING DIR: $TMPDIR <<<"

function render_index() {
  set -euo pipefail

  mkdir -p "${TMPDIR}/rhdh/rhdh"
  local_index_file="${TMPDIR}/rhdh/rhdh/render.yaml"

  debugf "Rendering index image $INDEX_IMAGE as a local file: $local_index_file..."

  prod_operator_package_name="rhdh"
  prod_operator_name="${prod_operator_package_name}-operator"
  debugf "Fetching metadata for the ${prod_operator_package_name} operator catalog channel, packages, and bundles."
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

  # Filtering out to keep only the elements related to RHDH and to the versions selected
  opm render "${INDEX_IMAGE}" --output=yaml | \
    yq 'select(
        (.schema == "olm.package" and .name == "'${prod_operator_package_name}'")
        or
        (.schema == "olm.channel" and .package == "'${prod_operator_package_name}'" and .name == "fast")
        or
        '"$chanFilterList"'
        or
        '"$bundleFilterList"'
      )' | yq '.entries |= map(select('"$chanEntriesFilterList"'))' \
      > "${local_index_file}"

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
      targetImg="${TO_REGISTRY}/$(extract_last_two_elements "${img%@*}"):$imgDigest"
    elif [[ "$img" == *":"* ]]; then
      imgDir="./extraImages/${img%:*}/tag_$imgTag"
      imgTag="${img##*:}"
      targetImg="${TO_REGISTRY}/$(extract_last_two_elements "${img%:*}"):$imgTag"
    else
      imgDir="./extraImages/${img}/tag_latest"
      targetImg="${TO_REGISTRY}/$(extract_last_two_elements "${img}"):latest"
    fi

    if [[ -n "$TO_REGISTRY" ]]; then
      mirror_image_to_registry "$img" "$targetImg"
    else
      if [ ! -d "$imgDir" ]; then
        mkdir -p "${imgDir}"
        mirror_image_to_archive "$img" "$imgDir"
      fi
    fi
  done
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
      extraImg="$(extract_last_two_elements "${parent_path}"):${sha256_hash}"
      debugf "Extra-image: $extraImg"
      if [[ -n "$TO_REGISTRY" ]]; then
        targetImg="${TO_REGISTRY}/${extraImg%@*}"
        push_image_from_archive "$sha256_dir" "$targetImg"
      fi
    done

    # Iterate over all directories named "tag_*"
    find "$BASE_DIR" -type d -name "tag_*" | while read -r tag_dir; do
      relative_path=${tag_dir#"$BASE_DIR/"}
      tag_hash=${tag_dir##*/tag_}
      parent_path=$(dirname "$relative_path")
      debugf "parent_path: $parent_path"
      extraImg="$(extract_last_two_elements "${parent_path}"):${tag_hash}"
      debugf "Extra-image: $extraImg"
      if [[ -n "$TO_REGISTRY" ]]; then
        targetImg="${TO_REGISTRY}/${extraImg%:*}"
        push_image_from_archive "$tag_dir" "$targetImg"
      fi
    done
  fi
}

#useInternalOcpRegistry=""
#if [[ -z "${TO_REGISTRY}" && -z "${TO_DIR}" ]]; then
#  warnf "Both --to-registry and --to-dir not specified.
#The script will try to detect if it is currently connected to an OCP cluster, and if so, try to push images to the OCP internal registry.
#But it is generally recommended to be explicit and specify the mirror registry you want to use."
#  if ! is_openshift; then
#    errorf "Unable to determine if the current cluster is OCP. I don't know where to mirror the images to. Please specify either --to-registry or --to-dir (not both)."
#    exit 1
#  fi
#  useInternalOcpRegistry="true"
#  # TODO(rm3l): try to automatically use the internal cluster registry
#fi
#debugf "useInternalOcpRegistry=$useInternalOcpRegistry"

function process_related_images() {

  for bundleImg in $(grep -E '^image: .*operator-bundle' "${TMPDIR}/rhdh/rhdh/render.yaml" | awk '{print $2}' | uniq); do
    debugf "bundleImg=$bundleImg"
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
          all_related_images=()
          debugf "\t finding related images in $file to mirror..."
          images=$(grep -E 'image: ' "$file" | awk -F ': ' '{print $2}' | uniq)
          if [[ -n "$images" ]]; then
            all_related_images+=($images)
          fi
          # TODO(rm3l): we should use spec.relatedImages instead, but it seems to be incomplete in some bundles
          related_images=$(yq '.spec.install.spec.deployments[].spec.template.spec.containers[].env[] | select(.name | test("^RELATED_IMAGE_")).value' "$file" || true)
          if [[ -n "$related_images" ]]; then
            all_related_images+=($related_images)
          fi
          for relatedImage in "${all_related_images[@]}"; do
            relatedImageDigest="${relatedImage##*@sha256:}"
            imgDir="./images/${relatedImage%@*}/sha256_$relatedImageDigest"
            if [[ -n "$TO_REGISTRY" ]]; then
              targetImg="${TO_REGISTRY}/$(extract_last_two_elements "${relatedImage%@*}"):$relatedImageDigest"
              mirror_image_to_registry "$relatedImage" "$targetImg"
              debugf "replacing image refs in file '${file}'"
              sed -i 's#'$relatedImage'#'$targetImg'#g' "$file"
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
          newBundleImage="${TO_REGISTRY}/$(extract_last_two_elements "${bundleImg%@*}"):${digest}"
          debugf "\t Pushing updated bundle image: ./bundles/${digest}/src => ${newBundleImage}..."
          skopeo copy --dest-tls-verify=false "oci:./bundles/${digest}/src:latest" "docker://${newBundleImage}"

          sed -i "s#${bundleImg}#${newBundleImage}#g" "./rhdh/rhdh/render.yaml"
      fi
    fi
  done

  if [ ! -f "rhdh/rhdh.Dockerfile" ]; then
    debugf "\t Regenerating Dockerfile so the index can be rebuilt..."
    opm generate dockerfile rhdh/rhdh

    if [[ -n "$TO_REGISTRY" ]]; then
      infof "Building the catalog image locally."
      pushd "rhdh"
      my_operator_index="${TO_REGISTRY}/rhdh/index:latest"
      podman build -t "$my_operator_index" -f "./rhdh.Dockerfile" --no-cache .

      infof "Deploying your catalog image to the $my_operator_index registry."
      skopeo copy --src-tls-verify=false --dest-tls-verify=false --all "containers-storage:$my_operator_index" "docker://$my_operator_index"
      popd
    fi
  fi
}

function process_images_from_dir() {

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
        related_images=$(yq '.spec.install.spec.deployments[].spec.template.spec.containers[].env[] | select(.name | test("^RELATED_IMAGE_")).value' "$file" || true)
        if [[ -n "$related_images" ]]; then
          all_related_images+=($related_images)
        fi
        for relatedImage in "${all_related_images[@]}"; do
          relatedImageDigest="${relatedImage##*@sha256:}"
          imgDir="${FROM_DIR}/images/${relatedImage%@*}/sha256_$relatedImageDigest"
          if [ ! -d "$imgDir" ]; then
            warnf "Skipping related image $relatedImage not found mirrored in dir: $FROM_DIR/images"
            continue
          fi
          if [[ -n "$TO_REGISTRY" ]]; then
            targetImg="${TO_REGISTRY}/$(extract_last_two_elements "${relatedImage%@*}"):$relatedImageDigest"
            push_image_from_archive "$imgDir" "$targetImg"
            debugf "replacing image refs in file '${file}'"
            sed -i 's#'$relatedImage'#'$targetImg'#g' "$file"
          fi
        done
      fi
    done

    if [[ -n "$TO_REGISTRY" ]]; then
        # repack the image with the changes
        debugf "\t Repacking image ./bundles/${digest}/src => ./bundles/${digest}/unpacked..."
        umoci repack --image "${TMPDIR}/bundles/${digest}/src:latest" "${TMPDIR}/bundles/${digest}/unpacked"

        # Push the bundle to the mirror registry
        newBundleImage="${TO_REGISTRY}/$(extract_last_two_elements "${bundleImg%@*}"):${digest}"
        debugf "\t Pushing updated bundle image: ./bundles/${digest}/src => ${newBundleImage}..."
        skopeo copy --dest-tls-verify=false "oci:${TMPDIR}/bundles/${digest}/src:latest" "docker://${newBundleImage}"

        sed -i "s#${bundleImg}#${newBundleImage}#g" "${TMPDIR}/rhdh/rhdh/render.yaml"
    fi
  done

  if [[ -n "$TO_REGISTRY" ]]; then
    pushd "${TMPDIR}/rhdh"
    my_operator_index="${TO_REGISTRY}/rhdh/index:latest"
    debugf "Building the catalog image locally: $my_operator_index"
    podman build -t "$my_operator_index" -f "./rhdh.Dockerfile" --no-cache .

    debugf "Deploying your catalog image to the $my_operator_index registry."
    skopeo copy --src-tls-verify=false --dest-tls-verify=false --all "containers-storage:$my_operator_index" "docker://$my_operator_index"
    popd
  fi
}

function mirror_image_to_registry() {
  local src_image=$1
  local dest_image=$2
  echo "Mirroring $src_image to $dest_image..."
  skopeo copy --all --dest-tls-verify=false docker://"$src_image" docker://"$dest_image"
}

function mirror_image_to_archive() {
  local src_image=$1
  local archive_path=$2
  debugf "Saving $src_image to $archive_path..."
  skopeo copy --all --preserve-digests --dest-tls-verify=false docker://"$src_image" dir:"$archive_path"
}

function push_image_from_archive() {
  local archive_path=$1
  local dest_image=$2
  echo "Pushing $archive_path to $dest_image..."
  skopeo copy --all --dest-tls-verify=false dir:"$archive_path" docker://"$dest_image"
}

check_tool "yq"
check_tool "umoci"
check_tool "skopeo"
if [[ -n "$TO_REGISTRY" ]]; then
  check_tool "podman"
fi

if [[ -n "${TO_DIR}" ]]; then
  cp -vr "${SCRIPT_PATH}" "${TO_DIR}/install.sh"
fi

if [[ -z "${FROM_DIR}" ]]; then
  render_index
  process_related_images
else
  process_images_from_dir
  mirror_extra_images_from_dir
fi
mirror_extra_images

# create OLM resources
manifestsTargetDir="${TMPDIR}"
if [[ -n "${FROM_DIR}" ]]; then
  manifestsTargetDir="${FROM_DIR}"
fi

nsOperator="rhdh-operator"
cat <<EOF > "${manifestsTargetDir}/namespace.yaml"
apiVersion: v1
kind: Namespace
metadata:
  name: ${nsOperator}
EOF

cat <<EOF > "${manifestsTargetDir}/operatorGroup.yaml"
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: rhdh-operator-group
  namespace: ${nsOperator}
EOF

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
  my_operator_index="${TO_REGISTRY}/rhdh/index:latest"
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
  # Create this image pull secret if your mirror registry requires auth
  - reg-pull-secret
  publisher: "Red Hat"
  displayName: "Red Hat Developer Hub (Airgapped)"
EOF

cat <<EOF > "${manifestsTargetDir}/subscription.yaml"
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: rhdh-operator
  namespace: ${nsOperator}
spec:
  channel: fast
  installPlanApproval: Automatic
  name: rhdh
  source: rhdh-catalog
  sourceNamespace: ${NAMESPACE_CATALOGSOURCE}
EOF

if [[ -n "${TO_REGISTRY}" ]]; then
  # IDMS will only work on regular OCP clusters. It doesn't work on ROSA or clusters with hosted control planes like on IBM Cloud.
  cat <<EOF > "${manifestsTargetDir}/imageDigestMirrorSet.yaml"
apiVersion: config.openshift.io/v1
kind: ImageDigestMirrorSet
metadata:
  name: rhdh-idms
spec:
  imageDigestMirrors:
  - mirrors:
    - ${TO_REGISTRY}/rhel9/postgresql-15
    source: registry.redhat.io/rhel9/postgresql-15
  - mirrors:
    - ${TO_REGISTRY}/rhdh
    source: registry.redhat.io/rhdh
  - mirrors:
    - ${TO_REGISTRY}/openshift4/ose-kube-rbac-proxy
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
    - ${TO_REGISTRY}/${targetImgLastTwo}
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
    - ${TO_REGISTRY}/${targetImgLastTwo}
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
    - ${TO_REGISTRY}/${targetImgLastTwo}
    source: ${targetImg}
EOF
  done

  # Create the IDMS (OCP-specific) and CatalogSource
  if [[ "${IS_OPENSHIFT}" = "true" ]]; then
     invoke_cluster_cli apply -f "${manifestsTargetDir}/imageDigestMirrorSet.yaml"
  fi
  invoke_cluster_cli apply -f "${manifestsTargetDir}/catalogSource.yaml"
fi

cat <<EOF > "${manifestsTargetDir}/subscription.yaml"
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: rhdh-operator
  namespace: ${nsOperator}
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

      kubectl -n ${nsOperator} apply -f ${manifestsTargetDir}/namespace.yaml
      kubectl -n ${nsOperator} apply -f ${manifestsTargetDir}/operatorGroup.yaml
      kubectl -n ${nsOperator} apply -f ${manifestsTargetDir}/subscription.yaml
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

  if [[ "${IS_OPENSHIFT}" = "true" ]]; then
    OCP_CONSOLE_ROUTE_HOST=$(invoke_cluster_cli get route console -n openshift-console -o=jsonpath='{.spec.host}')
    CLUSTER_ROUTER_BASE=$(invoke_cluster_cli get ingress.config.openshift.io/cluster '-o=jsonpath={.spec.domain}')
    echo -n "

  To install, go to:
  https://${OCP_CONSOLE_ROUTE_HOST}/catalog/ns/${nsOperator}?catalogType=OperatorBackedService

  Or "
  else
    echo -n "

  To install on Kubernetes, run: "
  fi

  CLI_TOOL="kubectl"
  if [[ "${IS_OPENSHIFT}" = "true" ]]; then
    CLI_TOOL="oc"
  fi
  CR_EXAMPLE="
  cat <<EOF | ${CLI_TOOL} apply -f -
  apiVersion: rhdh.redhat.com/v1alpha3
  kind: Backstage
  metadata:
    name: developer-hub
    namespace: ${nsOperator}
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
  "

  if [[ "${IS_OPENSHIFT}" = "true" ]]; then
    echo "
  Once deployed, Developer Hub will be available at
  https://backstage-developer-hub-${nsOperator}.${CLUSTER_ROUTER_BASE}
  "
  fi
fi
