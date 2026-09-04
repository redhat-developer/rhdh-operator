#!/usr/bin/env bash
#
# Sync vendored Lightspeed Core config snippets from upstream.
#

set -euo pipefail

UPSTREAM_REPO="redhat-ai-dev/lightspeed-configs"
UPSTREAM_STACK_PATH="lightspeed-core-configs/lightspeed-stack.yaml"
UPSTREAM_PROFILE_PATH="lightspeed-core-configs/rhdh-profile.py"
UPSTREAM_ENV_PATH="env/default-values.env"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

CONFIGMAP_FILE="${REPO_ROOT}/config/profile/rhdh/default-config/flavours/intelligent-assistant/configmap-files.yaml"
EXAMPLE_SECRET_FILE="${REPO_ROOT}/examples/intelligent-assistant.yaml"

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

# Upstream env keys that are not user Secret fields.
# Images stay hardcoded in deployment.yaml; storage, OTEL, and logging are sidecar defaults.
SKIP_SECRET_KEYS=(
    LIGHTSPEED_CORE_IMAGE
    RAG_CONTENT_IMAGE
    KV_STORE_PATH
    SQL_STORE_PATH
    SQLITE_STORE_DIR
    OTEL_SDK_DISABLED
    LLAMA_STACK_LOGGING
    GOOGLE_APPLICATION_CREDENTIALS_HOST_PATH
)

render_secret_entries() {
    # Comma-separated: BSD awk (macOS) rejects newlines in -v values.
    local skip
    skip="$(IFS=,; printf '%s' "${SKIP_SECRET_KEYS[*]}")"

    awk -F= -v skip="$skip" '
        BEGIN {
            n = split(skip, keys, ",")
            for (i = 1; i <= n; i++) {
                if (keys[i] != "") skipset[keys[i]] = 1
            }
        }
        { sub(/\r$/, "") }
        /^[[:space:]]*($|#)/ { next }
        skipset[$1] || seen[$1]++ { next }
        { print "  " $1 ": \"\"" }
    ' "$1"
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

    local stack_file="${TMP_DIR}/lightspeed-stack.yaml"
    local profile_file="${TMP_DIR}/rhdh-profile.py"
    local env_file="${TMP_DIR}/default-values.env"
    local stack_block="${TMP_DIR}/stack-block.yaml"
    local profile_block="${TMP_DIR}/profile-block.yaml"
    local secret_entries="${TMP_DIR}/secret-entries.yaml"

    fetch_upstream_file "$UPSTREAM_STACK_PATH" "$stack_file"
    fetch_upstream_file "$UPSTREAM_PROFILE_PATH" "$profile_file"
    fetch_upstream_file "$UPSTREAM_ENV_PATH" "$env_file"

    indent_file "$stack_file" > "$stack_block"
    indent_file "$profile_file" > "$profile_block"
    render_secret_entries "$env_file" > "$secret_entries"

    replace_indented_block "$CONFIGMAP_FILE" "  lightspeed-stack.yaml: |" 4 "$stack_block"
    replace_indented_block "$CONFIGMAP_FILE" "  rhdh-profile.py: |" 4 "$profile_block"
    replace_indented_block "$EXAMPLE_SECRET_FILE" "stringData:" 2 "$secret_entries"

    printf 'Synced Lightspeed content from %s@%s\n' "$UPSTREAM_REPO" "$REF"
    [[ "$UPDATED" -eq 1 ]] || printf 'already up to date\n'
}

main "$@"
