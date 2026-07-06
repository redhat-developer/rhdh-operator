#!/bin/bash
# Extract dynamic-plugins.default.yaml from catalog-index image
# and create local-test/default-dynamic-plugins.yaml for local testing
#
# The local-test directory is git-ignored, allowing developers to customize
# dynamic-plugins for local testing. Other config files come from default-config.
#
# Usage: Run from project root:
#   ./hack/create-local-dynamic-plugins.sh
#   IMAGE=quay.io/rhdh/plugin-catalog-index:1.10 ./hack/create-local-dynamic-plugins.sh

set -euo pipefail

# Save current directory (project root)
PROJECT_ROOT="$(pwd)"

IMAGE="${IMAGE:-quay.io/rhdh/plugin-catalog-index:latest}"
OUTPUT_DIR="config/profile/rhdh/local-test"
OUTPUT_FILE="${OUTPUT_DIR}/dynamic-plugins.yaml"

# Verify we're in project root
if [[ ! -f "go.mod" ]] || [[ ! -d "config/profile/rhdh" ]]; then
  echo "Error: This script must be run from the project root directory" >&2
  echo "Usage: ./hack/create-local-dynamic-plugins.sh" >&2
  exit 1
fi

# Verify skopeo is available
if ! command -v skopeo &> /dev/null; then
  echo "Error: skopeo command not found" >&2
  echo "Please install skopeo to use this script" >&2
  echo "  macOS: brew install skopeo" >&2
  echo "  Fedora/RHEL: dnf install skopeo" >&2
  exit 1
fi

# Convert paths to absolute
OUTPUT_DIR="${PROJECT_ROOT}/${OUTPUT_DIR}"
OUTPUT_FILE="${PROJECT_ROOT}/${OUTPUT_FILE}"

echo "=== Creating local-test dynamic-plugins from catalog-index ==="
echo "Source image: ${IMAGE}"
echo "Output file: ${OUTPUT_FILE}"
echo ""

# Create local-test directory
mkdir -p "${OUTPUT_DIR}"

# Create temporary directory
TEMP_DIR=$(mktemp -d)
trap 'rm -rf ${TEMP_DIR}' EXIT

echo "Extracting dynamic-plugins.default.yaml from image using skopeo..."
cd "${TEMP_DIR}"

# Use skopeo to copy image to a local directory (linux/amd64)
skopeo copy --override-arch amd64 --override-os linux "docker://${IMAGE}" "dir:./image"

# Find and extract the layer containing dynamic-plugins.default.yaml
for layer in image/*; do
  [[ "$layer" == */manifest.json || "$layer" == */version ]] && continue
  if tar -tf "$layer" 2>/dev/null | grep -q "dynamic-plugins.default.yaml"; then
    tar -xf "$layer" dynamic-plugins.default.yaml 2>/dev/null || true
    break
  fi
done

# Check if dynamic-plugins.default.yaml was extracted
if [[ ! -f "dynamic-plugins.default.yaml" ]]; then
  echo "Error: dynamic-plugins.default.yaml not found in image" >&2
  echo "Trying alternative extraction..." >&2

  # Alternative: extract all layers and search
  mkdir -p rootfs
  for layer in image/*; do
    [[ "$layer" == */manifest.json || "$layer" == */version ]] && continue
    tar -xf "$layer" -C rootfs 2>/dev/null || true
  done

  if [[ -f "rootfs/dynamic-plugins.default.yaml" ]]; then
    cp rootfs/dynamic-plugins.default.yaml ./
  else
    echo "Error: Could not find dynamic-plugins.default.yaml in any layer" >&2
    exit 1
  fi
fi

# Create ConfigMap with the extracted content
echo "Creating ConfigMap at ${OUTPUT_FILE}..."

# Create ConfigMap header
cat > "${OUTPUT_FILE}" <<'EOF'
# WARNING: This file is auto-generated!
#
# This file is automatically extracted from the catalog-index image
# by ./hack/create-local-dynamic-plugins.sh
# This is for LOCAL TESTING ONLY and is git-ignored.
#
# You can edit this file for local testing purposes.
# To regenerate: make local-dynamic-plugins
#
apiVersion: v1
kind: ConfigMap
metadata:
  name: default-dynamic-plugins
data:
  dynamic-plugins.yaml: |
EOF

# Append the extracted content with proper indentation (4 spaces)
sed 's/^/    /' dynamic-plugins.default.yaml >> "${OUTPUT_FILE}"

echo ""
echo "=== Local test config created ==="
echo "File: ${OUTPUT_FILE}"
echo "Size: $(du -h "${OUTPUT_FILE}" | cut -f1)"
echo ""
echo "This file is git-ignored. Edit as needed for local testing."
echo "Use 'make run' to test (default-config + local-test overlay)."
