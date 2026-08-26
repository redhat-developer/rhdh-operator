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
#   file:/path/to/plugin                      - Local directory (file: protocol)
#
# Input file format (one per line):
#   url [integrity]
# Where integrity is optional (sha512-..., sha384-..., or sha256-...)
# For OCI URLs, integrity is ignored (digest in URL provides verification)
#
# Catalog Index (Extensions UI):
#   Set CATALOG_INDEX_IMAGE to extract catalog-entities from an OCI image.
#   Entities are copied to CATALOG_ENTITIES_EXTRACT_DIR/catalog-entities.
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

# Termination message file for Kubernetes to read on container exit
TERMINATION_LOG="${TERMINATION_LOG:-/dev/termination-log}"

# File to track download failures (for parallel execution)
FAILURE_LOG="${FAILURE_LOG:-/tmp/plugin-failures.log}"

# Catalog index settings (for Extensions UI catalog entities)
CATALOG_INDEX_IMAGE="${CATALOG_INDEX_IMAGE:-}"
CATALOG_ENTITIES_EXTRACT_DIR="${CATALOG_ENTITIES_EXTRACT_DIR:-/tmp/extensions}"

# ============================================================================
# Signal Handling - Forward SIGTERM to process group so children are terminated
# instead of outliving the container's grace period.
# Note: SIGKILL (signal 9) cannot be trapped - this is a kernel limitation.
# ============================================================================
trap 'trap - TERM; kill 0' TERM

# ============================================================================
# Termination Message - Write error details for Kubernetes to expose in status
# ============================================================================
write_termination_msg() {
    local msg="$1"
    # Write to termination log (Kubernetes reads this on container exit)
    # Truncate to 4KB (Kubernetes limit)
    echo "${msg}" | head -c 4096 > "${TERMINATION_LOG}" 2>/dev/null || true
}

# Exit with error and termination message
fail_with_msg() {
    local msg="$1"
    local exit_code="${2:-1}"
    write_termination_msg "${msg}"
    echo "Error: ${msg}" >&2
    exit "${exit_code}"
}

# Record a plugin failure (only first failure is kept)
record_failure() {
    local plugin_name="$1"
    local error_msg="$2"
    # Only write if no failure recorded yet (first failure wins)
    if [[ ! -f "${FAILURE_LOG}" ]]; then
        echo "${plugin_name}: ${error_msg}" > "${FAILURE_LOG}" 2>/dev/null || true
    fi
}

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
        fail_with_msg "Neither skopeo nor oras found. Install one to download OCI artifacts."
    fi
    echo "Using OCI tool: ${OCI_TOOL}"
}

# ============================================================================
# OCI Artifact Validation
# Validates that the downloaded artifact has the io.backstage.dynamic-packages
# annotation, confirming it's a valid plugin artifact built by rhdh-cli.
# ============================================================================
validate_plugin_artifact() {
    local oci_dir="$1"
    local label="$2"

    # For oras layout, check index.json -> manifest -> annotation
    if [[ -f "${oci_dir}/index.json" ]]; then
        local digest
        digest=$(grep -o '"sha256:[^"]*"' "${oci_dir}/index.json" | head -1 | tr -d '"' | sed 's/sha256://')
        if [[ -n "${digest}" && -f "${oci_dir}/blobs/sha256/${digest}" ]]; then
            if grep -q '"io.backstage.dynamic-packages"' "${oci_dir}/blobs/sha256/${digest}" 2>/dev/null; then
                return 0
            fi
        fi
    fi

    # For skopeo dir: transport, check manifest.json directly
    if [[ -f "${oci_dir}/manifest.json" ]]; then
        if grep -q '"io.backstage.dynamic-packages"' "${oci_dir}/manifest.json" 2>/dev/null; then
            return 0
        fi
    fi

    echo "[FAIL] ${label}: not a valid plugin artifact (missing io.backstage.dynamic-packages annotation)" >&2
    return 1
}

# ============================================================================
# OCI Registry - shared extraction helper
# Downloads OCI image and extracts layers to a directory
# Sets EXTRACT_DIR to the path of extracted content (caller must clean up)
# Args:
#   $1 - OCI image reference
#   $2 - Label for error messages
#   $3 - Optional: "skip-validation" to skip plugin artifact validation
# ============================================================================
extract_oci_image() {
    local image="$1"
    local label="$2"  # For error messages
    local skip_validation="${3:-}"

    # Strip oci:// prefix if present
    local clean_url="${image#oci://}"

    local tmp_oci
    tmp_oci=$(mktemp -d)

    # Download using detected OCI tool
    # Workaround: Force linux/amd64 platform for manifest list images.
    # The catalog-index is built by Konflux as a manifest list but contains only
    # platform-independent YAML files. Without --override-arch/--override-os,
    # skopeo fails on non-Linux platforms (e.g., macOS). See RHDHBUGS-2747.
    local download_err
    case "${OCI_TOOL}" in
        skopeo)
            if ! download_err=$(skopeo copy --override-arch amd64 --override-os linux "docker://${clean_url}" "dir:${tmp_oci}" 2>&1); then
                echo "[FAIL] ${label}: skopeo copy failed: ${download_err}" >&2
                rm -rf "${tmp_oci}"
                return 1
            fi
            ;;
        oras)
            if ! download_err=$(oras copy --platform linux/amd64 "${clean_url}" --to-oci-layout "${tmp_oci}:latest" 2>&1); then
                echo "[FAIL] ${label}: oras copy failed: ${download_err}" >&2
                rm -rf "${tmp_oci}"
                return 1
            fi
            ;;
        *)
            echo "[FAIL] ${label}: no OCI tool available" >&2
            rm -rf "${tmp_oci}"
            return 1
            ;;
    esac

    # Validate the artifact has the required annotation (skip for non-plugin artifacts)
    if [[ "${skip_validation}" != "skip-validation" ]]; then
        if ! validate_plugin_artifact "${tmp_oci}" "${label}"; then
            rm -rf "${tmp_oci}"
            return 1
        fi
    fi

    # Find layer files (skip manifests/config)
    # shellcheck disable=SC2012 # OCI blob filenames are SHA256 hashes (alphanumeric only)
    local layer_file
    if [[ "${OCI_TOOL}" == "oras" ]]; then
        layer_file=$(find "${tmp_oci}/blobs/sha256" -type f -print0 | xargs -0 ls -S 2>/dev/null | head -1)
    else
        layer_file=$(find "${tmp_oci}" -maxdepth 1 -type f ! -name "*manifest*" ! -name "*version*" -print0 | xargs -0 ls -S 2>/dev/null | head -1)
    fi

    if [[ -z "${layer_file}" || ! -f "${layer_file}" ]]; then
        echo "[FAIL] ${label}: could not find layer blob" >&2
        rm -rf "${tmp_oci}"
        return 1
    fi

    # Extract layer to temp directory
    EXTRACT_DIR=$(mktemp -d)
    if ! tar -xzf "${layer_file}" -C "${EXTRACT_DIR}" 2>/dev/null && \
       ! tar -xf "${layer_file}" -C "${EXTRACT_DIR}" 2>/dev/null; then
        echo "[FAIL] ${label}: extraction failed" >&2
        rm -rf "${tmp_oci}" "${EXTRACT_DIR}"
        EXTRACT_DIR=""
        return 1
    fi

    rm -rf "${tmp_oci}"
    # EXTRACT_DIR is set for caller to use and clean up
}

# ============================================================================
# OCI Registry - plugin download (oci://)
# ============================================================================
download_oci() {
    local url="$1"
    local plugin_name="$2"
    local plugin_dir="$3"

    EXTRACT_DIR=""
    if ! extract_oci_image "${url}" "${plugin_name}"; then
        return 1
    fi

    # Move only the plugin subfolder
    if [[ -d "${EXTRACT_DIR}/${plugin_name}" ]]; then
        mv "${EXTRACT_DIR}/${plugin_name}" "${plugin_dir}"
    else
        echo "[FAIL] ${plugin_name}: expected subfolder not found" >&2
        rm -rf "${EXTRACT_DIR}"
        return 1
    fi

    rm -rf "${EXTRACT_DIR}"
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

    if ! curl --fail -sL "${url}" -o "${tmp_file}"; then
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
    local encoded_name
    encoded_name=$(url_encode "${package_name}")

    # Prepare curl auth headers
    local auth_header=""
    if [[ -n "${NPM_AUTH_TOKEN}" ]]; then
        auth_header="Authorization: Bearer ${NPM_AUTH_TOKEN}"
    fi

    # Helper function for curl with optional auth
    npm_curl() {
        if [[ -n "${auth_header}" ]]; then
            curl --fail -sL -H "${auth_header}" "$@"
        else
            curl --fail -sL "$@"
        fi
    }

    # Resolve version if not specified (requires full package doc)
    if [[ -z "${package_version}" ]]; then
        local full_metadata
        full_metadata=$(mktemp)

        if ! npm_curl "${NPM_REGISTRY}/${encoded_name}" -o "${full_metadata}"; then
            echo "[FAIL] ${plugin_name}: failed to fetch package metadata" >&2
            rm -f "${full_metadata}"
            return 1
        fi

        # Extract "latest" from dist-tags
        package_version=$(grep -o '"latest":"[^"]*"' "${full_metadata}" | head -1 | cut -d'"' -f4)
        rm -f "${full_metadata}"

        if [[ -z "${package_version}" ]]; then
            echo "[FAIL] ${plugin_name}: could not determine latest version" >&2
            return 1
        fi
    fi

    # Fetch version-specific metadata (small, unambiguous response)
    # GET ${registry}/${package}/${version} returns only that version's data
    local version_metadata
    version_metadata=$(mktemp)

    if ! npm_curl "${NPM_REGISTRY}/${encoded_name}/${package_version}" -o "${version_metadata}"; then
        echo "[FAIL] ${plugin_name}: version ${package_version} not found" >&2
        rm -f "${version_metadata}"
        return 1
    fi

    # Check for error response
    if grep -q '"error"' "${version_metadata}" 2>/dev/null; then
        local error_msg
        error_msg=$(grep -o '"error":"[^"]*"' "${version_metadata}" | head -1 | cut -d'"' -f4)
        echo "[FAIL] ${plugin_name}: registry error: ${error_msg:-unknown}" >&2
        rm -f "${version_metadata}"
        return 1
    fi

    # Extract tarball URL (unambiguous in version-specific doc)
    local tarball_url
    tarball_url=$(grep -o '"tarball":"[^"]*"' "${version_metadata}" | head -1 | cut -d'"' -f4)

    if [[ -z "${tarball_url}" ]]; then
        echo "[FAIL] ${plugin_name}: no tarball URL in version metadata" >&2
        rm -f "${version_metadata}"
        return 1
    fi

    # Use user-provided integrity if available, otherwise extract from registry
    local integrity
    if [[ -n "${user_integrity}" ]]; then
        integrity="${user_integrity}"
    else
        integrity=$(grep -o '"integrity":"[^"]*"' "${version_metadata}" | head -1 | cut -d'"' -f4)
    fi

    rm -f "${version_metadata}"

    # Download tarball
    local tmp_file
    tmp_file=$(mktemp)

    if ! npm_curl "${tarball_url}" -o "${tmp_file}"; then
        echo "[FAIL] ${plugin_name}: failed to download tarball" >&2
        rm -f "${tmp_file}"
        return 1
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
# File URL (file:/) - directory only
# ============================================================================
download_file() {
    local url="$1"
    local plugin_name="$2"
    local plugin_dir="$3"

    # Strip file:// or file:/ prefix
    local path="${url#file://}"
    path="${path#file:}"

    if [[ ! -d "${path}" ]]; then
        echo "[FAIL] ${plugin_name}: directory not found: ${path}" >&2
        return 1
    fi

    mkdir -p "${plugin_dir}"
    cp -r "${path}"/* "${plugin_dir}/" 2>/dev/null || \
    cp -r "${path}"/. "${plugin_dir}/"
}

# ============================================================================
# Catalog Index Extraction
# Extracts catalog-entities from CATALOG_INDEX_IMAGE for Extensions UI
# ============================================================================
extract_catalog_entities() {
    local image="$1"
    local dest_dir="$2"

    if [[ -z "${image}" ]]; then
        return 0
    fi

    echo "=== Extracting catalog entities from ${image} ==="

    EXTRACT_DIR=""
    # Skip plugin validation - catalog index is not a plugin artifact
    if ! extract_oci_image "${image}" "catalog-index" "skip-validation"; then
        echo "WARNING: Failed to extract catalog index" >&2
        return 1
    fi

    # Look for catalog-entities/extensions
    local entities_src=""
    if [[ -d "${EXTRACT_DIR}/catalog-entities/extensions" ]]; then
        entities_src="${EXTRACT_DIR}/catalog-entities/extensions"
    elif [[ -d "${EXTRACT_DIR}/catalog-entities" ]]; then
        entities_src="${EXTRACT_DIR}/catalog-entities"
    fi

    if [[ -n "${entities_src}" && -d "${entities_src}" ]]; then
        local entities_dest="${dest_dir}/catalog-entities"
        mkdir -p "${dest_dir}"
        rm -rf "${entities_dest}"
        cp -r "${entities_src}" "${entities_dest}"
        local count
        count=$(find "${entities_dest}" -type f \( -name "*.yaml" -o -name "*.yml" \) 2>/dev/null | wc -l | tr -d ' ')
        echo "Catalog entities extracted to ${entities_dest} (${count} files)"
    else
        echo "WARNING: No catalog-entities found in ${image}" >&2
    fi

    rm -rf "${EXTRACT_DIR}"
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
    plugin_name=$(echo "${url}" | sed 's|oci://||' | sed 's|https\?://||' | sed 's|file://||' | sed 's|file:||' | sed 's|@sha256:.*||' | sed 's|@.*||' | awk -F'/' '{print $NF}')

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
        file:*)
            download_file "${url}" "${plugin_name}" "${plugin_dir}" || result=$?
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
    else
        record_failure "${plugin_name}" "download failed from ${url}"
    fi
    return ${result}
}

# ============================================================================
# Main
# ============================================================================

START_TIME=$(date +%s)

if [[ ! -f "${INPUT_FILE}" ]]; then
    fail_with_msg "Input file not found: ${INPUT_FILE}"
fi

# Acquire lock to prevent concurrent installations
create_lock

# Clear failure log from previous runs
rm -f "${FAILURE_LOG}"

# Detect OCI tool (skopeo preferred, oras fallback)
detect_oci_tool

# Create output directory (may already exist from create_lock)
mkdir -p "${OUTPUT_DIR}"

# Extract catalog entities from catalog index image (for Extensions UI)
if [[ -n "${CATALOG_INDEX_IMAGE}" ]]; then
    extract_catalog_entities "${CATALOG_INDEX_IMAGE}" "${CATALOG_ENTITIES_EXTRACT_DIR}"
else
    echo "=== CATALOG_INDEX_IMAGE not set, skipping catalog entities extraction"
fi

export -f download_plugin extract_oci_image validate_plugin_artifact download_oci download_http download_npm download_local download_file detect_oci_tool parse_npmrc url_encode verify_integrity record_failure
export OUTPUT_DIR OCI_TOOL NPM_REGISTRY NPM_AUTH_TOKEN FAILURE_LOG

total=$(grep -cv '^#\|^$' "${INPUT_FILE}" 2>/dev/null || echo 0)

# Exit successfully if no packages to download (all plugins are local paths)
if [[ "${total}" -eq 0 ]]; then
    echo "=== No remote plugins to download (all plugins are local paths) ==="
    echo "=== Done in 0s ==="
    exit 0
fi

echo "=== Downloading ${total} plugins to ${OUTPUT_DIR} (${PARALLEL_JOBS} parallel) ==="
echo ""

# Use xargs for parallel execution
# Capture xargs exit code - it returns 123 if any command fails
XARGS_EXIT=0
# shellcheck disable=SC2016 # Single quotes intentional - $1/$2 expand in inner bash
grep -v '^#' "${INPUT_FILE}" | grep -v '^$' | \
    xargs -P "${PARALLEL_JOBS}" -I {} bash -c 'download_plugin "$1" "$2"' _ {} "${OUTPUT_DIR}" || XARGS_EXIT=$?

echo ""

END_TIME=$(date +%s)
ELAPSED=$((END_TIME - START_TIME))

# Check for failure and write termination message
if [[ -f "${FAILURE_LOG}" ]]; then
    echo "=== FAILED ==="
    cat "${FAILURE_LOG}"
    echo ""
    echo "Elapsed time: ${ELAPSED}s"
    # Write termination message with failure details
    write_termination_msg "$(cat "${FAILURE_LOG}")"
    exit 1
elif [[ ${XARGS_EXIT} -ne 0 ]]; then
    # Fallback if FAILURE_LOG wasn't created but xargs failed
    echo "=== FAILED ==="
    echo "Plugin installation failed (exit code ${XARGS_EXIT})"
    echo ""
    echo "Elapsed time: ${ELAPSED}s"
    write_termination_msg "Plugin installation failed (exit code ${XARGS_EXIT})"
    exit 1
fi

echo "=== Complete ==="
echo "Plugins in ${OUTPUT_DIR}:"
find "${OUTPUT_DIR}" -mindepth 1 -maxdepth 1 -type d -exec basename {} \; 2>/dev/null | head -20
echo ""
echo "Elapsed time: ${ELAPSED}s"
