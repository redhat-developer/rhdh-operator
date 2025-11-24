#!/bin/bash
#
# Validate that FROM statements with digests in Dockerfiles use manifest list digests
# (multi-architecture) rather than single-architecture manifest digests.
#
# This script validates the current state of Dockerfiles in the repository.
# It checks specific Dockerfiles that are expected to use multi-arch images.
#
# Usage:
#   ./hack/validate-image-digests.sh
#
# Requirements:
#   - skopeo (for inspecting container image manifests)
#   - jq (for parsing JSON, optional but recommended)
#
# Exit codes:
#   0 - All digests are valid manifest lists (or no digests found)
#   1 - One or more digests are not manifest lists
#

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Dockerfiles to validate (these are expected to use multi-arch base images)
DOCKERFILES=(
    "Dockerfile"
    ".rhdh/docker/Dockerfile"
    ".rhdh/docker/bundle.Dockerfile"
    "bundle/rhdh/bundle.Dockerfile"
    "bundle/backstage.io/bundle.Dockerfile"
)

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Functions
log_success() {
    echo -e "${GREEN}✅ $*${NC}"
}

log_warning() {
    echo -e "${YELLOW}⚠️  $*${NC}"
}

log_error() {
    echo -e "${RED}❌ $*${NC}"
}

# Main logic
main() {
    cd "$REPO_ROOT"
    
    # Check if skopeo is available
    if ! command -v skopeo &> /dev/null; then
        log_error "skopeo not found in PATH"
        echo "Please install skopeo to run this validation"
        echo "See: https://github.com/containers/skopeo"
        exit 1
    fi
    
    # Extract all FROM statements with digests from specified Dockerfiles
    declare -A digests_map
    declare -A digests_files_map
    
    for dockerfile in "${DOCKERFILES[@]}"; do
        if [ ! -f "$dockerfile" ]; then
            continue
        fi
        
        # Get FROM statements with digests, excluding --platform lines
        from_lines=$(grep -E '^\s*FROM\s+.*@sha256:' "$dockerfile" | grep -v -E '^\s*FROM\s+--platform' || true)
        if [ -z "$from_lines" ]; then
            continue
        fi
        
        # Process each FROM line
        while IFS= read -r line; do
            if [[ -z "$line" ]]; then
                continue
            fi
            
            # Extract the image reference with digest
            image_ref=$(echo "$line" | sed -E 's/^\s*FROM\s+([^ ]+@sha256:[a-f0-9]+).*/\1/')
            
            if [ -n "$image_ref" ]; then
                # Mark that we've seen this digest
                digests_map["$image_ref"]=1
                
                # Append this file to the list of files containing this digest
                if [ -n "${digests_files_map[$image_ref]:-}" ]; then
                    digests_files_map["$image_ref"]="${digests_files_map[$image_ref]}|$dockerfile"
                else
                    digests_files_map["$image_ref"]="$dockerfile"
                fi
            fi
        done <<< "$from_lines"
    done
    
    # Check if we have any digests to validate
    if [ ${#digests_map[@]} -eq 0 ]; then
        exit 0
    fi
    
    # Track validation results
    has_errors=0
    validated_count=0
    
    # Process each digest
    for image_ref in "${!digests_map[@]}"; do
        # Get all files containing this digest
        IFS='|' read -ra source_files <<< "${digests_files_map[$image_ref]}"
        
        # Skopeo doesn't support references with both tag and digest (image:tag@sha256:...)
        # Strip the tag if present, keeping only image@digest
        if [[ "$image_ref" =~ :[^@]+@sha256: ]]; then
            image_ref_for_inspect=$(echo "$image_ref" | sed -E 's/:[^@]+(@sha256:)/\1/')
        else
            image_ref_for_inspect="$image_ref"
        fi
        
        # Inspect the manifest using skopeo
        set +e  # Temporarily disable exit on error
        manifest_info=$(skopeo inspect --raw "docker://$image_ref_for_inspect" 2>&1)
        inspect_status=$?
        set -e  # Re-enable exit on error
        
        # Check if inspection failed
        if [ $inspect_status -ne 0 ]; then
            log_error "Failed to inspect: $image_ref"
            echo "  File(s):"
            for file in "${source_files[@]}"; do
                echo "    - $file"
            done
            echo
            has_errors=1
            continue
        fi

        # Check if the manifest is a manifest list (multi-architecture)
        if echo "$manifest_info" | grep -q '"mediaType".*"application/vnd.docker.distribution.manifest.list.v2+json"' || \
           echo "$manifest_info" | grep -q '"mediaType".*"application/vnd.oci.image.index.v1+json"'; then
            # Valid manifest list
            log_success "Manifest list (multi-arch): $image_ref"
            echo "  File(s):"
            for file in "${source_files[@]}"; do
                echo "    - $file"
            done
            echo
            validated_count=$((validated_count + 1))
            continue
        fi

        # Check if it's a single-architecture manifest
        if echo "$manifest_info" | grep -q '"mediaType".*"application/vnd.docker.distribution.manifest.v2+json"' || \
           echo "$manifest_info" | grep -q '"mediaType".*"application/vnd.oci.image.manifest.v1+json"'; then
            # Extract the specific architecture
            if command -v jq &> /dev/null; then
                arch_info=$(skopeo inspect "docker://$image_ref_for_inspect" 2>/dev/null | jq -r '"\(.Os)/\(.Architecture)"' 2>/dev/null || echo "")
                if [ -n "$arch_info" ] && [ "$arch_info" != "/" ]; then
                    log_error "Single-architecture manifest: $arch_info"
                else
                    log_error "Single-architecture manifest"
                fi
            else
                log_error "Single-architecture manifest"
            fi
            echo "  Image: $image_ref"
            echo "  File(s):"
            for file in "${source_files[@]}"; do
                echo "    - $file"
            done
            echo
            has_errors=1
        else
            log_warning "Unknown manifest type: $image_ref"
            echo "  File(s):"
            for file in "${source_files[@]}"; do
                echo "    - $file"
            done
            echo
            has_errors=1
        fi
        echo
    done

    if [ $has_errors -ne 0 ]; then
        exit 1
    fi

    # Show success summary
    echo ""
    log_success "All $validated_count digest(s) are manifest lists"
}

# Run main function
main "$@"
