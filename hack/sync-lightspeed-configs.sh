#!/usr/bin/env bash
#
# Sync vendored Lightspeed config snippets from upstream.
#

set -euo pipefail

UPSTREAM_REPO="redhat-ai-dev/lightspeed-configs"
UPSTREAM_CONFIG_PATH="llama-stack-configs/config.yaml"
UPSTREAM_STACK_PATH="lightspeed-core-configs/lightspeed-stack.yaml"
UPSTREAM_PROFILE_PATH="lightspeed-core-configs/rhdh-profile.py"
UPSTREAM_ENV_PATH="env/default-values.env"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

CONFIGMAP_FILE="${REPO_ROOT}/config/profile/rhdh/default-config/flavours/lightspeed/configmap-files.yaml"
EXAMPLE_SECRET_FILE="${REPO_ROOT}/examples/lightspeed.yaml"

REF="main"
UPDATED=0
TMP_DIR=""

usage() {
    cat <<'EOF'
Usage:
  ./hack/sync-lightspeed-configs.sh [--ref <branch-or-tag>]
EOF
}

parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --ref)
                REF="$2"
                shift 2
                ;;
            -h|--help)
                usage
                exit 0
                ;;
            *)
                usage >&2
                exit 1
                ;;
        esac
    done
}

fetch_upstream_file() {
    curl -fsSL --get \
        -H "Accept: application/vnd.github.raw" \
        -H "User-Agent: rhdh-operator-sync-lightspeed-configs" \
        --data-urlencode "ref=${REF}" \
        "https://api.github.com/repos/${UPSTREAM_REPO}/contents/$1" \
        -o "$2"
}

indent_file() {
    sed 's/^/    /' "$1"
}

render_secret_entries() {
    awk -F= '
        { sub(/\r$/, "") }
        /^[[:space:]]*($|#)/ { next }
        $1 == "LIGHTSPEED_CORE_IMAGE" || $1 == "RAG_CONTENT_IMAGE" || seen[$1]++ { next }
        { print "  " $1 ": \"\"" }
    ' "$1"
}

render_lightspeed_stack_yaml() {
    local source_file="$1"
    local destination_file="$2"

    cp "$source_file" "$destination_file"
    cat <<'EOF' >> "$destination_file"
mcp_servers:
  - name: mcp-integration-tools
    provider_id: "model-context-protocol"
    url: "http://localhost:7007/api/mcp-actions/v1"
    authorization_headers:
      Authorization: "client"
EOF
}

cleanup() {
    local exit_code=$?

    if [[ -n "${TMP_DIR:-}" ]]; then
        rm -rf "$TMP_DIR" || true
    fi

    return "$exit_code"
}

replace_indented_block() {
    local file="$1"
    local marker="$2"
    local indent="$3"
    local replacement="$4"
    local tmp

    tmp="${TMP_DIR}/$(basename "$file").tmp"

    awk \
        -v marker="$marker" \
        -v indent="$indent" \
        -v replacement="$replacement" '
        BEGIN {
            prefix = sprintf("%" indent "s", "")
            while ((getline line < replacement) > 0) {
                lines[++n] = line
            }
            close(replacement)
        }

        $0 == marker {
            print
            replaced++
            skip = 1
            next
        }

        skip && ($0 == "" || index($0, prefix) == 1) {
            next
        }

        skip {
            for (i = 1; i <= n; i++) {
                print lines[i]
            }
            skip = 0
        }

        { print }

        END {
            if (skip) {
                for (i = 1; i <= n; i++) {
                    print lines[i]
                }
            }

            if (replaced != 1) {
                exit 1
            }
        }
    ' "$file" > "$tmp"

    if ! cmp -s "$file" "$tmp"; then
        mv "$tmp" "$file"
        UPDATED=1
    else
        rm -f "$tmp"
    fi
}

main() {
    parse_args "$@"

    TMP_DIR="$(mktemp -d)"
    trap cleanup EXIT

    local config_file="${TMP_DIR}/config.yaml"
    local stack_file="${TMP_DIR}/lightspeed-stack.yaml"
    local rendered_stack_file="${TMP_DIR}/lightspeed-stack.rendered.yaml"
    local profile_file="${TMP_DIR}/rhdh-profile.py"
    local env_file="${TMP_DIR}/default-values.env"
    local config_block="${TMP_DIR}/config-block.yaml"
    local stack_block="${TMP_DIR}/stack-block.yaml"
    local profile_block="${TMP_DIR}/profile-block.yaml"
    local secret_entries="${TMP_DIR}/secret-entries.yaml"

    fetch_upstream_file "$UPSTREAM_CONFIG_PATH" "$config_file"
    fetch_upstream_file "$UPSTREAM_STACK_PATH" "$stack_file"
    fetch_upstream_file "$UPSTREAM_PROFILE_PATH" "$profile_file"
    fetch_upstream_file "$UPSTREAM_ENV_PATH" "$env_file"

    indent_file "$config_file" > "$config_block"
    render_lightspeed_stack_yaml "$stack_file" "$rendered_stack_file"
    indent_file "$rendered_stack_file" > "$stack_block"
    indent_file "$profile_file" > "$profile_block"
    render_secret_entries "$env_file" > "$secret_entries"

    replace_indented_block "$CONFIGMAP_FILE" "  config.yaml: |" 4 "$config_block"
    replace_indented_block "$CONFIGMAP_FILE" "  lightspeed-stack.yaml: |" 4 "$stack_block"
    replace_indented_block "$CONFIGMAP_FILE" "  rhdh-profile.py: |" 4 "$profile_block"
    replace_indented_block "$EXAMPLE_SECRET_FILE" "stringData:" 2 "$secret_entries"

    printf 'Synced Lightspeed content from %s@%s\n' "$UPSTREAM_REPO" "$REF"
    [[ "$UPDATED" -eq 1 ]] || printf 'already up to date\n'
}

main "$@"
