i#!/bin/bash
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
# OCI Download Tools:
#   - oras (preferred): Uses 'oras copy --to-oci-layout' for optimized single operation.
#     ~30% faster than manifest+blob fetch. No jq dependency.
#   - skopeo (fallback): Red Hat certified, FIPS compliant, CVE tracked.
#     Uses 'skopeo copy' to dir: transport then extracts layer.
#
# The script auto-detects available tools (prefers oras, falls back to skopeo).

set -euo pipefail

INPUT_FILE="${1:-${INPUT_FILE:-/input/packages.txt}}"
OUTPUT_DIR="${2:-${OUTPUT_DIR:-/dynamic-plugins-root}}"
PARALLEL_JOBS="${3:-${PARALLEL_JOBS:-4}}"

# ============================================================================
# Tool Detection
# ============================================================================

detect_oci_tool() {
    # Use OCI_TOOL if already set, otherwise auto-detect
    if [[ -n "${OCI_TOOL:-}" ]]; then
        echo "Using OCI tool: ${OCI_TOOL} (from env)"
        return
    fi

    if command -v oras &> /dev/null; then
        OCI_TOOL="oras"
    elif command -v skopeo &> /dev/null; then
        OCI_TOOL="skopeo"
    else
        echo "Error: Neither oras nor skopeo found. Install one to download OCI artifacts." >&2
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

    if ! oras copy "${clean_url}" --to-oci-layout "${tmp_oci}:latest" >/dev/null 2>&1; then
        echo "[FAIL] ${plugin_name}: oras copy failed" >&2
        rm -rf "${tmp_oci}"
        return 1
    fi

    # Find the layer blob (largest file in blobs/sha256/, skip config and manifest)
    local layer_file
    layer_file=$(ls -S "${tmp_oci}/blobs/sha256/"* 2>/dev/null | head -1)

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
    if ! skopeo copy "${docker_url}" "dir:${tmp_dir}" 2>/dev/null; then
        echo "[FAIL] ${plugin_name}: skopeo copy failed" >&2
        rm -rf "${tmp_dir}"
        return 1
    fi

    # Find the layer blob (largest file, skip manifest.json and version)
    local layer_file
    layer_file=$(ls -S "${tmp_dir}"/* 2>/dev/null | grep -v manifest | grep -v version | head -1)

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
    local url="$1"
    local plugin_name="$2"
    local plugin_dir="$3"

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

    local tmp_file
    tmp_file=$(mktemp)

    if ! curl -sL "${url}" -o "${tmp_file}"; then
        echo "[FAIL] ${plugin_name}: download failed" >&2
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
# NPM package (@scope/package or package)
# ============================================================================
download_npm() {
    local url="$1"
    local plugin_name="$2"
    local plugin_dir="$3"

    # TODO: Implement NPM download
    # npm pack "${url}" --pack-destination /tmp
    # tar -xzf /tmp/*.tgz -C "${plugin_dir}"
    echo "[FAIL] ${plugin_name}: NPM download not implemented" >&2
    return 1
}

# ============================================================================
# Local file/directory (./)
# ============================================================================
download_local() {
    local url="$1"
    local plugin_name="$2"
    local plugin_dir="$3"

    # TODO: Implement local copy
    # cp -r "${url}" "${plugin_dir}"
    echo "[FAIL] ${plugin_name}: local copy not implemented" >&2
    return 1
}

# ============================================================================
# Main download router
# ============================================================================
download_plugin() {
    local url="$1"
    local output_dir="$2"

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
    case "${url}" in
        oci://*)
            download_oci "${url}" "${plugin_name}" "${plugin_dir}"
            ;;
        http://*|https://*)
            download_http "${url}" "${plugin_name}" "${plugin_dir}"
            ;;
        @*)
            download_npm "${url}" "${plugin_name}" "${plugin_dir}"
            ;;
        ./*)
            download_local "${url}" "${plugin_name}" "${plugin_dir}"
            ;;
        *)
            echo "[FAIL] ${plugin_name}: unknown URL format: ${url}" >&2
            return 1
            ;;
    esac

    if [[ $? -eq 0 ]]; then
        echo "[DONE] ${plugin_name}"
    fi
}

# ============================================================================
# Main
# ============================================================================

START_TIME=$(date +%s)

if [[ ! -f "${INPUT_FILE}" ]]; then
    echo "Error: Input file not found: ${INPUT_FILE}" >&2
    exit 1
fi

# Detect OCI tool (oras preferred, skopeo fallback)
detect_oci_tool

# Create output directory
mkdir -p "${OUTPUT_DIR}"

export -f download_plugin download_oci download_oci_oras download_oci_skopeo download_http download_npm download_local detect_oci_tool
export OUTPUT_DIR OCI_TOOL

total=$(grep -v '^#' "${INPUT_FILE}" | grep -v '^$' | wc -l | tr -d ' ')

echo "=== Downloading ${total} plugins to ${OUTPUT_DIR} (${PARALLEL_JOBS} parallel) ==="
echo ""

# Use xargs for parallel execution
grep -v '^#' "${INPUT_FILE}" | grep -v '^$' | \
    xargs -P "${PARALLEL_JOBS}" -I {} bash -c 'download_plugin "$1" "$2"' _ {} "${OUTPUT_DIR}"

echo ""
echo "=== Complete ==="
echo "Plugins in ${OUTPUT_DIR}:"
ls -d "${OUTPUT_DIR}"/*/ 2>/dev/null | xargs -I {} basename {} | head -20

END_TIME=$(date +%s)
ELAPSED=$((END_TIME - START_TIME))
echo ""
echo "Elapsed time: ${ELAPSED}s"
