#!/bin/bash
# Copy default-config and plugin-deps to LOCALBIN for 'make run' and 'make test'
#
# Dynamic plugins source depends on OPERATOR_DP_PROCESSING:
#   - true:  uses local-test/dynamic-plugins.yaml (rhdh profile only)
#   - false: uses default-config/dynamic-plugins.yaml
#
# Usage: ./hack/copy-local-dynamic-plugins.sh <PROFILE> <LOCALBIN>

set -euo pipefail

PROFILE="${1:-rhdh}"
LOCALBIN="${2:-./bin}"

DEFAULT_CONFIG_SRC="config/profile/${PROFILE}/default-config"
PLUGIN_DEPS_SRC="config/profile/${PROFILE}/plugin-deps"
DEFAULT_CONFIG_TARGET="${LOCALBIN}/default-config"
PLUGIN_DEPS_TARGET="${LOCALBIN}/plugin-deps"

# Step 1: Copy default-config
mkdir -p "${DEFAULT_CONFIG_TARGET}"
rm -fr "${DEFAULT_CONFIG_TARGET:?}"/*
cp -r "${DEFAULT_CONFIG_SRC}"/* "${DEFAULT_CONFIG_TARGET}/"

# Step 2: Copy plugin-deps if it exists
mkdir -p "${PLUGIN_DEPS_TARGET}"
rm -fr "${PLUGIN_DEPS_TARGET:?}"/*
if [[ -d "${PLUGIN_DEPS_SRC}" ]]; then
    cp -r "${PLUGIN_DEPS_SRC}"/* "${PLUGIN_DEPS_TARGET}/" 2>/dev/null || :
fi

# Step 3: For OPERATOR_DP_PROCESSING=true and rhdh profile, overlay dynamic-plugins.yaml from local-test
if [[ "${OPERATOR_DP_PROCESSING:-false}" == "true" && "${PROFILE}" == "rhdh" ]]; then
    LOCAL_TEST_DIR="config/profile/${PROFILE}/local-test"
    DYNAMIC_PLUGINS_FILE="${LOCAL_TEST_DIR}/dynamic-plugins.yaml"

    if [[ ! -d "${LOCAL_TEST_DIR}" ]]; then
        echo "Error: local-test directory not found at ${LOCAL_TEST_DIR}" >&2
        echo "" >&2
        echo "Run 'make local-dynamic-plugins' to generate it first." >&2
        echo "" >&2
        echo "This extracts dynamic-plugins.default.yaml from the catalog-index image" >&2
        echo "and creates a local configuration for testing." >&2
        exit 1
    fi

    if [[ ! -f "${DYNAMIC_PLUGINS_FILE}" ]]; then
        echo "Error: dynamic-plugins.yaml not found at ${DYNAMIC_PLUGINS_FILE}" >&2
        echo "" >&2
        echo "Run 'make local-dynamic-plugins' to regenerate the local-test directory." >&2
        exit 1
    fi

    cp "${DYNAMIC_PLUGINS_FILE}" "${DEFAULT_CONFIG_TARGET}/"
    echo "Using local-test dynamic-plugins.yaml (OPERATOR_DP_PROCESSING=true)"
else
    echo "Using default-config dynamic-plugins.yaml (OPERATOR_DP_PROCESSING=${OPERATOR_DP_PROCESSING:-false})"
fi
