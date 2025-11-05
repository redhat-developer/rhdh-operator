#!/bin/bash
#
# Script to mirror Red Hat Developer Hub dynamic plugin OCI artifacts for deployments in restricted environments.
# This script is installation-method agnostic and works with both operator and helm deployments
# on both OpenShift and Kubernetes platforms.
#
# Requires: skopeo, jq, tar, base64

set -euo pipefail

NC='\033[0m'

PLUGIN_INDEX=""
PLUGIN_LIST_FILE=""
PLUGIN_URLS=()
PLUGIN_IMAGES=()

TO_REGISTRY=""
TO_DIR=""
FROM_DIR=""

REGISTRY_AUTH_FILE="${REGISTRY_AUTH_FILE:-}"

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
    echo "Error: Required tool '$1' is not installed." >&2
    exit 1
  fi
}

check_tool "skopeo"
check_tool "jq"
check_tool "tar"
check_tool "base64"

function usage() {
  echo "
Red Hat Developer Hub - Dynamic Plugin OCI Artifact Mirroring Script

This script mirrors dynamic plugin OCI artifacts for RHDH deployments in restricted environments.
It is installation-method agnostic and works with both operator and helm deployments.

Usage:
  $0 [OPTIONS]

Options:
  --plugin-index <oci-url>               : Plugin catalog index to query for version-specific plugins
                                           (e.g., oci://quay.io/rhdh/plugin-catalog-index:1.8)
  --plugin-list <file>                   : Local .txt file with plugin OCI references (oci:// URL per line,
                                           comments with '#' are ignored, no quotes needed in file)
  --plugins <oci-url> [<oci-url> ...]    : Space-separated list of plugin OCI URLs to mirror
                                           Note: URLs containing '!' must be quoted (e.g., 'oci://...:tag!subpath')
  --to-registry <registry_url>           : Mirror the plugins to the specified registry
                                           (assumes you are already logged in)
  --to-dir </absolute/path/to/dir>       : Mirror plugins to the specified directory (for fully disconnected environments)
                                           This directory can be transferred to a disconnected network
  --from-dir </absolute/path/to/dir>     : Load plugins from the specified directory and push to registry
                                           (for use in disconnected environments after transferring the directory)
  -h, --help                             : Show this help message

Important Notes:
  - When using --plugins on the command line, URLs containing '!' must be quoted with single quotes
    Example: 'oci://registry/image:tag!subpath'
  - When using --plugin-list with a text file, do NOT use quotes around URLs
    The file should contain raw URLs, one per line

Examples:

  # Mirror all plugins from a catalog index to a registry
  $0 \\
    --plugin-index oci://quay.io/rhdh/plugin-catalog-index:1.8 \\
    --to-registry registry.example.com

  # Mirror specific plugins from a file list
  $0 \\
    --plugin-list /path/to/plugins.txt \\
    --to-registry registry.example.com

  # Mirror specific plugins by direct OCI reference (use quotes for URLs with '!')
  $0 \\
    --plugins 'oci://quay.io/rhdh-plugin-catalog/backstage-community-plugin-quay:1.8.0--1.22.1!backstage-community-plugin-quay' \\
              'oci://quay.io/rhdh-plugin-catalog/backstage-community-plugin-github-actions:1.8.0--0.11.1!backstage-community-plugin-github-actions' \\
    --to-registry registry.example.com

  # Combined mode: catalog index + custom plugins
  $0 \\
    --plugin-index oci://quay.io/rhdh/plugin-catalog-index:1.8 \\
    --plugins 'oci://custom-registry.example.com/my-plugin:1.0!my-plugin' \\
    --to-registry registry.example.com

  # Export plugins to directory (for fully disconnected environments)
  $0 \\
    --plugin-index oci://quay.io/rhdh/plugin-catalog-index:1.8 \\
    --to-dir /path/to/export

  # Import plugins from directory and push to registry (in disconnected environment)
  $0 \\
    --from-dir /path/to/export \\
    --to-registry registry.example.com

Plugin List File Format:
  Create a .txt file with one plugin OCI URL per line. Lines starting with '#' are treated as comments.
  Example plugins.txt content:
    # Red Hat Developer Hub Plugin List
    oci://quay.io/rhdh-plugin-catalog/backstage-community-plugin-quay:1.8
    oci://quay.io/rhdh-plugin-catalog/backstage-community-plugin-github-actions:1.7
    oci://quay.io/rhdh-plugin-catalog/backstage-community-plugin-azure-devops:1.6
    'oci://quay.io/rhdh-plugin-catalog/backstage-community-plugin-dynatrace:1.8.0--10.6.0!backstage-community-plugin-dynatrace'
    'oci://quay.io/rhdh-plugin-catalog/backstage-community-plugin-analytics-provider-segment:1.8.0--1.16.0'
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
  errorf "Plugin index must be in OCI format: oci://registry/org/image:tag"
  exit 1
fi

# Validate plugin URLs format
for url in "${PLUGIN_URLS[@]}"; do
  if [[ ! "$url" =~ ^oci:// ]]; then
    errorf "Plugin URL must be in OCI format: oci://registry/org/image:tag. Got: $url"
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

# Setup working directory
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

# Registry authentication
function merge_registry_auth() {
  set -euo pipefail

  currentRegistryAuthFile="${REGISTRY_AUTH_FILE:-${XDG_RUNTIME_DIR:-/run/user/$(id -u)}/containers/auth.json}"
  debugf "Current registry auth file: $currentRegistryAuthFile"
  
  if [ ! -f "${currentRegistryAuthFile}" ]; then
    debugf "No existing registry auth file found. Proceeding without pre-existing authentication."
    return
  fi

  # Create a temporary XDG_RUNTIME_DIR for isolated authentication
  XDG_RUNTIME_DIR=$(mktemp -d)
  export XDG_RUNTIME_DIR
  mkdir -p "${XDG_RUNTIME_DIR}/containers"
  # shellcheck disable=SC2064
  trap "rm -fr $XDG_RUNTIME_DIR || true" EXIT
  
  export REGISTRY_AUTH_FILE="${XDG_RUNTIME_DIR}/containers/auth.json"
  debugf "Using registry auth file: $REGISTRY_AUTH_FILE"

  # Copy existing auth to the new location
  cp "${currentRegistryAuthFile}" "${REGISTRY_AUTH_FILE}"
}

merge_registry_auth

# Extract the last two path elements from an image URL (e.g., org/image from registry.io/org/image)
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

# Copy OCI images to registry, save to directory, or push from directory using skopeo
function mirror_image_to_registry() {
  local src_image="$1"
  local dest_image="$2"
  
  infof "Mirroring $src_image to $dest_image..."
  
  # Handle OCI URLs (remove oci:// prefix and handle subpaths)
  if [[ "$src_image" == oci://* ]]; then
    # Extract the Docker image reference (everything before the ! for subpaths)
    local docker_ref="${src_image%!*}"
    docker_ref="${docker_ref#oci://}"
    skopeo copy --preserve-digests --remove-signatures --all --dest-tls-verify=false "docker://$docker_ref" "docker://$dest_image" || return 1
  else
    skopeo copy --preserve-digests --remove-signatures --all --dest-tls-verify=false "docker://$src_image" "docker://$dest_image" || return 1
  fi
  return 0
}

function mirror_image_to_archive() {
  local src_image="$1"
  local archive_path="$2"

  debugf "Saving $src_image to $archive_path..."
  
  # Handle OCI URLs (remove oci:// prefix and handle subpaths)
  if [[ "$src_image" == oci://* ]]; then
    # Extract the Docker image reference (everything before the ! for subpaths)
    local docker_ref="${src_image%!*}"
    docker_ref="${docker_ref#oci://}"
    skopeo copy --preserve-digests --remove-signatures --all "docker://$docker_ref" "dir:$archive_path" || return 1
  else
    skopeo copy --preserve-digests --remove-signatures --all "docker://$src_image" "dir:$archive_path" || return 1
  fi
  return 0
}

function push_image_from_archive() {
  local archive_path="$1"
  local dest_image="$2"
  
  infof "Pushing $archive_path to $dest_image..."
  skopeo copy --preserve-digests --remove-signatures --all --dest-tls-verify=false "dir:$archive_path" "docker://$dest_image" || return 1
  return 0
}

# Download the catalog index, extract package YAML files,
# filter by version, and return a unique list of plugin OCI URLs
function resolve_plugin_index() {
  local index_url="$1"
  infof "Resolving plugins from catalog index: $index_url"
  
  # Strip oci:// prefix and extract version for filtering
  if [[ "$index_url" =~ ^oci://(.+) ]]; then
    local registry_ref="${BASH_REMATCH[1]}"
    
    # Extract version/digest for plugin filtering logic
    local version
    if [[ "$registry_ref" =~ @(.+)$ ]]; then
      # Digest format: capture everything after @
      version="${BASH_REMATCH[1]}"
    elif [[ "$registry_ref" =~ :([^:@]+)$ ]]; then
      # Tag format: capture everything after last :
      version="${BASH_REMATCH[1]}"
    fi
    
    debugf "Extracting plugin catalog index image: $registry_ref"
    
    # Create temporary directory for extracting the catalog index
    local temp_dir
    temp_dir=$(mktemp -d)
    
    # Cleanup temp directory when function returns
    # shellcheck disable=SC2064
    trap "rm -rf '$temp_dir'" RETURN
    
    # Extract the catalog index image
    if ! skopeo copy "docker://$registry_ref" "dir:$temp_dir/catalog-index" 2>/dev/null; then
      errorf "Failed to extract catalog index image: $registry_ref"
      return 1
    fi
    
    # Find the largest layer (likely contains the catalog data)
    local catalog_layer
    catalog_layer=$(find "$temp_dir/catalog-index" -type f -not -name "manifest.json" -not -name "version" -exec ls -la {} \; | sort -k5 -nr | head -1 | awk '{print $NF}')
    
    if [[ -z "$catalog_layer" ]]; then
      errorf "No catalog data layer found in index image"
      return 1
    fi
    
    debugf "Using catalog layer: $(basename "$catalog_layer")"
    
    # Extract the catalog layer
    local catalog_data_dir="$temp_dir/catalog-data"
    mkdir -p "$catalog_data_dir"
    
    if ! tar -xf "$catalog_layer" -C "$catalog_data_dir" 2>/dev/null; then
      errorf "Failed to extract catalog data from layer"
      return 1
    fi
    
    # Find all package YAML files
    local package_files
    package_files=$(find "$catalog_data_dir" -name "*.yaml" -path "*/packages/*" 2>/dev/null)
    
    if [[ -z "$package_files" ]]; then
      warnf "No package files found in catalog index"
      return 1
    fi
    
    debugf "Found $(echo "$package_files" | wc -l) package files in catalog index"
    
    # Extract OCI plugin URLs from package files
    local temp_plugin_images=()
    local filtered_plugins=0
    
    while IFS= read -r package_file; do
      if [[ -f "$package_file" ]]; then
        # Extract dynamicArtifact field that contains OCI URLs
        local oci_urls
        oci_urls=$(grep -E "^\s*dynamicArtifact:\s*oci://" "$package_file" 2>/dev/null | sed 's/.*dynamicArtifact:\s*//' | tr -d ' ')
        
        if [[ -n "$oci_urls" ]]; then
          # Filter by version if specified (not 'next', 'latest', or sha256 digest)
          # When using @sha256:digest for catalog index, don't filter plugins by digest
          if [[ "$version" != "next" && "$version" != "latest" && ! "$version" =~ ^sha256: ]]; then
            # Check if the OCI URL contains the version
            if echo "$oci_urls" | grep -q ":$version"; then
              temp_plugin_images+=("$oci_urls")
              ((filtered_plugins++))
            fi
          else
            # For 'next', 'latest', or digest-based catalog index, include all OCI URLs
            temp_plugin_images+=("$oci_urls")
            ((filtered_plugins++))
          fi
        fi
      fi
    done <<< "$package_files"
    
    # Remove duplicates
    if [[ ${#temp_plugin_images[@]} -gt 0 ]]; then
      local unique_plugins=()
      for plugin in "${temp_plugin_images[@]}"; do
        local found=false
        for existing in "${unique_plugins[@]}"; do
          if [[ "$existing" == "$plugin" ]]; then
            found=true
            break
          fi
        done
        if [[ "$found" == "false" ]]; then
          unique_plugins+=("$plugin")
        fi
      done
      PLUGIN_IMAGES=("${unique_plugins[@]}")
    fi
    
    infof "Found ${#PLUGIN_IMAGES[@]} unique plugins from catalog index"
    if [[ "$version" != "next" && "$version" != "latest" && ! "$version" =~ ^sha256: ]]; then
      debugf "Filtered to ${#PLUGIN_IMAGES[@]} plugins matching version $version"
    fi
    
    return 0
  else
    errorf "Invalid OCI URL format: $index_url. Expected format: oci://registry/org/image:tag or oci://registry/org/image@sha256:digest"
    return 1
  fi
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
      mirror_image_to_registry "$img" "$targetImg"
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
        mirror_image_to_archive "$img" "$imgDir"
        ret=$?
        set -e
        if [ $ret -eq 0 ]; then
          ((success_count++)) || true
        else
          ((failure_count++)) || true
          warnf "Failed to save plugin: $img"
        fi
      else
        debugf "Plugin already exists in directory: $imgDir"
        ((success_count++)) || true
      fi
    fi
  done
  
  infof "Plugin mirroring completed: ${success_count} successful, ${failure_count} failed"
  
  if [[ $failure_count -gt 0 ]]; then
    return 1
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
  
  return 0
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

# Summary
if [[ -n "${TO_DIR}" ]]; then
  infof ""
  infof "Export completed successfully!"
  infof "Plugins have been saved to: ${TO_DIR}"
  infof ""
  infof "Next steps for fully disconnected environments:"
  infof "1. Transfer ${TO_DIR} to your disconnected network"
  infof "2. Run this script again with --from-dir and --to-registry:"
  infof "   $0 --from-dir ${TO_DIR} --to-registry <your-registry>"
  infof ""
elif [[ -n "${TO_REGISTRY}" ]]; then
  infof ""
  infof "Mirroring completed successfully!"
  infof "Plugins have been pushed to registry: ${TO_REGISTRY}"
  infof ""
  infof "You can now configure your RHDH deployment to use these mirrored plugins."
  infof "Refer to the RHDH documentation for instructions on configuring dynamic plugins"
  infof "in airgapped environments for both operator and helm deployments."
  infof ""
fi

popd >/dev/null

