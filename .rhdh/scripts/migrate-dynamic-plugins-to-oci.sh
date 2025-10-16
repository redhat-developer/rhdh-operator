#!/bin/bash
#
# This script is to help migrate from bundled plugin wrappers to OCI artifacts. It detects the current RHDH installation, extracts the catalog index, and provides migration suggestions for airgap environments.
#
# Requires: oc (OCP) or kubectl (K8s), jq, yq, umoci, base64, opm, skopeo

set -euo pipefail

CATALOG_INDEX="quay.io/rhdh/plugin-catalog-index:1.8"
NAMESPACE=""
BACKSTAGE_NAME=""
TMPDIR=$(mktemp -d)

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

function log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

function log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

function log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

function log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

cleanup() {
    if [[ -d "$TMPDIR" ]]; then
        rm -rf "$TMPDIR"
    fi
}
trap cleanup EXIT

function usage() {
    echo "Migration Script for Dynamic Plugins to OCI Artifacts

This script is to help migrate from bundled plugin wrappers to OCI artifacts.
It detects the current RHDH installation, extracts the catalog index,
 and provides migration suggestions for airgap environments.

Usage: $0 [OPTIONS]

Options:
  --catalog-index <image>     : Catalog index image (default: $CATALOG_INDEX)
  --namespace <namespace>     : Namespace where RHDH is installed (auto-detected if not specified)
  --backstage-name <name>     : Name of the Backstage instance (auto-detected if not specified)
  --help                      : Show this help message

Examples:
  # Basic migration analysis
  $0

  # Analyze specific installation
  $0 --namespace rhdh-operator --backstage-name developer-hub"
}

function parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            --catalog-index)
                CATALOG_INDEX="$2"
                shift 2
                ;;
            --namespace)
                NAMESPACE="$2"
                shift 2
                ;;
            --backstage-name)
                BACKSTAGE_NAME="$2"
                shift 2
                ;;
            --help)
                usage
                exit 0
                ;;
            *)
                log_error "Unknown option: $1"
                usage
                exit 1
                ;;
        esac
    done
}

function check_dependencies() {
    local missing_tools=()

    for tool in oc skopeo umoci jq yq; do
        if ! command -v "$tool" &> /dev/null; then
            missing_tools+=("$tool")
        fi
    done

    if [[ ${#missing_tools[@]} -gt 0 ]]; then
        log_error "Missing required tools: ${missing_tools[*]}"
        log_error "Please install the missing tools and try again."
        exit 1
    fi
}

function detect_cluster() {
    log_info "Detecting cluster environment..."

    if ! oc cluster-info &> /dev/null; then
        log_error "Not connected to a Kubernetes/OpenShift cluster"
        log_error "Please ensure you're logged in with 'oc login' or 'kubectl'"
        exit 1
    fi

    local cluster_info
    cluster_info=$(oc cluster-info | head -1)
    log_success "Connected to cluster: $cluster_info"

    # Detect if it's OpenShift
    if oc get clusterversion &> /dev/null; then
        log_info "Detected OpenShift cluster"
        local ocp_version
        ocp_version=$(oc get clusterversion -o jsonpath='{.items[0].status.desired.version}')
        log_info "OpenShift version: $ocp_version"
    else
        log_info "Detected Kubernetes cluster"
    fi
}

function detect_rhdh_installation() {
    log_info "Auto-detecting RHDH installation..."

    if [[ -z "$NAMESPACE" ]]; then
        # Find namespaces with Backstage instances
        local namespaces
        namespaces=$(oc get backstage --all-namespaces -o jsonpath='{range .items[*]}{.metadata.namespace}{"\n"}{end}' | sort -u)

        if [[ -z "$namespaces" ]]; then
            log_error "No Backstage instances found in the cluster"
            exit 1
        fi

        # Use the first namespace found
        NAMESPACE=$(echo "$namespaces" | head -1)
        log_info "Auto-detected namespace: $NAMESPACE"
    fi

    if [[ -z "$BACKSTAGE_NAME" ]]; then
        # Find Backstage instances in the namespace
        local backstage_instances
        backstage_instances=$(oc get backstage -n "$NAMESPACE" -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}')

        if [[ -z "$backstage_instances" ]]; then
            log_error "No Backstage instances found in namespace: $NAMESPACE"
            exit 1
        fi

        # Use the first instance found
        BACKSTAGE_NAME=$(echo "$backstage_instances" | head -1)
        log_info "Auto-detected Backstage instance: $BACKSTAGE_NAME"
    fi

    log_success "Target installation: $BACKSTAGE_NAME in namespace $NAMESPACE"
}

function extract_catalog_index() {
    log_info "Extracting catalog index: $CATALOG_INDEX"

    local catalog_dir="$TMPDIR/catalog-index"
    mkdir -p "$catalog_dir"

    # Copy the catalog index image
    log_info "Downloading catalog index image..."
    if ! skopeo copy "docker://$CATALOG_INDEX" "oci:$catalog_dir:latest"; then
        log_error "Failed to download catalog index image"
        exit 1
    fi

    # Unpack the image
    log_info "Unpacking catalog index..."
    if ! umoci unpack --image "$catalog_dir:latest" "$catalog_dir/unpacked" --rootless; then
        log_error "Failed to unpack catalog index"
        exit 1
    fi

    log_success "Catalog index extracted successfully"

    # Store paths for later use
    CATALOG_INDEX_JSON="$catalog_dir/unpacked/rootfs/index.json"
    CATALOG_DYNAMIC_PLUGINS="$catalog_dir/unpacked/rootfs/dynamic-plugins.default.yaml"
    # Debug: Check file structure and copy catalog index
    log_info "Debug: Checking catalog index file structure..."
    log_info "Debug: Catalog index file: $CATALOG_INDEX_JSON"
    log_info "Debug: File exists: $(test -f "$CATALOG_INDEX_JSON" && echo "YES" || echo "NO")"
    log_info "Debug: File size: $(test -f "$CATALOG_INDEX_JSON" && wc -c < "$CATALOG_INDEX_JSON" || echo "N/A")"
    log_info "Debug: Directory contents: $(ls -la "$(dirname "$CATALOG_INDEX_JSON")" 2>/dev/null || echo "N/A")"

    if [[ -f "$CATALOG_INDEX_JSON" ]]; then
        cp "$CATALOG_INDEX_JSON" "/tmp/debug-catalog-index.json"
        log_info "Debug: Catalog index copied to /tmp/debug-catalog-index.json"
        
        # Debug: Show what's in the catalog index
        log_info "Debug: Catalog index contains $(jq 'length' "$CATALOG_INDEX_JSON") plugins:"
        jq -r 'keys[]' "$CATALOG_INDEX_JSON" | head -10 | while read -r plugin; do
            log_info "  - $plugin"
        done
        if [[ $(jq 'length' "$CATALOG_INDEX_JSON") -gt 10 ]]; then
            log_info "  ... and $(($(jq 'length' "$CATALOG_INDEX_JSON") - 10)) more plugins"
        fi
    else
        log_error "Debug: Catalog index file not found!"
    fi
    
    # Extract bundled plugins from the catalog's dynamic-plugins.default.yaml
    if [[ -f "$CATALOG_DYNAMIC_PLUGINS" ]]; then
        log_info "Debug: Found dynamic-plugins.default.yaml in catalog index"
        local catalog_bundled_plugins="$TMPDIR/catalog-bundled-plugins.txt"
        grep "\./dynamic-plugins/dist/" "$CATALOG_DYNAMIC_PLUGINS" | sed 's/.*package: *\.\/dynamic-plugins\/dist\///' | sed 's/-dynamic$//' > "$catalog_bundled_plugins" 2>/dev/null || true
        
        if [[ -f "$catalog_bundled_plugins" && -s "$catalog_bundled_plugins" ]]; then
            local catalog_bundled_count
            catalog_bundled_count=$(wc -l < "$catalog_bundled_plugins")
            log_info "Debug: Found $catalog_bundled_count bundled plugins in catalog index"
            
            log_info "Debug: Bundled plugins in catalog index:"
            while IFS= read -r plugin; do
                log_info "  - $plugin"
            done < "$catalog_bundled_plugins"
        else
            log_info "Debug: No bundled plugins found in catalog index"
        fi
    else
        log_warn "Debug: dynamic-plugins.default.yaml not found in catalog index"
    fi
}

function analyze_current_plugins() {
    log_info "Analyzing current plugin configuration..."

    local configmap_name="backstage-dynamic-plugins-$BACKSTAGE_NAME"

    # Check if the configmap exists
    if ! oc get configmap "$configmap_name" -n "$NAMESPACE" &> /dev/null; then
        log_error "Dynamic plugins configmap not found: $configmap_name"
        exit 1
    fi

    # Extract current configuration
    local current_config="$TMPDIR/current-dynamic-plugins.yaml"
    oc get configmap "$configmap_name" -n "$NAMESPACE" -o jsonpath='{.data.dynamic-plugins\.yaml}' > "$current_config"
    
    log_success "Current plugin configuration extracted"
    
    # Debug: Show what's in the current config
    log_info "Debug: Current dynamic-plugins.yaml content:"
    if [[ -f "$current_config" && -s "$current_config" ]]; then
        cat "$current_config" | head -20
        if [[ $(wc -l < "$current_config") -gt 20 ]]; then
            log_info "... (truncated, showing first 20 lines)"
        fi
    else
        log_warn "Current config file is empty or doesn't exist"
    fi

        # Check if dynamic-plugins.default.yaml is included
        if grep -q "dynamic-plugins.default.yaml" "$current_config"; then
        log_info "Configuration includes dynamic-plugins.default.yaml"
        
        # Use bundled plugins from catalog index instead of container extraction
        local catalog_bundled_plugins="$TMPDIR/catalog-bundled-plugins.txt"
        if [[ -f "$CATALOG_DYNAMIC_PLUGINS" ]]; then
            log_info "Extracting bundled plugins from catalog index..."
            grep "\./dynamic-plugins/dist/" "$CATALOG_DYNAMIC_PLUGINS" | sed 's/.*package: *\.\/dynamic-plugins\/dist\///' | sed 's/-dynamic$//' > "$catalog_bundled_plugins" 2>/dev/null || true
            
            if [[ -f "$catalog_bundled_plugins" && -s "$catalog_bundled_plugins" ]]; then
                local bundled_count
                bundled_count=$(wc -l < "$catalog_bundled_plugins")
                log_warn "Found $bundled_count bundled plugins in catalog index that could be migrated"
                
                log_info "Bundled plugins available for migration:"
                while IFS= read -r plugin; do
                    log_info "  - $plugin"
                done < "$catalog_bundled_plugins"
            else
                log_info "No bundled plugins found in catalog index"
            fi
        else
            log_warn "Catalog dynamic-plugins.default.yaml not found"
        fi
    fi

    # Analyze custom bundled plugins in the configmap
    local custom_bundled_plugins
    custom_bundled_plugins=$(grep -c "\./dynamic-plugins/dist/" "$current_config" || true)

    if [[ "$custom_bundled_plugins" -gt 0 ]]; then
        log_warn "Found $custom_bundled_plugins custom bundled plugin references that need migration"

        # Extract bundled plugin names
        local bundled_plugin_names="$TMPDIR/bundled-plugins.txt"
        grep "\./dynamic-plugins/dist/" "$current_config" | sed 's/.*package: *\.\/dynamic-plugins\/dist\///' | sed 's/-dynamic$//' > "$bundled_plugin_names"

        log_info "Custom bundled plugins found:"
        while IFS= read -r plugin; do
            log_info "  - $plugin"
        done < "$bundled_plugin_names"
    fi

    # Combine all bundled plugins for analysis
    local all_bundled_plugins="$TMPDIR/all-bundled-plugins.txt"
    if [[ -f "${catalog_bundled_plugins:-}" ]]; then
        cp "$catalog_bundled_plugins" "$all_bundled_plugins"
    else
        touch "$all_bundled_plugins"
    fi
    
    if [[ -f "${bundled_plugin_names:-}" ]]; then
        cat "$bundled_plugin_names" >> "$all_bundled_plugins"
    fi

    # Remove duplicates
    sort -u "$all_bundled_plugins" > "$all_bundled_plugins.tmp" && mv "$all_bundled_plugins.tmp" "$all_bundled_plugins"
    
    local total_bundled
    total_bundled=$(wc -l < "$all_bundled_plugins" 2>/dev/null || echo "0")
    
    if [[ "$total_bundled" -gt 0 ]]; then
        log_warn "Total bundled plugins that could be migrated: $total_bundled"
    else
        log_success "No bundled plugins found - migration may not be needed"
    fi
    
    # Store for later use
    # CURRENT_CONFIG="$current_config"  # Not currently used
    ALL_BUNDLED_PLUGINS="$all_bundled_plugins"
}

function find_oci_equivalents() {
    log_info "Finding OCI equivalents for bundled plugins..."
    
    local bundled_plugins="${ALL_BUNDLED_PLUGINS:-$TMPDIR/bundled-plugins.txt}"
    local oci_mapping="$TMPDIR/oci-mapping.json"
    
    if [[ ! -f "$bundled_plugins" || ! -s "$bundled_plugins" ]]; then
        log_info "No bundled plugins to migrate"
        return 0
    fi
    
    # Create mapping file
    echo "{}" > "$oci_mapping"
    
    # Process each bundled plugin
    while IFS= read -r plugin; do
        log_info "Looking for OCI equivalent for: $plugin"
        
        # Try multiple matching strategies
        local oci_ref=""
        local base_plugin_name="$plugin"
        
        # Strategy 1: Remove -dynamic suffix
        if [[ "$plugin" == *"-dynamic" ]]; then
            base_plugin_name="${plugin%-dynamic}"
            log_info "  Trying base name: $base_plugin_name"
        fi
        
        # Strategy 2: Try exact match first
        log_info "  Checking catalog index file: $CATALOG_INDEX_JSON"
        log_info "  Plugin name: '$plugin'"
        
        # Test the jq command directly
        local test_result
        test_result=$(jq -r --arg plugin "$plugin" 'to_entries[] | select(.key == $plugin) | .value.registryReference' "$CATALOG_INDEX_JSON" 2>/dev/null || echo "")
        log_info "  Direct jq test result: '$test_result'"
        
        oci_ref=$(jq -r --arg plugin "$plugin" '
            to_entries[] | 
            select(.key == $plugin) | 
            .value.registryReference' "$CATALOG_INDEX_JSON" 2>/dev/null || echo "")
        log_info "  Exact match result: '$oci_ref'"
        
        # Strategy 3: Try base name match
        if [[ -z "$oci_ref" && "$base_plugin_name" != "$plugin" ]]; then
            oci_ref=$(jq -r --arg plugin "$base_plugin_name" '
                to_entries[] | 
                select(.key == $plugin) | 
                .value.registryReference' "$CATALOG_INDEX_JSON" 2>/dev/null || echo "")
        fi
        
        # Strategy 4: Try contains match for base name
        if [[ -z "$oci_ref" && "$base_plugin_name" != "$plugin" ]]; then
            oci_ref=$(jq -r --arg plugin "$base_plugin_name" '
                to_entries[] | 
                select(.key | contains($plugin)) | 
                .value.registryReference' "$CATALOG_INDEX_JSON" 2>/dev/null || echo "")
        fi
        
        # Strategy 5: Try contains match for original name
        if [[ -z "$oci_ref" ]]; then
            oci_ref=$(jq -r --arg plugin "$plugin" '
                to_entries[] | 
                select(.key | contains($plugin)) | 
                .value.registryReference' "$CATALOG_INDEX_JSON" 2>/dev/null || echo "")
        fi
        
        log_info "  Final oci_ref value: '$oci_ref'"
        if [[ -n "$oci_ref" ]]; then
            log_success "Found OCI equivalent: $oci_ref"
            # Update mapping
            jq --arg plugin "$plugin" --arg oci "$oci_ref" '.[$plugin] = $oci' "$oci_mapping" > "$oci_mapping.tmp" && mv "$oci_mapping.tmp" "$oci_mapping"
        else
            log_warn "No OCI equivalent found for: $plugin"
        fi
    done < "$bundled_plugins"
    
    OCI_MAPPING="$oci_mapping"
}


function display_results() {
    log_success "=== Migration Analysis Complete ==="
    echo
    
    log_info "Summary:"
    echo "  - Cluster: $(oc cluster-info | head -1)"
    echo "  - Target: $BACKSTAGE_NAME in namespace $NAMESPACE"
    echo "  - Catalog Index: $CATALOG_INDEX"
    echo
    
    if [[ -n "${OCI_MAPPING:-}" && -f "$OCI_MAPPING" ]]; then
        local migration_count
        migration_count=$(jq 'length' "$OCI_MAPPING")
        log_info "Plugins to migrate: $migration_count"
        
        if [[ "$migration_count" -gt 0 ]]; then
            echo
            log_info "Migration mappings:"
            jq -r 'to_entries[] | "  " + .key + " -> " + .value' "$OCI_MAPPING"
        fi
    fi
    
    
}

function main() {
    log_info "Starting Dynamic Plugins to OCI Artifacts Migration Analysis"
    echo
    
    parse_args "$@"
    check_dependencies
    detect_cluster
    detect_rhdh_installation
    extract_catalog_index
    analyze_current_plugins
    find_oci_equivalents
    display_results
    
    log_success "Migration analysis completed successfully!"
}
main "$@"
