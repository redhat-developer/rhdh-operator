#!/bin/bash
# Download dynamic plugins from various sources
#
# Usage: ./install_plugins.sh [input_file] [output_dir] [parallel_jobs]
#   input_file: File with plugin URLs (default: /input/packages.txt)
#   output_dir: Directory to install plugins (default: /dynamic-plugins-root)
#   parallel_jobs: Number of parallel downloads (default: 4)
#
# Supported URL formats:
#   oci://ghcr.io/org/repo/plugin@sha256:...  - OCI registry
#   https://example.com/plugin.tgz            - HTTP(S) download
#   @backstage/plugin-techdocs                - NPM package
#   ./path/to/plugin                          - Local file/directory
#
# Input file format (one per line):
#   url [integrity]
# Where integrity is optional (sha512-..., sha384-..., or sha256-...)
# For OCI URLs, integrity is ignored (digest in URL provides verification)
#
# OCI Download Tools:
#   - skopeo (preferred): Red Hat certified, FIPS compliant, CVE tracked.
#     Uses 'skopeo copy' to dir: transport then extracts layer.
#   - oras (fallback): Uses 'oras copy --to-oci-layout' for optimized single operation.
#
# The script auto-detects available tools (prefers skopeo, falls back to oras).

set -euo pipefail

INPUT_FILE="${1:-${INPUT_FILE:-/input/packages.txt}}"
OUTPUT_DIR="${2:-${OUTPUT_DIR:-/dynamic-plugins-root}}"
PARALLEL_JOBS="${3:-${PARALLEL_JOBS:-4}}"
LOCK_FILE="${OUTPUT_DIR}/install-dynamic-plugins.lock"

# ============================================================================
# Signal Handling - Forward SIGTERM to process group so children are terminated
# instead of outliving the container's grace period.
# Note: SIGKILL (signal 9) cannot be trapped - this is a kernel limitation.
# ============================================================================
trap 'trap - TERM; kill 0' TERM

# ============================================================================
# Lock Management - Prevent concurrent plugin installations
# ============================================================================
create_lock() {
    # Ensure output directory exists for lock file
    mkdir -p "${OUTPUT_DIR}"

    while true; do
        # Try to create lock file exclusively (fails if exists)
        if (set -o noclobber; echo $$ > "${LOCK_FILE}") 2>/dev/null; then
            echo "======= Created lock file: ${LOCK_FILE}"
            return 0
        fi

        # Lock exists - check if holding process is still alive
        local lock_pid
        lock_pid=$(cat "${LOCK_FILE}" 2>/dev/null || echo "")
        if [[ -n "${lock_pid}" ]] && ! kill -0 "${lock_pid}" 2>/dev/null; then
            # Stale lock - process no longer exists
            echo "======= Removing stale lock (PID ${lock_pid} not found)"
            rm -f "${LOCK_FILE}"
            continue
        fi

        echo "======= Waiting for lock release (held by PID ${lock_pid:-unknown})..."
        sleep 1
    done
}

remove_lock() {
    if [[ -f "${LOCK_FILE}" ]]; then
        rm -f "${LOCK_FILE}"
        echo "======= Removed lock file: ${LOCK_FILE}"
    fi
}

# Ensure lock is removed on exit (normal, error, or signal)
trap 'remove_lock' EXIT

# ============================================================================
# Tool Detection
# ============================================================================

detect_oci_tool() {
    # Use OCI_TOOL if already set, otherwise auto-detect
    if [[ -n "${OCI_TOOL:-}" ]]; then
        echo "Using OCI tool: ${OCI_TOOL} (from env)"
        return
    fi

    if command -v skopeo &> /dev/null; then
        OCI_TOOL="skopeo"
    elif command -v oras &> /dev/null; then
        OCI_TOOL="oras"
    else
        echo "Error: Neither skopeo nor oras found. Install one to download OCI artifacts." >&2
        exit 1
    fi
    echo "Using OCI tool: ${OCI_TOOL}"
}

# ============================================================================
# OCI Registry - oras implementation (uses oras copy for single optimized operation)
# ============================================================================
download_oci_oras() {
    local url="$1"
    local plugin_name="$2"
    local plugin_dir="$3"

    # Strip oci:// prefix
    local clean_url="${url#oci://}"

    # Copy to OCI layout (single optimized operation, ~30% faster than manifest+blob)
    local tmp_oci
    tmp_oci=$(mktemp -d)

    local oras_err
    if ! oras_err=$(oras copy "${clean_url}" --to-oci-layout "${tmp_oci}:latest" 2>&1); then
        echo "[FAIL] ${plugin_name}: oras copy failed: ${oras_err}" >&2
        rm -rf "${tmp_oci}"
        return 1
    fi

    # Find the layer blob (largest file in blobs/sha256/, skip config and manifest)
    # shellcheck disable=SC2012 # OCI blob filenames are SHA256 hashes (alphanumeric only)
    local layer_file
    layer_file=$(find "${tmp_oci}/blobs/sha256" -type f -print0 | xargs -0 ls -S 2>/dev/null | head -1)

    if [[ -z "${layer_file}" || ! -f "${layer_file}" ]]; then
        echo "[FAIL] ${plugin_name}: could not find layer blob" >&2
        rm -rf "${tmp_oci}"
        return 1
    fi

    # Extract to temp directory, then move only the plugin subfolder
    local tmp_extract
    tmp_extract=$(mktemp -d)

    if tar -xzf "${layer_file}" -C "${tmp_extract}" 2>/dev/null || \
       tar -xf "${layer_file}" -C "${tmp_extract}" 2>/dev/null; then
        # Move only the plugin subfolder (skip index.json etc)
        if [[ -d "${tmp_extract}/${plugin_name}" ]]; then
            mv "${tmp_extract}/${plugin_name}" "${plugin_dir}"
        else
            echo "[FAIL] ${plugin_name}: expected subfolder not found" >&2
            rm -rf "${tmp_oci}" "${tmp_extract}"
            return 1
        fi
    else
        echo "[FAIL] ${plugin_name}: extraction failed" >&2
        rm -rf "${tmp_oci}" "${tmp_extract}"
        return 1
    fi

    rm -rf "${tmp_oci}" "${tmp_extract}"
}

# ============================================================================
# OCI Registry - skopeo implementation
# ============================================================================
download_oci_skopeo() {
    local url="$1"
    local plugin_name="$2"
    local plugin_dir="$3"

    # Strip oci:// prefix and convert to docker:// format
    local clean_url="${url#oci://}"
    local docker_url="docker://${clean_url}"

    local tmp_dir
    tmp_dir=$(mktemp -d)

    # Copy image to dir: transport (extracts layers as files)
    local skopeo_err
    if ! skopeo_err=$(skopeo copy "${docker_url}" "dir:${tmp_dir}" 2>&1); then
        echo "[FAIL] ${plugin_name}: skopeo copy failed: ${skopeo_err}" >&2
        rm -rf "${tmp_dir}"
        return 1
    fi

    # Find the layer blob (largest file, skip manifest.json and version)
    # shellcheck disable=SC2012 # Skopeo dir: filenames are SHA256 hashes (alphanumeric only)
    local layer_file
    layer_file=$(find "${tmp_dir}" -maxdepth 1 -type f ! -name "*manifest*" ! -name "*version*" -print0 | xargs -0 ls -S 2>/dev/null | head -1)

    if [[ -z "${layer_file}" || ! -f "${layer_file}" ]]; then
        echo "[FAIL] ${plugin_name}: could not find layer blob" >&2
        rm -rf "${tmp_dir}"
        return 1
    fi

    # Extract to temp directory, then move only the plugin subfolder
    local tmp_extract
    tmp_extract=$(mktemp -d)

    if tar -xzf "${layer_file}" -C "${tmp_extract}" 2>/dev/null || \
       tar -xf "${layer_file}" -C "${tmp_extract}" 2>/dev/null; then
        # Move only the plugin subfolder (skip index.json etc)
        if [[ -d "${tmp_extract}/${plugin_name}" ]]; then
            mv "${tmp_extract}/${plugin_name}" "${plugin_dir}"
        else
            echo "[FAIL] ${plugin_name}: expected subfolder not found" >&2
            rm -rf "${tmp_dir}" "${tmp_extract}"
            return 1
        fi
    else
        echo "[FAIL] ${plugin_name}: extraction failed" >&2
        rm -rf "${tmp_dir}" "${tmp_extract}"
        return 1
    fi

    rm -rf "${tmp_dir}" "${tmp_extract}"
}

# ============================================================================
# OCI Registry - router (oci://)
# ============================================================================
download_oci() {
    local plugin_name="$2"

    case "${OCI_TOOL}" in
        oras)
            download_oci_oras "$@"
            ;;
        skopeo)
            download_oci_skopeo "$@"
            ;;
        *)
            echo "[FAIL] ${plugin_name}: no OCI tool available" >&2
            return 1
            ;;
    esac
}

# ============================================================================
# HTTP(S) download (http:// or https://)
# ============================================================================
download_http() {
    local url="$1"
    local plugin_name="$2"
    local plugin_dir="$3"
    local integrity="${4:-}"

    local tmp_file
    tmp_file=$(mktemp)

    if ! curl -sL "${url}" -o "${tmp_file}"; then
        echo "[FAIL] ${plugin_name}: download failed" >&2
        rm -f "${tmp_file}"
        return 1
    fi

    # Verify integrity if provided
    if [[ -n "${integrity}" ]] && ! verify_integrity "${tmp_file}" "${integrity}"; then
        echo "[FAIL] ${plugin_name}: integrity verification failed" >&2
        rm -f "${tmp_file}"
        return 1
    fi

    mkdir -p "${plugin_dir}"
    if tar -xzf "${tmp_file}" -C "${plugin_dir}" 2>/dev/null || \
       tar -xf "${tmp_file}" -C "${plugin_dir}" 2>/dev/null; then
        : # success
    else
        echo "[FAIL] ${plugin_name}: extraction failed" >&2
        rm -f "${tmp_file}"
        rm -rf "${plugin_dir}"
        return 1
    fi

    rm -f "${tmp_file}"
}

# ============================================================================
# NPM package (@scope/package or package@version)
# Emulates npm pack using curl + registry API
# Supports: @scope/package, @scope/package@version, package, package@version
# ============================================================================

# Parse .npmrc file for registry URL and auth token
parse_npmrc() {
    local npmrc="${NPM_CONFIG_USERCONFIG:-$HOME/.npmrc}"

    # Defaults
    NPM_REGISTRY="${NPM_REGISTRY:-https://registry.npmjs.org}"
    NPM_AUTH_TOKEN="${NPM_AUTH_TOKEN:-}"

    if [[ -f "${npmrc}" ]]; then
        # Extract registry (last one wins)
        local reg
        reg=$(grep -E '^registry\s*=' "${npmrc}" 2>/dev/null | tail -1 | sed 's/registry\s*=\s*//' | tr -d '"' | tr -d "'" | tr -d ' ')
        if [[ -n "${reg}" ]]; then
            NPM_REGISTRY="${reg%/}"  # Remove trailing slash
        fi

        # Extract auth token (//registry.npmjs.org/:_authToken=xxx)
        local token
        token=$(grep -E ':_authToken=' "${npmrc}" 2>/dev/null | head -1 | sed 's/.*:_authToken=//' | tr -d '"' | tr -d "'" | tr -d ' ')
        if [[ -n "${token}" ]]; then
            NPM_AUTH_TOKEN="${token}"
        fi
    fi
}

# URL-encode a string (for scoped package names)
url_encode() {
    local string="$1"
    # Encode @ and / for scoped packages
    echo "${string}" | sed 's/@/%40/g; s|/|%2f|g'
}

# Verify package integrity using openssl
verify_integrity() {
    local file="$1"
    local integrity="$2"

    if [[ -z "${integrity}" ]] || [[ "${SKIP_INTEGRITY_CHECK:-}" == "true" ]]; then
        return 0
    fi

    # Parse algorithm-hash format (e.g., sha512-abc123...)
    local algorithm hash
    algorithm=$(echo "${integrity}" | cut -d'-' -f1)
    hash=$(echo "${integrity}" | cut -d'-' -f2-)

    # Validate algorithm
    case "${algorithm}" in
        sha512|sha384|sha256) ;;
        *)
            echo "Unsupported integrity algorithm: ${algorithm}" >&2
            return 1
            ;;
    esac

    # Compute hash and compare
    local computed
    computed=$(openssl dgst -"${algorithm}" -binary "${file}" | openssl base64 -A)

    if [[ "${computed}" != "${hash}" ]]; then
        echo "Integrity check failed: expected ${hash}, got ${computed}" >&2
        return 1
    fi

    return 0
}

download_npm() {
    local url="$1"
    local plugin_name="$2"
    local plugin_dir="$3"
    local user_integrity="${4:-}"

    # Parse .npmrc for registry and auth
    parse_npmrc

    # Parse package spec: @scope/package@version or package@version
    local package_name package_version
    if [[ "${url}" =~ ^(@[^@]+)@(.+)$ ]]; then
        # Scoped package with version: @scope/package@version
        package_name="${BASH_REMATCH[1]}"
        package_version="${BASH_REMATCH[2]}"
    elif [[ "${url}" =~ ^(@[^@]+)$ ]]; then
        # Scoped package without version: @scope/package
        package_name="${BASH_REMATCH[1]}"
        package_version=""
    elif [[ "${url}" =~ ^([^@]+)@(.+)$ ]]; then
        # Unscoped package with version: package@version
        package_name="${BASH_REMATCH[1]}"
        package_version="${BASH_REMATCH[2]}"
    else
        # Unscoped package without version: package
        package_name="${url}"
        package_version=""
    fi

    # Build registry URL (encode scoped package names)
    local encoded_name registry_url
    encoded_name=$(url_encode "${package_name}")
    registry_url="${NPM_REGISTRY}/${encoded_name}"

    # Prepare curl auth headers
    local auth_header=""
    if [[ -n "${NPM_AUTH_TOKEN}" ]]; then
        auth_header="Authorization: Bearer ${NPM_AUTH_TOKEN}"
    fi

    # Fetch package metadata
    local metadata
    metadata=$(mktemp)

    if [[ -n "${auth_header}" ]]; then
        if ! curl -sL -H "${auth_header}" "${registry_url}" -o "${metadata}"; then
            echo "[FAIL] ${plugin_name}: failed to fetch package metadata" >&2
            rm -f "${metadata}"
            return 1
        fi
    else
        if ! curl -sL "${registry_url}" -o "${metadata}"; then
            echo "[FAIL] ${plugin_name}: failed to fetch package metadata" >&2
            rm -f "${metadata}"
            return 1
        fi
    fi

    # Check for error response
    if grep -q '"error"' "${metadata}" 2>/dev/null; then
        local error_msg
        error_msg=$(grep -o '"error":"[^"]*"' "${metadata}" | head -1 | cut -d'"' -f4)
        echo "[FAIL] ${plugin_name}: registry error: ${error_msg:-unknown}" >&2
        rm -f "${metadata}"
        return 1
    fi

    # Resolve version (use latest if not specified)
    if [[ -z "${package_version}" ]]; then
        package_version=$(grep -o '"latest":"[^"]*"' "${metadata}" | head -1 | cut -d'"' -f4)
        if [[ -z "${package_version}" ]]; then
            echo "[FAIL] ${plugin_name}: could not determine latest version" >&2
            rm -f "${metadata}"
            return 1
        fi
    fi

    # Extract tarball URL for the version
    # NPM tarball URLs contain the version, so we can match on that
    local tarball_url

    # Find tarball URL containing the version (handles nested JSON without jq)
    tarball_url=$(grep -o "\"tarball\":\"[^\"]*${package_version}[^\"]*\"" "${metadata}" | \
                  head -1 | cut -d'"' -f4)

    if [[ -z "${tarball_url}" ]]; then
        echo "[FAIL] ${plugin_name}: could not find tarball URL for version ${package_version}" >&2
        rm -f "${metadata}"
        return 1
    fi

    # Use user-provided integrity if available, otherwise extract from registry
    # Registry integrity is in the dist block near the tarball URL
    local integrity
    if [[ -n "${user_integrity}" ]]; then
        integrity="${user_integrity}"
    else
        # Extract integrity from the dist block (near tarball with same version)
        integrity=$(grep -o "\"tarball\":\"[^\"]*${package_version}[^\"]*\"[^}]*\"integrity\":\"[^\"]*\"" "${metadata}" | \
                    grep -o '"integrity":"[^"]*"' | head -1 | cut -d'"' -f4)
    fi

    rm -f "${metadata}"

    # Download tarball
    local tmp_file
    tmp_file=$(mktemp)

    if [[ -n "${auth_header}" ]]; then
        if ! curl -sL -H "${auth_header}" "${tarball_url}" -o "${tmp_file}"; then
            echo "[FAIL] ${plugin_name}: failed to download tarball" >&2
            rm -f "${tmp_file}"
            return 1
        fi
    else
        if ! curl -sL "${tarball_url}" -o "${tmp_file}"; then
            echo "[FAIL] ${plugin_name}: failed to download tarball" >&2
            rm -f "${tmp_file}"
            return 1
        fi
    fi

    # Verify integrity
    if ! verify_integrity "${tmp_file}" "${integrity}"; then
        echo "[FAIL] ${plugin_name}: integrity verification failed" >&2
        rm -f "${tmp_file}"
        return 1
    fi

    # Extract tarball (npm tarballs have "package/" prefix)
    local tmp_extract
    tmp_extract=$(mktemp -d)

    if ! tar -xzf "${tmp_file}" -C "${tmp_extract}" 2>/dev/null; then
        echo "[FAIL] ${plugin_name}: extraction failed" >&2
        rm -f "${tmp_file}"
        rm -rf "${tmp_extract}"
        return 1
    fi

    rm -f "${tmp_file}"

    # Move extracted content (strip "package/" prefix)
    mkdir -p "${plugin_dir}"
    if [[ -d "${tmp_extract}/package" ]]; then
        # Standard npm tarball structure
        mv "${tmp_extract}/package"/* "${plugin_dir}/" 2>/dev/null || \
        cp -r "${tmp_extract}/package"/* "${plugin_dir}/"
    else
        # Non-standard structure, move everything
        mv "${tmp_extract}"/* "${plugin_dir}/" 2>/dev/null || \
        cp -r "${tmp_extract}"/* "${plugin_dir}/"
    fi

    rm -rf "${tmp_extract}"
}

# ============================================================================
# Local paths (./) - NOT SUPPORTED
# ============================================================================
download_local() {
    local plugin_name="$2"
    echo "[FAIL] ${plugin_name}: local paths (./) are not supported" >&2
    return 1
}

# ============================================================================
# Main download router
# ============================================================================
download_plugin() {
    local input_line="$1"
    local output_dir="$2"

    # Parse input: "url [integrity]" (space/tab separated)
    local url integrity
    url=$(echo "${input_line}" | awk '{print $1}')
    integrity=$(echo "${input_line}" | awk '{print $2}')

    # Extract plugin name from URL
    local plugin_name
    plugin_name=$(echo "${url}" | sed 's|oci://||' | sed 's|https\?://||' | sed 's|@sha256:.*||' | sed 's|@.*||' | awk -F'/' '{print $NF}')

    local plugin_dir="${output_dir}/${plugin_name}"

    if [[ -d "${plugin_dir}" && -n "$(ls -A "${plugin_dir}" 2>/dev/null)" ]]; then
        echo "[SKIP] ${plugin_name} (exists)"
        return 0
    fi

    echo "[DOWN] ${url}" # ${plugin_name}"

    # Route based on URL prefix
    local result=0
    case "${url}" in
        oci://*)
            # OCI uses digest in URL for verification, integrity ignored
            download_oci "${url}" "${plugin_name}" "${plugin_dir}" || result=$?
            ;;
        http://*|https://*)
            download_http "${url}" "${plugin_name}" "${plugin_dir}" "${integrity}" || result=$?
            ;;
        @*)
            # Scoped npm package: @scope/package or @scope/package@version
            download_npm "${url}" "${plugin_name}" "${plugin_dir}" "${integrity}" || result=$?
            ;;
        ./*)
            download_local "${url}" "${plugin_name}" "${plugin_dir}" || result=$?
            ;;
        *@*)
            # Unscoped npm package with version: package@version
            download_npm "${url}" "${plugin_name}" "${plugin_dir}" "${integrity}" || result=$?
            ;;
        *)
            # Assume unscoped npm package without version, or fail
            if [[ "${url}" =~ ^[a-zA-Z0-9._-]+$ ]]; then
                download_npm "${url}" "${plugin_name}" "${plugin_dir}" "${integrity}" || result=$?
            else
                echo "[FAIL] ${plugin_name}: unknown URL format: ${url}" >&2
                return 1
            fi
            ;;
    esac

    if [[ ${result} -eq 0 ]]; then
        echo "[DONE] ${plugin_name}"
    fi
    return ${result}
}

# ============================================================================
# Main
# ============================================================================

START_TIME=$(date +%s)

if [[ ! -f "${INPUT_FILE}" ]]; then
    echo "Error: Input file not found: ${INPUT_FILE}" >&2
    exit 1
fi

# Acquire lock to prevent concurrent installations
create_lock

# Detect OCI tool (skopeo preferred, oras fallback)
detect_oci_tool

# Create output directory (may already exist from create_lock)
mkdir -p "${OUTPUT_DIR}"

export -f download_plugin download_oci download_oci_oras download_oci_skopeo download_http download_npm download_local detect_oci_tool parse_npmrc url_encode verify_integrity
export OUTPUT_DIR OCI_TOOL NPM_REGISTRY NPM_AUTH_TOKEN

total=$(grep -cv '^#\|^$' "${INPUT_FILE}")

echo "=== Downloading ${total} plugins to ${OUTPUT_DIR} (${PARALLEL_JOBS} parallel) ==="
echo ""

# Use xargs for parallel execution
# shellcheck disable=SC2016 # Single quotes intentional - variables expand in inner bash
grep -v '^#' "${INPUT_FILE}" | grep -v '^$' | \
    xargs -P "${PARALLEL_JOBS}" -I {} bash -c 'download_plugin "$1" "$2"' _ {} "${OUTPUT_DIR}"

echo ""
echo "=== Complete ==="
echo "Plugins in ${OUTPUT_DIR}:"
find "${OUTPUT_DIR}" -mindepth 1 -maxdepth 1 -type d -exec basename {} \; 2>/dev/null | head -20

END_TIME=$(date +%s)
ELAPSED=$((END_TIME - START_TIME))
echo ""
echo "Elapsed time: ${ELAPSED}s"
