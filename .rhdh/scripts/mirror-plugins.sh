#!/bin/bash
#
# Script to mirror Red Hat Developer Hub dynamic plugin OCI artifacts for deployments in restricted environments.
# This script is installation-method agnostic and works with both operator and helm deployments
# on both OpenShift and Kubernetes platforms.
#
# Requires: skopeo, tar, jq, podman

set -euo pipefail

NC='\033[0m'

DEBUG=0

PLUGIN_INDEX=""
PLUGIN_LIST_FILE=""
PLUGIN_URLS=()
PLUGIN_IMAGES=()

TO_REGISTRY=""
INTERNAL_REGISTRY=""
TO_DIR=""
FROM_DIR=""

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
  if [[ $DEBUG -eq 1 ]]; then logf "DEBUG" "\033[0;90m" "$1"; fi
}

function errorf() {
  logf "ERROR" "\033[0;31m" "$1"
}

# Required tools and minimum versions (tested with):
#   skopeo >= 1.20    (for multi-arch image operations and manifest conversion)
#   tar >= 1.35       (GNU tar)
#   jq >= 1.7         (for JSON parsing and manipulation)
#   podman >= 5.6     (for building catalog index)
function check_tool() {
  if ! command -v "$1" >/dev/null; then
    echo "Error: Required tool '$1' is not installed." >&2
    exit 1
  fi
}

check_tool "skopeo"
check_tool "tar"
check_tool "jq"
check_tool "podman"

function usage() {
  echo "
Red Hat Developer Hub - Dynamic Plugin OCI Artifact Mirroring Script

This script mirrors dynamic plugin OCI artifacts for RHDH deployments in restricted environments.
It is installation-method agnostic and works with both operator and helm deployments.

Requirements:
  skopeo >= 1.20, tar >= 1.35, jq >= 1.7, podman >= 5.6

Usage:
  $0 [OPTIONS]

Options:
  --plugin-index <oci-url>               : Plugin catalog index to query for version-specific plugins
                                           (e.g., oci://quay.io/rhdh/plugin-catalog-index:1.9)
  --plugin-list <file>                   : Local .txt file with plugin OCI references (oci:// URL per line,
                                           comments with '#' are ignored, no quotes needed in file)
  --plugins <oci-url> [<oci-url> ...]    : Space-separated list of plugin OCI URLs to mirror
                                           Note: URLs containing '!' must be quoted (e.g., 'oci://...:tag!subpath')
  --to-registry <registry_url>           : Mirror the plugins to the specified registry
                                           (assumes you are already logged in)
  --internal-registry <registry_url>     : Registry URL used inside the catalog index for plugin references.
                                           Use this when the push registry differs from the in-cluster address.
                                           For OCP internal registry, use: image-registry.openshift-image-registry.svc:5000
                                           Defaults to the --to-registry value if not specified.
  --to-dir </absolute/path/to/dir>       : Mirror plugins to the specified directory (for fully disconnected environments)
                                           This directory can be transferred to a disconnected network
  --from-dir </absolute/path/to/dir>     : Load plugins from the specified directory and push to registry
                                           (for use in disconnected environments after transferring the directory)
  -h, --help                             : Show this help message

Examples:

  # Mirror all plugins from a catalog index to a registry
  # Substitute with your registry, e.g.:
  #   --to-registry localhost:5000
  #   --to-registry quay.io/myorg
  #   --to-registry default-route-openshift-image-registry.apps.<cluster-domain>
  $0 \\
    --plugin-index oci://quay.io/rhdh/plugin-catalog-index:1.9 \\
    --to-registry registry.example.com

  # Mirror specific plugins by direct OCI reference (use quotes for URLs with '!')
  $0 \\
    --plugins 'oci://quay.io/rhdh-plugin-catalog/backstage-community-plugin-quay:1.8.0--1.22.1!backstage-community-plugin-quay' \\
              'oci://quay.io/rhdh-plugin-catalog/backstage-community-plugin-github-actions:1.8.0--0.11.1!backstage-community-plugin-github-actions' \\
    --to-registry registry.example.com

  # Combined mode: catalog index + custom plugins
  $0 \\
    --plugin-index oci://quay.io/rhdh/plugin-catalog-index:1.9 \\
    --plugins 'oci://custom-registry.example.com/my-plugin:1.0!my-plugin' \\
    --to-registry registry.example.com

  # Export plugins to directory (for fully disconnected environments)
  $0 \\
    --plugin-index oci://quay.io/rhdh/plugin-catalog-index:1.9 \\
    --to-dir /path/to/export

  # Import plugins from directory and push to registry (in disconnected environment)
  $0 \\
    --from-dir /path/to/export \\
    --to-registry registry.example.com

  # Mirror specific plugins from a file list
  $0 \\
    --plugin-list /path/to/plugins.txt \\
    --to-registry registry.example.com

  # Mirror plugins to OCP internal registry with in-cluster address for catalog references
  # --to-registry is the external route used for pushing images
  # --internal-registry is the in-cluster service address that RHDH uses to pull plugins
  $0 \\
    --plugin-index oci://quay.io/rhdh/plugin-catalog-index:1.9 \\
    --to-registry default-route-openshift-image-registry.apps.example.com \\
    --internal-registry image-registry.openshift-image-registry.svc:5000

  Example plugins.txt content:
    # Red Hat Developer Hub Plugin List
    oci://quay.io/rhdh-plugin-catalog/backstage-community-plugin-quay:1.8
    oci://quay.io/rhdh-plugin-catalog/backstage-community-plugin-github-actions:1.7
    oci://quay.io/rhdh-plugin-catalog/backstage-community-plugin-azure-devops:1.8
    oci://quay.io/rhdh-plugin-catalog/backstage-community-plugin-dynatrace:1.8.0--10.6.0!backstage-community-plugin-dynatrace
    # 3-level ghcr.io references are also supported
    oci://ghcr.io/redhat-developer/rhdh-plugin-export-overlays/backstage-community-plugin-scaffolder-backend-module-quay:bs_1.45.3__2.14.0
"
}

# Parse command line arguments
while [[ "$#" -gt 0 ]]; do
  case $1 in
  '--plugin-index')
    PLUGIN_INDEX="$2"
    shift 1
    ;;
  '--plugin-list')
    PLUGIN_LIST_FILE=$(realpath "$2")
    shift 1
    ;;
  '--plugins')
    shift
    # Collect all plugin URLs until we hit another flag or end of arguments
    while [[ "$#" -gt 0 ]] && [[ "$1" != --* ]]; do
      PLUGIN_URLS+=("$1")
      shift
    done
    continue
    ;;
  '--to-registry')
    TO_REGISTRY="$2"
    shift 1
    ;;
  '--internal-registry')
    INTERNAL_REGISTRY="$2"
    shift 1
    ;;
  '--to-dir')
    TO_DIR=$(realpath "$2")
    shift 1
    ;;
  '--from-dir')
    FROM_DIR=$(realpath "$2")
    shift 1
    ;;
  '-h' | '--help')
    usage
    exit 0
    ;;
  *)
    errorf "Unknown option: $1"
    usage
    exit 1
    ;;
  esac
  shift 1
done

# Validate that at least one input source is specified (unless using --from-dir)
if [[ -z "${FROM_DIR}" ]]; then
  if [[ -z "$PLUGIN_INDEX" && -z "$PLUGIN_LIST_FILE" && ${#PLUGIN_URLS[@]} -eq 0 ]]; then
    errorf "No plugin source specified. Use --plugin-index, --plugin-list, or --plugins to specify plugins to mirror."
    usage
    exit 1
  fi
fi

# Validate plugin index format
if [[ -n "$PLUGIN_INDEX" && ! "$PLUGIN_INDEX" =~ ^oci:// ]]; then
  errorf "Plugin index must be in OCI format: oci://registry/org/image:tag or oci://registry/org/image@sha256:digest"
  exit 1
fi

# Validate plugin URLs format
for url in "${PLUGIN_URLS[@]}"; do
  if [[ ! "$url" =~ ^oci:// ]]; then
    errorf "Plugin URL must be in OCI format: oci://registry/org/image:tag or oci://registry/org/image@sha256:digest. Got: $url"
    exit 1
  fi
done

# Validate that either --to-registry or --to-dir is specified (but not both)
if [[ -z "${TO_REGISTRY}" && -z "${TO_DIR}" ]]; then
  if [[ -n "${FROM_DIR}" ]]; then
    errorf "--to-registry is required when using --from-dir"
  else
    errorf "Either --to-registry or --to-dir must be specified"
  fi
  exit 1
fi

if [[ -n "${TO_REGISTRY}" && -n "${TO_DIR}" ]]; then
  errorf "--to-registry and --to-dir are mutually exclusive. Please specify only one."
  exit 1
fi

if [[ -n "${FROM_DIR}" && -n "${TO_DIR}" ]]; then
  errorf "--from-dir and --to-dir are mutually exclusive. Please specify only one."
  exit 1
fi

if [[ -n "${FROM_DIR}" && ! -d "${FROM_DIR}" ]]; then
  errorf "Directory not found: ${FROM_DIR}"
  exit 1
fi

# Default INTERNAL_REGISTRY to TO_REGISTRY if not specified
# INTERNAL_REGISTRY is the address used inside catalog index references (what RHDH sees in-cluster).
# TO_REGISTRY is the address used for pushing images (the external route).
if [[ -z "${INTERNAL_REGISTRY}" && -n "${TO_REGISTRY}" ]]; then
  INTERNAL_REGISTRY="${TO_REGISTRY}"
fi

# Setup working directory
# Capture original directory for saving user-facing files
ORIGINAL_DIR="$(pwd)"

if [[ -n "${TO_DIR}" ]]; then
  mkdir -p "${TO_DIR}"
  TMPDIR="${TO_DIR}"
else
  TMPDIR=$(mktemp -d)
  # shellcheck disable=SC2064
  trap "rm -fr $TMPDIR || true" EXIT
fi

pushd "${TMPDIR}" >/dev/null
debugf "Working directory: $TMPDIR"

# Extract the last two path elements from an image URL (e.g., org/image from registry.io/org/image).
# This ensures compatibility with OCP internal registry which only supports 2-level paths (namespace/image).
# For 3-level sources like ghcr.io/org/sub/image, this returns sub/image.
# For 2-level sources like registry.access.redhat.com/org/image, this returns org/image (unchanged).
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

# Fallback registry for unreleased plugins
# When plugins reference registry.access.redhat.com/rhdh but aren't released yet,
# fall back to quay.io/rhdh where development builds are available
FALLBACK_SOURCE_REGISTRY="quay.io"
PRIMARY_SOURCE_REGISTRY="registry.access.redhat.com"

# Global variables set by get_effective_registry_ref
EFFECTIVE_REF=""
USED_FALLBACK=0

# Check if an image exists in a registry using skopeo inspect
# Returns 0 if image exists, 1 otherwise
# This is more reliable than parsing error strings from skopeo copy
function check_image_exists() {
  local docker_ref="$1"
  skopeo inspect "docker://$docker_ref" &>/dev/null
  return $?
}

# Get the effective registry reference, trying fallback if primary doesn't exist
# Sets global EFFECTIVE_REF to the working registry reference
# Sets global USED_FALLBACK=1 if fallback was used
# Returns 0 on success, 1 on failure
function get_effective_registry_ref() {
  local docker_ref="$1"
  EFFECTIVE_REF=""
  USED_FALLBACK=0
  
  # Check if the image exists at the primary location
  if check_image_exists "$docker_ref"; then
    EFFECTIVE_REF="$docker_ref"
    return 0
  fi
  
  # If the source is registry.access.redhat.com/rhdh, try fallback to quay.io/rhdh
  if [[ "$docker_ref" == "${PRIMARY_SOURCE_REGISTRY}/rhdh/"* ]]; then
    local fallback_ref="${docker_ref/${PRIMARY_SOURCE_REGISTRY}/${FALLBACK_SOURCE_REGISTRY}}"
    debugf "Primary registry image not found, checking fallback: $fallback_ref"
    
    if check_image_exists "$fallback_ref"; then
      warnf "Image not found at ${PRIMARY_SOURCE_REGISTRY}, using fallback: ${FALLBACK_SOURCE_REGISTRY}"
      EFFECTIVE_REF="$fallback_ref"
      USED_FALLBACK=1
      return 0
    fi
    
    # Neither primary nor fallback exists
    errorf "Image not found at both ${PRIMARY_SOURCE_REGISTRY} and ${FALLBACK_SOURCE_REGISTRY}"
    return 1
  fi
  
  # For non-RHDH registries, just report the failure
  return 1
}

# Copy OCI image using skopeo
# Supports both registry (docker://) and directory (dir://) destinations
# Falls back to quay.io/rhdh if registry.access.redhat.com/rhdh doesn't exist (unreleased plugins)
function mirror_image() {
  local src_image="$1"
  local dest="$2"
  
  # Strip OCI subpath (!suffix) if present, then strip oci:// prefix if present
  local docker_ref="${src_image%!*}"
  docker_ref="${docker_ref#oci://}"
  
  # Check if image exists and get effective registry reference (with fallback if needed)
  if ! get_effective_registry_ref "$docker_ref"; then
    errorf "Failed to find image: $docker_ref"
    return 1
  fi
  local effective_ref="$EFFECTIVE_REF"
  
  # Build skopeo flags as arrays to prevent word-splitting issues
  local -a skopeo_flags=(--remove-signatures --all)
  
  if [[ "$dest" == docker://* ]]; then
    skopeo_flags+=(--dest-tls-verify=false)
    # Don't preserve digests for registry destinations to allow format conversion
    # This ensures compatibility with registries that require manifest format conversion
    # (e.g., OpenShift internal registry requiring OCI format)
    infof "Mirroring $src_image to ${dest#docker://}..."
  else
    # Preserve digests for directory destinations (offline transfer integrity)
    skopeo_flags+=(--preserve-digests)
    debugf "Saving $src_image to ${dest#dir:}..."
  fi
  
  # Copy using the effective reference (primary or fallback)
  if skopeo copy "${skopeo_flags[@]}" "docker://$effective_ref" "$dest"; then
    if [[ "$USED_FALLBACK" -eq 1 ]]; then
      infof "Successfully mirrored using fallback registry: ${FALLBACK_SOURCE_REGISTRY}"
    fi
    return 0
  else
    errorf "Failed to mirror $effective_ref"
    return 1
  fi
}

function push_image_from_archive() {
  local archive_path="$1"
  local dest_image="$2"
  
  infof "Pushing $archive_path to $dest_image..."
  # Don't preserve digests when pushing to registry to allow format conversion
  # This ensures compatibility with registries that require manifest format conversion
  skopeo copy --remove-signatures --all --dest-tls-verify=false "dir:$archive_path" "docker://$dest_image" || return 1
  return 0
}

# Download the catalog index, extract the index.json file,
# and return the list of plugin OCI URLs it contains
function resolve_plugin_index() {
  local index_url="$1"
  infof "Resolving plugins from catalog index: $index_url"
  
  # Strip oci:// prefix
  if [[ "$index_url" =~ ^oci://(.+) ]]; then
    local registry_ref="${BASH_REMATCH[1]}"
    
    debugf "Extracting plugin catalog index image: $registry_ref"
    
    # Check if catalog index exists and get effective registry reference (with fallback if needed)
    if ! get_effective_registry_ref "$registry_ref"; then
      errorf "Failed to find catalog index image: $registry_ref"
      return 1
    fi
    local effective_ref="$EFFECTIVE_REF"
    
    # Create temporary directory for extracting the catalog index
    local temp_dir
    temp_dir=$(mktemp -d)
    
    # Cleanup temp directory when function returns
    # shellcheck disable=SC2064
    trap "rm -rf '$temp_dir'" RETURN
    
    # Extract the catalog index image using effective reference
    if ! skopeo copy "docker://$effective_ref" "dir:$temp_dir/catalog-index" 2>/dev/null; then
      errorf "Failed to extract catalog index image: $effective_ref"
      return 1
    fi
    
    if [[ "$USED_FALLBACK" -eq 1 ]]; then
      infof "Using catalog index from fallback registry: ${FALLBACK_SOURCE_REGISTRY}"
    fi
    
    # Extract all layers to find index.json
    local catalog_data_dir="$temp_dir/catalog-data"
    mkdir -p "$catalog_data_dir"
    
    # Extract all layers to get all catalog content (index.json, dynamic-plugins, catalog-entities, etc.)
    local found_index=false
    for layer in "$temp_dir/catalog-index"/*; do
      if [[ -f "$layer" ]] && [[ ! "$layer" =~ (manifest\.json|version)$ ]]; then
        debugf "Extracting layer: $(basename "$layer")"
        tar -xf "$layer" -C "$catalog_data_dir" 2>/dev/null || true
      fi
    done

    if [[ -f "$catalog_data_dir/index.json" ]]; then
      found_index=true
      debugf "Found index.json in extracted catalog data"
    fi

    if [[ "$found_index" != "true" ]]; then
      errorf "No index.json found in catalog index image"
      return 1
    fi
    
    # Parse index.json to extract plugin references
    local index_file="$catalog_data_dir/index.json"
    debugf "Parsing index.json for plugin references"
    
    # Check if jq is available
    if ! command -v jq &> /dev/null; then
      errorf "jq is required to parse index.json but is not installed"
      return 1
    fi
    
    # Extract registryReference values from index.json and convert to oci:// format
    local temp_plugin_images=()
    
    while IFS= read -r registry_ref; do
      if [[ -n "$registry_ref" ]]; then
        local oci_url="oci://$registry_ref"
        temp_plugin_images+=("$oci_url")
      fi
    done < <(jq -r '.[] | .registryReference // empty' "$index_file" 2>/dev/null)
    
    # Remove duplicates and sort
    if [[ ${#temp_plugin_images[@]} -gt 0 ]]; then
      mapfile -t PLUGIN_IMAGES < <(printf "%s\n" "${temp_plugin_images[@]}" | sort -u)
    fi
    
    infof "Found ${#PLUGIN_IMAGES[@]} unique plugins from catalog index"
    
    return 0
  else
    errorf "Invalid OCI URL format: $index_url. Expected format: oci://registry/org/image:tag or oci://registry/org/image@sha256:digest"
    return 1
  fi
}

# Rebuild and mirror the catalog index with updated registry references
# This ensures the catalog index works in disconnected environments
function mirror_catalog_index() {
  local index_url="$1"
  local target_registry="$2"
  local target_dir="$3"
  local internal_registry="${4:-$target_registry}"
  
  infof "Preparing catalog index for mirroring: $index_url"
  
  # Strip oci:// prefix
  if [[ ! "$index_url" =~ ^oci://(.+) ]]; then
    errorf "Invalid catalog index URL format: $index_url"
    return 1
  fi
  
  local registry_ref="${BASH_REMATCH[1]}"
  local original_registry
  local catalog_name
  local catalog_tag
  
  # Parse the registry reference
  if [[ "$registry_ref" =~ ^([^/]+)/(.+)@sha256:(.+)$ ]]; then
    original_registry="${BASH_REMATCH[1]}"
    catalog_name="${BASH_REMATCH[2]}"
    local original_digest="${BASH_REMATCH[3]}"
    catalog_tag="sha256-${original_digest}"
  elif [[ "$registry_ref" =~ ^([^/]+)/(.+):([^:@]+)$ ]]; then
    original_registry="${BASH_REMATCH[1]}"
    catalog_name="${BASH_REMATCH[2]}"
    catalog_tag="${BASH_REMATCH[3]}"
  else
    warnf "Could not parse catalog index reference, will mirror as-is: $registry_ref"
    catalog_tag="latest"
  fi

  # Flatten catalog_name to last 2 path elements for OCP internal registry compatibility
  # e.g., org/sub/plugin-catalog-index -> sub/plugin-catalog-index
  if [[ -n "$catalog_name" ]]; then
    catalog_name=$(extract_last_two_elements "$catalog_name")
  fi

  debugf "Original registry: $original_registry, Catalog: $catalog_name, Tag: $catalog_tag"
  
  # Check if catalog index exists and get effective registry reference (with fallback if needed)
  if ! get_effective_registry_ref "$registry_ref"; then
    errorf "Failed to find catalog index image: $registry_ref"
    return 1
  fi
  local effective_ref="$EFFECTIVE_REF"
  
  # Create temporary directory for catalog index work
  local temp_dir
  temp_dir=$(mktemp -d)
  # shellcheck disable=SC2064
  trap "rm -rf '$temp_dir'" RETURN
  
  # Extract the catalog index image using effective reference
  infof "Extracting catalog index image..."
  if ! skopeo copy "docker://$effective_ref" "dir:$temp_dir/catalog-index" 2>/dev/null; then
    errorf "Failed to extract catalog index image: $effective_ref"
    return 1
  fi
  
  if [[ "$USED_FALLBACK" -eq 1 ]]; then
    infof "Using catalog index from fallback registry: ${FALLBACK_SOURCE_REGISTRY}"
  fi
  
  # Extract layers to find and modify index.json
  local catalog_data_dir="$temp_dir/catalog-data"
  mkdir -p "$catalog_data_dir"
  
  local found_index=false

  # Extract all layers to get all catalog content (index.json, dynamic-plugins, catalog-entities, etc.)
  for layer in "$temp_dir/catalog-index"/*; do
    if [[ -f "$layer" ]] && [[ ! "$layer" =~ (manifest\.json|version)$ ]]; then
      debugf "Extracting layer: $(basename "$layer")"
      tar -xf "$layer" -C "$catalog_data_dir" 2>/dev/null || true
    fi
  done

  if [[ -f "$catalog_data_dir/index.json" ]]; then
    found_index=true
    debugf "Found index.json in extracted catalog data"
  fi

  if [[ "$found_index" != "true" ]]; then
    warnf "No index.json found in catalog index, mirroring as-is without modifications"
    # Mirror the original catalog index without modifications
    if [[ -n "$target_registry" ]]; then
      local target_image="$target_registry/$catalog_name:$catalog_tag"
      infof "Mirroring catalog index to: $target_image"
      skopeo copy --all --dest-tls-verify=false "docker://$registry_ref" "docker://$target_image" || return 1
    elif [[ -n "$target_dir" ]]; then
      local catalog_dir="$target_dir/catalog-index"
      mkdir -p "$catalog_dir"
      infof "Saving catalog index to: $catalog_dir"
      skopeo copy --all "docker://$registry_ref" "dir:$catalog_dir" || return 1
      # Save metadata for later use
      echo "$index_url" > "$catalog_dir/original-url.txt"
    fi
    return 0
  fi
  
  # Update index.json with new registry references
  local index_file="$catalog_data_dir/index.json"
  local updated_index="$temp_dir/index-updated.json"
  
  if [[ -n "$target_registry" ]]; then
    infof "Updating plugin registry references in index.json..."
    
    # Use jq to update all registryReference values
    # Keep only the last 2 path elements to ensure compatibility with OCP internal
    # registry (2-level paths). Works for both 2-level and 3-level source paths.
    if ! jq --arg target_reg "$internal_registry" '
      . | with_entries(
        .value.registryReference |= (
          if . then
            (split("/") | .[-2:] | join("/")) as $last_two |
            ($target_reg + "/" + $last_two)
          else
            .
          end
        )
      )
    ' "$index_file" > "$updated_index" 2>/dev/null || [[ ! -s "$updated_index" ]]; then
      errorf "Failed to update index.json with new registry references"
      return 1
    fi

    debugf "Updated $(jq '. | length' "$updated_index") plugin references in index.json"

    # Replace the original index.json with updated version
    cp "$updated_index" "$catalog_data_dir/index.json"

    # Update OCI references in dynamic-plugins.default.yaml
    infof "Updating OCI references in dynamic-plugins.default.yaml..."
    if [[ -f "$catalog_data_dir/dynamic-plugins.default.yaml" ]]; then
      # Replace OCI registry references, keeping only the last 2 path elements
      # to ensure compatibility with OCP internal registry (2-level paths).
      # Pattern: oci://REG/[extra/]ns/image:TAG -> oci://INTERNAL_REGISTRY/ns/image:TAG
      sed -i -E "s|oci://[^/]+(/[^/]+)*(/[^/]+/[^[:space:]\"']+)|oci://$internal_registry\2|g" "$catalog_data_dir/dynamic-plugins.default.yaml"
      debugf "Updated OCI references in dynamic-plugins.default.yaml"
      infof "=== dynamic-plugins.default.yaml after update ==="
      cat "$catalog_data_dir/dynamic-plugins.default.yaml"
      infof "=== end dynamic-plugins.default.yaml ==="
    fi

    # Update OCI references in all catalog-entities YAML files
    infof "Updating OCI references in catalog-entities..."
    local yaml_count=0
    while IFS= read -r yaml_file; do
      if [[ -n "$yaml_file" && -f "$yaml_file" ]]; then
        sed -i -E "s|oci://[^/]+(/[^/]+)*(/[^/]+/[^[:space:]\"']+)|oci://$internal_registry\2|g" "$yaml_file"
        ((yaml_count++)) || true
      fi
    done < <(find "$catalog_data_dir/catalog-entities" -name "*.yaml" -type f 2>/dev/null)

    if [[ $yaml_count -gt 0 ]]; then
      debugf "Updated OCI references in $yaml_count catalog-entity YAML files"
    fi

    # Rebuild the layer with updated index.json
    infof "Rebuilding catalog index image with updated references..."
    local new_layer="$temp_dir/new-layer.tar"
    tar -cf "$new_layer" -C "$catalog_data_dir" . 2>/dev/null
    
    # Create a Dockerfile to rebuild the image
    local build_dir="$temp_dir/build"
    mkdir -p "$build_dir"
    mkdir -p "$build_dir/content"
    
    # Copy all files from catalog_data_dir to build context
    cp -r "$catalog_data_dir"/* "$build_dir/content/"
    
    cat > "$build_dir/Dockerfile" << 'EOF'
FROM scratch
COPY content/ /
EOF
    
    debugf "Using podman to rebuild catalog index image"
    
    local temp_image_tag="localhost/temp-catalog-index:$catalog_tag"
    local target_image="$target_registry/$catalog_name:$catalog_tag"
    
    if ! podman build -t "$temp_image_tag" "$build_dir" &>/dev/null; then
      errorf "Failed to rebuild catalog index image with podman"
      return 1
    fi
    
    infof "Pushing rebuilt catalog index to: $target_image"
    if ! skopeo copy --all --dest-tls-verify=false "containers-storage:$temp_image_tag" "docker://$target_image" 2>&1; then
      errorf "Failed to push catalog index to registry"
      return 1
    fi
    
    # Clean up local image
    podman rmi "$temp_image_tag" &>/dev/null || true
    
    infof "Successfully mirrored catalog index with updated plugin references"
    
  elif [[ -n "$target_dir" ]]; then
    # Save catalog index content for later rebuilding
    local catalog_dir="$target_dir/catalog-index"
    mkdir -p "$catalog_dir"
    
    # Save the extracted content (index.json, dynamic-plugins, catalog-entities, etc.)
    cp -r "$catalog_data_dir"/* "$catalog_dir/"
  fi
  
  return 0
}

# Process a file containing one OCI URL per line,
# ignoring comments and blank lines, and return a list of valid plugin URLs
function load_plugin_list_from_file() {
  local file_path="$1"
  infof "Loading plugin list from file: $file_path"
  
  if [[ ! -f "$file_path" ]]; then
    errorf "Plugin list file not found: $file_path"
    return 1
  fi
  
  # Read plugin references from file (one per line, skip comments and empty lines)
  local temp_plugins=()
  while IFS= read -r line; do
    # Skip comments and empty lines
    [[ "$line" =~ ^[[:space:]]*# ]] && continue
    [[ -z "${line// }" ]] && continue
    
    # Trim whitespace
    line="${line#"${line%%[! ]*}"}"
    line="${line%"${line##*[! ]}"}"
    
    if [[ -n "$line" ]]; then
      # Validate OCI URL format
      if [[ ! "$line" =~ ^oci:// ]]; then
        warnf "Skipping invalid plugin URL (must start with oci://): $line"
        continue
      fi
      temp_plugins+=("$line")
    fi
  done < "$file_path"
  
  if [[ ${#temp_plugins[@]} -eq 0 ]]; then
    warnf "No valid plugin references found in file: $file_path"
    return 1
  fi
  
  PLUGIN_IMAGES=("${temp_plugins[@]}")
  infof "Loaded ${#PLUGIN_IMAGES[@]} plugins from file"
  return 0
}

# Main plugin mirroring logic
function mirror_plugins() {
  infof "Starting plugin artifact mirroring..."
  
  # Resolve plugin list from all sources
  local all_plugins=()
  
  # Add plugins from catalog index
  if [[ -n "$PLUGIN_INDEX" ]]; then
    debugf "Resolving plugins from index: $PLUGIN_INDEX"
    local temp_plugins=()
    if resolve_plugin_index "$PLUGIN_INDEX"; then
      temp_plugins=("${PLUGIN_IMAGES[@]}")
      all_plugins+=("${temp_plugins[@]}")
      debugf "Added ${#temp_plugins[@]} plugins from catalog index"
    else
      errorf "Failed to resolve plugin index: $PLUGIN_INDEX"
      return 1
    fi
  fi
  
  # Add plugins from file
  if [[ -n "$PLUGIN_LIST_FILE" ]]; then
    debugf "Loading plugins from file: $PLUGIN_LIST_FILE"
    local temp_plugins=()
    if load_plugin_list_from_file "$PLUGIN_LIST_FILE"; then
      temp_plugins=("${PLUGIN_IMAGES[@]}")
      all_plugins+=("${temp_plugins[@]}")
      debugf "Added ${#temp_plugins[@]} plugins from file"
    else
      errorf "Failed to load plugin list from file: $PLUGIN_LIST_FILE"
      return 1
    fi
  fi
  
  # Add plugins from direct URLs
  if [[ ${#PLUGIN_URLS[@]} -gt 0 ]]; then
    debugf "Adding ${#PLUGIN_URLS[@]} plugins from direct URLs"
    all_plugins+=("${PLUGIN_URLS[@]}")
  fi
  
  # Check if we have any plugins
  if [[ ${#all_plugins[@]} -eq 0 ]]; then
    errorf "No plugins found to mirror"
    return 1
  fi
  
  # Deduplicate plugins (remove duplicates while preserving order)
  PLUGIN_IMAGES=()
  local seen_plugins=()
  for plugin in "${all_plugins[@]}"; do
    # Check if we've seen this plugin before
    local is_duplicate=false
    for seen in "${seen_plugins[@]}"; do
      if [[ "$plugin" == "$seen" ]]; then
        is_duplicate=true
        break
      fi
    done
    
    if [[ "$is_duplicate" == "false" ]]; then
      PLUGIN_IMAGES+=("$plugin")
      seen_plugins+=("$plugin")
    else
      debugf "Skipping duplicate plugin: $plugin"
    fi
  done
  
  infof "Total unique plugins to mirror: ${#PLUGIN_IMAGES[@]}"
  
  # Mirror each plugin
  local success_count=0
  local failure_count=0
  
  for img in "${PLUGIN_IMAGES[@]}"; do
    debugf "Processing plugin: $img"
    
    # Remove oci:// prefix for processing
    local img_no_prefix="${img#oci://}"
    
    # Determine target image name and directory structure
    local imgDir
    local targetImg
    local lastTwo
    
    if [[ "$img_no_prefix" == *"@sha256:"* ]]; then
      local imgDigest="${img_no_prefix##*@sha256:}"
      imgDir="./plugins/${img_no_prefix%@*}/sha256_$imgDigest"
      lastTwo=$(extract_last_two_elements "${img_no_prefix%@*}")
      targetImg="${TO_REGISTRY}/${lastTwo}@sha256:${imgDigest}"
    elif [[ "$img_no_prefix" == *":"* ]]; then
      local imgTag="${img_no_prefix##*:}"
      imgDir="./plugins/${img_no_prefix%:*}/tag_$imgTag"
      lastTwo=$(extract_last_two_elements "${img_no_prefix%:*}")
      # Strip OCI subpath (everything after !) from tag for registry compatibility
      local clean_tag="${imgTag%%!*}"
      targetImg="${TO_REGISTRY}/${lastTwo}:$clean_tag"
    else
      imgDir="./plugins/${img_no_prefix}/tag_latest"
      lastTwo=$(extract_last_two_elements "${img_no_prefix}")
      targetImg="${TO_REGISTRY}/${lastTwo}:latest"
    fi
    
    # Mirror to registry or directory
    if [[ -n "$TO_REGISTRY" ]]; then
      set +e
      mirror_image "$img" "docker://$targetImg"
      ret=$?
      set -e
      if [ $ret -eq 0 ]; then
        ((success_count++)) || true
      else
        ((failure_count++)) || true
        warnf "Failed to mirror plugin: $img"
      fi
    else
      if [ ! -d "$imgDir" ]; then
        mkdir -p "${imgDir}"
        set +e
        mirror_image "$img" "dir:$imgDir"
        ret=$?
        set -e
        if [ $ret -eq 0 ]; then
          ((success_count++)) || true
        else
          ((failure_count++)) || true
          warnf "Failed to save plugin: $img"
        fi
      else
        # Validate that the existing directory is complete
        if [[ -f "$imgDir/manifest.json" ]]; then
          debugf "Plugin already exists in directory: $imgDir"
          ((success_count++)) || true
        else
          warnf "Existing plugin directory is incomplete (missing manifest.json): $imgDir"
          warnf "Re-downloading plugin: $img"
          set +e
          mirror_image "$img" "dir:$imgDir"
          ret=$?
          set -e
          if [ $ret -eq 0 ]; then
            ((success_count++)) || true
          else
            ((failure_count++)) || true
            warnf "Failed to save plugin: $img"
          fi
        fi
      fi
    fi
  done
  
  infof "Plugin mirroring completed: ${success_count} successful, ${failure_count} failed"
  
  if [[ $failure_count -gt 0 ]]; then
    return 1
  fi
  
  # Mirror the catalog index if one was specified
  if [[ -n "$PLUGIN_INDEX" ]]; then
    infof ""
    infof "Mirroring catalog index..."
    if [[ -n "$TO_REGISTRY" ]]; then
      if ! mirror_catalog_index "$PLUGIN_INDEX" "$TO_REGISTRY" "" "$INTERNAL_REGISTRY"; then
        warnf "Failed to mirror catalog index, but plugins were mirrored successfully"
        warnf "You may need to manually configure the catalog index in your deployment"
      fi
    elif [[ -n "$TO_DIR" ]]; then
      if ! mirror_catalog_index "$PLUGIN_INDEX" "" "$TO_DIR"; then
        warnf "Failed to save catalog index, but plugins were saved successfully"
      fi
    fi
  fi
  
  return 0
}

function mirror_plugins_from_dir() {
  infof "Starting plugin mirroring from directory: ${FROM_DIR}"
  
  local BASE_DIR="${FROM_DIR}/plugins"
  if [ ! -d "${BASE_DIR}" ]; then
    errorf "No plugins directory found in ${FROM_DIR}"
    return 1
  fi
  
  # Parse original sources from existing summary file if available
  local existing_summary="${FROM_DIR}/rhdh-plugin-mirroring-summary.txt"
  if [[ -f "$existing_summary" ]]; then
    infof "Reading original plugin sources from existing summary..."
    parse_original_sources "$existing_summary"
    
    # Populate PLUGIN_IMAGES array with original sources (exclude catalog index)
    PLUGIN_IMAGES=()
    for original in "${!ORIGINAL_SOURCES[@]}"; do
      # Check if this is a catalog index or a plugin
      if [[ ! "$original" =~ plugin-catalog-index ]]; then
        PLUGIN_IMAGES+=("$original")
      fi
    done
    
    # Sort for consistent output
    if [[ ${#PLUGIN_IMAGES[@]} -gt 0 ]]; then
      mapfile -t PLUGIN_IMAGES < <(printf "%s\n" "${PLUGIN_IMAGES[@]}" | sort -u)
    fi
  else
    debugf "No existing summary found, will generate basic mapping from directory structure"
  fi
  
  # Check if there's a catalog index by looking at the summary file
  # The catalog index mapping is in the summary file (if present)
  if [[ -f "$existing_summary" ]] && [[ -d "${FROM_DIR}/catalog-index" ]]; then
    # Look for catalog index mapping in summary (format: "oci://...plugin-catalog-index:... → ...")
    local original_url
    original_url=$(grep "plugin-catalog-index" "$existing_summary" | head -n1 | sed -E 's/^(oci:[^ ]+).*/\1/' || true)
    if [[ -n "$original_url" ]]; then
      PLUGIN_INDEX="$original_url"
      debugf "Restored original catalog index URL: $PLUGIN_INDEX"
    fi
  fi
  
  debugf "Processing plugins from ${BASE_DIR}..."
  
  local success_count=0
  local failure_count=0
  
  # Process plugins with SHA256 digests
  while IFS= read -r sha256_dir; do
    if [[ -z "$sha256_dir" ]]; then
      continue
    fi
    
    local relative_path=${sha256_dir#"$BASE_DIR/"}
    local sha256_hash=${sha256_dir##*/sha256_}
    local parent_path
    parent_path=$(dirname "$relative_path")
    
    debugf "Processing plugin with SHA256: $parent_path@sha256:$sha256_hash"
    
    local lastTwo
    lastTwo=$(extract_last_two_elements "$parent_path")
    local targetImg="${TO_REGISTRY}/${lastTwo}@sha256:${sha256_hash}"
    
    if push_image_from_archive "$sha256_dir" "$targetImg"; then
      ((success_count++)) || true
    else
      ((failure_count++)) || true
      warnf "Failed to push plugin: $parent_path@sha256:$sha256_hash"
    fi
  done < <(find "$BASE_DIR" -type d -name "sha256_*" 2>/dev/null)

  # Process plugins with tags
  while IFS= read -r tag_dir; do
    if [[ -z "$tag_dir" ]]; then
      continue
    fi
    
    local relative_path=${tag_dir#"$BASE_DIR/"}
    local tag_value=${tag_dir##*/tag_}
    local parent_path
    parent_path=$(dirname "$relative_path")
    
    debugf "Processing plugin with tag: $parent_path:$tag_value"
    
    # Strip OCI subpath (everything after !) from tag for registry compatibility
    local clean_tag="${tag_value%%!*}"
    
    local lastTwo
    lastTwo=$(extract_last_two_elements "$parent_path")
    local targetImg="${TO_REGISTRY}/${lastTwo}:${clean_tag}"
    
    if push_image_from_archive "$tag_dir" "$targetImg"; then
      ((success_count++)) || true
    else
      ((failure_count++)) || true
      warnf "Failed to push plugin: $parent_path:$tag_value"
    fi
  done < <(find "$BASE_DIR" -type d -name "tag_*" 2>/dev/null)
  
  infof "Plugin mirroring from directory completed: ${success_count} successful, ${failure_count} failed"
  
  if [[ $failure_count -gt 0 ]]; then
    return 1
  fi
  
  # Push the catalog index if it was saved
  local catalog_dir="${FROM_DIR}/catalog-index"
  if [[ -d "$catalog_dir" ]] && [[ -n "$TO_REGISTRY" ]]; then
    infof ""
    infof "Rebuilding and pushing catalog index from saved data..."
    
    # Check if we have the catalog index information
    if [[ -z "$PLUGIN_INDEX" ]]; then
      warnf "No catalog index information found in mirroring summary"
      warnf "Catalog index was not included in the original export"
      return 0
    fi
    
    # Parse catalog name and tag from PLUGIN_INDEX
    # Format: oci://registry/org/image:tag or oci://registry/org/image@digest
    local catalog_name catalog_tag
    local registry_ref="${PLUGIN_INDEX#oci://}"
    
    if [[ "$registry_ref" =~ ^[^/]+/(.+)@sha256:(.+)$ ]]; then
      catalog_name="${BASH_REMATCH[1]}"
      local original_digest="${BASH_REMATCH[2]}"
      catalog_tag="sha256-${original_digest}"
    elif [[ "$registry_ref" =~ ^[^/]+/(.+):([^:@]+)$ ]]; then
      catalog_name="${BASH_REMATCH[1]}"
      catalog_tag="${BASH_REMATCH[2]}"
    else
      catalog_name="rhdh/plugin-catalog-index"
      catalog_tag="latest"
    fi

    # Flatten catalog_name to last 2 path elements for OCP internal registry compatibility
    catalog_name=$(extract_last_two_elements "$catalog_name")

    debugf "Rebuilding catalog index: $catalog_name:$catalog_tag"
    
    # Check for saved index.json
    if [[ ! -f "$catalog_dir/index.json" ]]; then
      warnf "No index.json found in saved catalog index"
      return 0
    fi
    
    # Update index.json with new registry
    local updated_index="$catalog_dir/index-updated.json"
    infof "Updating plugin registry references in index.json..."
    
    if ! jq --arg target_reg "$INTERNAL_REGISTRY" '
      . | with_entries(
        .value.registryReference |= (
          if . then
            # Keep only the last 2 path elements for OCP internal registry compatibility
            (split("/") | .[-2:] | join("/")) as $last_two |
            ($target_reg + "/" + $last_two)
          else
            .
          end
        )
      )
    ' "$catalog_dir/index.json" > "$updated_index" 2>/dev/null || [[ ! -s "$updated_index" ]]; then
      warnf "Failed to update index.json with new registry references"
      return 0
    fi

    # Replace index.json with updated version
    cp "$updated_index" "$catalog_dir/index.json"

    # Update OCI references in dynamic-plugins.default.yaml
    infof "Updating OCI references in dynamic-plugins.default.yaml..."
    if [[ -f "$catalog_dir/dynamic-plugins.default.yaml" ]]; then
      # Keep only the last 2 path elements for OCP internal registry compatibility
      sed -i -E "s|oci://[^/]+(/[^/]+)*(/[^/]+/[^[:space:]\"']+)|oci://$INTERNAL_REGISTRY\2|g" "$catalog_dir/dynamic-plugins.default.yaml"
      debugf "Updated OCI references in dynamic-plugins.default.yaml"
    fi

    # Update OCI references in all catalog-entities YAML files
    infof "Updating OCI references in catalog-entities..."
    local yaml_count=0
    while IFS= read -r yaml_file; do
      if [[ -n "$yaml_file" && -f "$yaml_file" ]]; then
        sed -i -E "s|oci://[^/]+(/[^/]+)*(/[^/]+/[^[:space:]\"']+)|oci://$INTERNAL_REGISTRY\2|g" "$yaml_file"
        ((yaml_count++)) || true
      fi
    done < <(find "$catalog_dir/catalog-entities" -name "*.yaml" -type f 2>/dev/null)
    
    if [[ $yaml_count -gt 0 ]]; then
      debugf "Updated OCI references in $yaml_count catalog-entity YAML files"
    fi
    
    debugf "Using podman to rebuild catalog index"
    
    # Create Dockerfile in a temporary build directory
    local build_dir="$catalog_dir/build"
    mkdir -p "$build_dir/content"
    
    # Copy catalog content (index.json, dynamic-plugins, catalog-entities, etc.)
    # Exclude build/ and index-updated.json (temporary files)
    for item in "$catalog_dir"/*; do
      local basename_item
      basename_item=$(basename "$item")
      if [[ "$basename_item" != "build" && "$basename_item" != "index-updated.json" ]]; then
        cp -r "$item" "$build_dir/content/"
      fi
    done
    
    cat > "$build_dir/Dockerfile" << 'EOF'
FROM scratch
COPY content/ /
EOF
    
    local temp_image_tag="localhost/temp-catalog-index:$catalog_tag"
    local target_image="$TO_REGISTRY/$catalog_name:$catalog_tag"
    
    if ! podman build -t "$temp_image_tag" "$build_dir" &>/dev/null; then
      warnf "Failed to rebuild catalog index image with podman"
      rm -rf "$build_dir" || true
      rm -f "$updated_index" || true
      return 0
    fi
    
    infof "Pushing rebuilt catalog index to: $target_image"
    if ! skopeo copy --all --dest-tls-verify=false "containers-storage:$temp_image_tag" "docker://$target_image" 2>&1; then
      warnf "Failed to push catalog index to registry"
      podman rmi "$temp_image_tag" &>/dev/null || true
      rm -rf "$build_dir" || true
      rm -f "$updated_index" || true
      return 0
    fi
    
    podman rmi "$temp_image_tag" &>/dev/null || true
    
    # Clean up temporary files and directories
    rm -rf "$build_dir" || true
    rm -f "$updated_index" || true
    
    infof "Successfully pushed catalog index"
  fi
  
  return 0
}

# Parse original plugin sources from an existing mirroring summary file
function parse_original_sources() {
  local summary_file="$1"
  
  if [[ ! -f "$summary_file" ]]; then
    debugf "No existing summary file found: $summary_file"
    return 1
  fi
  
  # Read original sources from the summary file
  # Format: "oci://original → destination"
  declare -gA ORIGINAL_SOURCES
  
  while IFS= read -r line; do
    # Skip comments and empty lines
    [[ "$line" =~ ^[[:space:]]*# ]] && continue
    [[ -z "${line// }" ]] && continue
    
    # Parse mapping line: "source → destination"
    if [[ "$line" =~ ^(oci://[^[:space:]]+)[[:space:]]*→ ]]; then
      local original="${BASH_REMATCH[1]}"
      ORIGINAL_SOURCES["$original"]=1
      debugf "Found original source: $original"
    fi
  done < "$summary_file"
  
  return 0
}

# Generate a mapping file with mirrored plugin references
# Similar to oc-mirror's mapping output for improved user experience
function generate_mapping_file() {
  local output_file="$1"
  local mode="$2"  # "registry" or "directory"
  local update_note="${3:-}"  # Optional update note (default to empty string)
  
  # Simple clean header
  echo "# This file contains a mapping of all mirrored plugins and their locations." > "$output_file"
  echo "# Total plugins mirrored: ${#PLUGIN_IMAGES[@]}" >> "$output_file"
  
  # Add update note if provided
  if [[ -n "$update_note" ]]; then
    echo "# $update_note" >> "$output_file"
  fi
  
  echo "" >> "$output_file"
  
  # Add catalog index mapping if present
  if [[ -n "$PLUGIN_INDEX" ]]; then
    if [[ "$mode" == "registry" ]]; then
      # Parse catalog index URL for registry mode
      if [[ "$PLUGIN_INDEX" =~ ^oci://(.+) ]]; then
        local registry_ref="${BASH_REMATCH[1]}"
        local catalog_name catalog_tag
        
        if [[ "$registry_ref" =~ ^[^/]+/(.+)@sha256:(.+)$ ]]; then
          catalog_name="${BASH_REMATCH[1]}"
          local original_digest="${BASH_REMATCH[2]}"
          catalog_tag="sha256-${original_digest}"
        elif [[ "$registry_ref" =~ ^[^/]+/(.+):([^:@]+)$ ]]; then
          catalog_name="${BASH_REMATCH[1]}"
          catalog_tag="${BASH_REMATCH[2]}"
        else
          catalog_name="rhdh/plugin-catalog-index"
          catalog_tag="latest"
        fi

        # Flatten catalog_name to last 2 path elements for OCP internal registry compatibility
        catalog_name=$(extract_last_two_elements "$catalog_name")

        echo "$PLUGIN_INDEX → oci://${INTERNAL_REGISTRY}/${catalog_name}:${catalog_tag}" >> "$output_file"
      fi
    elif [[ "$mode" == "directory" ]] && [[ -d "${TO_DIR}/catalog-index" ]]; then
      echo "$PLUGIN_INDEX → ${TO_DIR}/catalog-index/" >> "$output_file"
    fi
    echo "" >> "$output_file"
  fi
  
  # Add all plugin mappings
  if [[ "$mode" == "registry" ]]; then
    # Registry mode: show original → mirrored registry reference
    for img in "${PLUGIN_IMAGES[@]}"; do
      local img_no_prefix="${img#oci://}"
      local lastTwo
      local targetImg
      
      if [[ "$img_no_prefix" == *"@sha256:"* ]]; then
        local imgDigest="${img_no_prefix##*@sha256:}"
        lastTwo=$(extract_last_two_elements "${img_no_prefix%@*}")
        targetImg="${INTERNAL_REGISTRY}/${lastTwo}@sha256:${imgDigest}"
      elif [[ "$img_no_prefix" == *":"* ]]; then
        local imgTag="${img_no_prefix##*:}"
        lastTwo=$(extract_last_two_elements "${img_no_prefix%:*}")
        local clean_tag="${imgTag%%!*}"
        targetImg="${INTERNAL_REGISTRY}/${lastTwo}:$clean_tag"
      else
        lastTwo=$(extract_last_two_elements "${img_no_prefix}")
        targetImg="${INTERNAL_REGISTRY}/${lastTwo}:latest"
      fi

      echo "$img → oci://$targetImg" >> "$output_file"
    done
    
  elif [[ "$mode" == "directory" ]]; then
    # Directory mode: show original → local directory path
    for img in "${PLUGIN_IMAGES[@]}"; do
      local img_no_prefix="${img#oci://}"
      local imgDir
      
      if [[ "$img_no_prefix" == *"@sha256:"* ]]; then
        local imgDigest="${img_no_prefix##*@sha256:}"
        imgDir="${TO_DIR}/plugins/${img_no_prefix%@*}/sha256_$imgDigest"
      elif [[ "$img_no_prefix" == *":"* ]]; then
        local imgTag="${img_no_prefix##*:}"
        imgDir="${TO_DIR}/plugins/${img_no_prefix%:*}/tag_$imgTag"
      else
        imgDir="${TO_DIR}/plugins/${img_no_prefix}/tag_latest"
      fi
      
      echo "$img → $imgDir" >> "$output_file"
    done
  fi
  
  echo "" >> "$output_file"
}


# Main execution
infof "Red Hat Developer Hub - Dynamic Plugin OCI Artifact Mirroring Script"
infof "======================================================================"

if [[ -n "${FROM_DIR}" ]]; then
  # Import mode: load plugins from directory and push to registry
  mirror_plugins_from_dir
else
  # Export mode: resolve plugins and mirror to registry or directory
  mirror_plugins
fi

# Generate mapping file for user reference
if [ ${#PLUGIN_IMAGES[@]} -gt 0 ]; then
  if [[ -n "${FROM_DIR}" ]] && [[ -n "${TO_REGISTRY}" ]]; then
    # Update mode: migrating from directory to registry - update the file in place
    update_note="Updated from ${FROM_DIR} to ${TO_REGISTRY}"
    generate_mapping_file "${FROM_DIR}/rhdh-plugin-mirroring-summary.txt" "registry" "$update_note"
  elif [[ -n "${TO_DIR}" ]]; then
    # Export mode: mirroring to directory
    generate_mapping_file "${TO_DIR}/rhdh-plugin-mirroring-summary.txt" "directory"
  elif [[ -n "${TO_REGISTRY}" ]]; then
    # Direct mirror mode: mirroring to registry - save in original directory
    generate_mapping_file "${ORIGINAL_DIR}/rhdh-plugin-mirroring-summary.txt" "registry"
  fi
fi

# Summary
if [[ -n "${TO_DIR}" ]]; then
  infof ""
  infof "Export completed successfully!"
  infof "Plugins have been saved to: ${TO_DIR}/plugins"
  if [[ -n "${PLUGIN_INDEX}" ]] && [[ -d "${TO_DIR}/catalog-index" ]]; then
    infof "Catalog index has been saved to: ${TO_DIR}/catalog-index"
  fi
  infof "Plugin mapping: ${TO_DIR}/rhdh-plugin-mirroring-summary.txt"
  infof ""
  infof "Next steps for fully disconnected environments:"
  infof "1. Transfer ${TO_DIR} to your disconnected network"
  infof "2. Run this script again with --from-dir and --to-registry:"
  infof "   $0 --from-dir ${TO_DIR} --to-registry YOUR_REGISTRY"
  infof ""
elif [[ -n "${TO_REGISTRY}" ]]; then
  infof ""
  infof "Mirroring completed successfully!"
  infof "Plugins have been pushed to registry: ${TO_REGISTRY}"
  if [[ -n "${PLUGIN_INDEX}" ]]; then
    infof "Catalog index has been pushed with updated plugin references"
  fi
  # Show the mapping file location
  if [[ -n "${FROM_DIR}" ]]; then
    infof "Plugin mapping: ${FROM_DIR}/rhdh-plugin-mirroring-summary.txt"
  else
    infof "Plugin mapping: ${ORIGINAL_DIR}/rhdh-plugin-mirroring-summary.txt"
  fi
  infof ""
  infof "You can now configure your RHDH deployment to use these mirrored plugins."
  if [[ -n "${PLUGIN_INDEX}" ]]; then
    infof "The catalog index is available at: ${INTERNAL_REGISTRY}/rhdh/plugin-catalog-index"
  fi
  if [[ "${INTERNAL_REGISTRY}" != "${TO_REGISTRY}" ]]; then
    infof "Catalog index references point to in-cluster registry: ${INTERNAL_REGISTRY}"
  fi
  infof "Refer to the RHDH documentation for instructions on configuring dynamic plugins"
  infof "in airgapped environments for both operator and helm deployments."
  infof ""
fi

popd >/dev/null

