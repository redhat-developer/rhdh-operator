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

# Cleanup function
cleanup() {
    if [[ -d "$TMPDIR" ]]; then
        rm -rf "$TMPDIR"
    fi
}
trap cleanup EXIT

function usage() {
    echo "Migration Script for Dynamic Plugins to OCI Artifacts

This script helps customers migrate from bundled plugin wrappers to OCI artifacts
from reg.rh.io. It detects the current RHDH installation, extracts the catalog index,
and provides migration suggestions.

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

# Parse command line arguments
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

# Check required tools
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

# Detect cluster type and connection
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

# Auto-detect RHDH installation
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

# Extract catalog index
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
    # CATALOG_DYNAMIC_PLUGINS="$catalog_dir/unpacked/rootfs/dynamic-plugins.default.yaml"  # Not currently used
    CATALOG_INDEX_JSON="$catalog_dir/unpacked/rootfs/index.json"
    # CATALOG_ENTITIES_DIR="$catalog_dir/unpacked/rootfs/catalog-entities"  # Not currently used
    
    # Debug: Check file structure and copy catalog index
    log_info "Debug: Checking catalog index file structure..."
    log_info "Debug: Catalog index file: $CATALOG_INDEX_JSON"
    log_info "Debug: File exists: $(test -f "$CATALOG_INDEX_JSON" && echo "YES" || echo "NO")"
    log_info "Debug: File size: $(test -f "$CATALOG_INDEX_JSON" && wc -c < "$CATALOG_INDEX_JSON" || echo "N/A")"
    log_info "Debug: Directory contents: $(ls -la "$(dirname "$CATALOG_INDEX_JSON")" 2>/dev/null || echo "N/A")"
    
    if [[ -f "$CATALOG_INDEX_JSON" ]]; then
        cp "$CATALOG_INDEX_JSON" "/tmp/debug-catalog-index.json"
        log_info "Debug: Catalog index copied to /tmp/debug-catalog-index.json"
    else
        log_error "Debug: Catalog index file not found!"
    fi
}

# Analyze current plugin configuration
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
    
        # Check if dynamic-plugins.default.yaml is included
        if grep -q "dynamic-plugins.default.yaml" "$current_config"; then
        log_info "Configuration includes dynamic-plugins.default.yaml"
        
        # Get the RHDH container image to extract the default config
        local rhdh_image
        rhdh_image=$(oc get deployment "backstage-$BACKSTAGE_NAME" -n "$NAMESPACE" -o jsonpath='{.spec.template.spec.containers[0].image}')
        log_info "Extracting bundled plugins from RHDH container: $rhdh_image"
        
        # Extract bundled plugins from the container
        local container_bundled_plugins="$TMPDIR/container-bundled-plugins.txt"
        oc run temp-extract-bundled --image="$rhdh_image" --rm -it --restart=Never --command -- grep "\./dynamic-plugins/dist/" /opt/app-root/src/dynamic-plugins.default.yaml | sed 's/.*package: *\.\/dynamic-plugins\/dist\///' | sed 's/-dynamic$//' | tr -d '\r' > "$container_bundled_plugins" 2>/dev/null || true
        
        if [[ -f "$container_bundled_plugins" && -s "$container_bundled_plugins" ]]; then
            local bundled_count
            bundled_count=$(wc -l < "$container_bundled_plugins")
            log_warn "Found $bundled_count bundled plugins in RHDH container that could be migrated"
            
            log_info "Bundled plugins in container:"
            while IFS= read -r plugin; do
                log_info "  - $plugin"
            done < "$container_bundled_plugins"
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
    if [[ -f "${container_bundled_plugins:-}" ]]; then
        cp "$container_bundled_plugins" "$all_bundled_plugins"
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

# Find OCI equivalents for bundled plugins
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

# Generate migration plan
function generate_migration_plan() {
    log_info "Generating migration plan..."
    
    local migration_plan
    migration_plan="/tmp/migration-plan-$(date +%Y%m%d-%H%M%S).yaml"
    
    cat > "$migration_plan" << EOF
# Migration Plan for Dynamic Plugins to OCI Artifacts
# Generated on: $(date)
# Source: $BACKSTAGE_NAME in namespace $NAMESPACE
# Catalog Index: $CATALOG_INDEX

migration:
  source:
    namespace: $NAMESPACE
    backstage_instance: $BACKSTAGE_NAME
    configmap: backstage-dynamic-plugins-$BACKSTAGE_NAME
  
  target:
    catalog_index: $CATALOG_INDEX
    registry: quay.io/rhdh-plugin-catalog
  
  plugins_to_migrate:
EOF

    # Add plugin mappings with proper OCI format
    if [[ -n "${OCI_MAPPING:-}" && -f "$OCI_MAPPING" ]]; then
        jq -r 'to_entries[] | "    - original_package: \"./dynamic-plugins/dist/" + .key + "\"\n      plugin_name: \"" + (.key | sub("-dynamic$"; "")) + "\"\n      oci_package: \"oci://" + .value + "!" + (.key | sub("-dynamic$"; "")) + "\"\n      disabled: false\n      migration_required: true"' "$OCI_MAPPING" >> "$migration_plan"
    fi
    
    cat >> "$migration_plan" << EOF

  what_to_do_next:
    step_1: "Copy the plugin mappings above"
    step_2: "Open your dynamic-plugins.yaml file"
    step_3: "Replace the old plugin lines with the new OCI lines"
    step_4: "Save the file"
    step_5: "Apply the changes to your cluster"
    
  example_before_and_after:
    before: "package: ./dynamic-plugins/dist/backstage-community-plugin-acr"
    after: "package: oci://quay.io/rhdh-plugin-catalog/backstage-community-plugin-acr:1.8.0--1.15.2!backstage-community-plugin-acr"
    
  important_notes:
    - "Keep a backup of your original file!"
    - "Test in a development environment first"
    - "Each plugin has a specific version - don't change the version numbers"
    
  copy_paste_ready_config:
    description: "Replace your current dynamic-plugins.yaml with this:"
    config: |
      dynamicPlugins:
        packages:
EOF

    # Add the actual config that users can copy-paste
    if [[ -n "${OCI_MAPPING:-}" && -f "$OCI_MAPPING" ]]; then
        echo "          # Migrated plugins:" >> "$migration_plan"
        jq -r 'to_entries[] | "          - package: \"oci://" + .value + "!" + (.key | sub("-dynamic$"; "")) + "\""' "$OCI_MAPPING" >> "$migration_plan"
    fi
    
    cat >> "$migration_plan" << EOF

EOF


    log_success "Migration plan generated: $migration_plan"
    MIGRATION_PLAN="$migration_plan"
    
    # Display the migration plan content
    echo
    log_info "=== Migration Plan Content ==="
    cat "$migration_plan"
    echo
}


# Display results
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
    
    echo
    log_info "Generated files:"
    echo "  - Migration Plan: $MIGRATION_PLAN"
    
}

# Main function
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
    generate_migration_plan
    display_results
    
    log_success "Migration analysis completed successfully!"
}

# Run main function
main "$@"
