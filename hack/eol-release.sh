#!/usr/bin/env bash
#
# Mark a release branch as end-of-life in the CI and Renovate configuration.
#
# Given the GA tag that just shipped (e.g. 2.1.0), compute the release branch
# that is now two GA releases behind (release-1.9) and remove it from:
#   - .github/workflows/nightly.yaml                          (nightly matrix)
#   - .github/workflows/nightly-upgrade-test.yaml             (upgrade paths)
#   - .github/workflows/nightly-postgresql-upgrade-test.yaml  (upgrade paths)
#   - .github/renovate.json                                   (disable updates)
#
# Dry-run by default; pass --write to apply the edits.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/eol-release.XXXXXX")"
trap 'rm -rf "${WORK}"' EXIT

TAG=""
TAGS_FILE=""
WRITE=0
CHANGED=0

log()  { printf '%s\n' "$*"; }
warn() { printf 'warning: %s\n' "$*" >&2; }
die()  { printf 'error: %s\n' "$*" >&2; exit 2; }

usage() {
    cat <<'EOF'
Usage:
  ./hack/eol-release.sh --tag <GA tag> [--write] [--root <dir>] [--tags-file <file>]

Options:
  --tag <x.y.0>     GA tag that just shipped, e.g. 2.1.0 (required)
  --write           Apply the edits (default: dry-run, prints a diff)
  --root <dir>      Repository root (default: parent of this script)
  --tags-file <f>   Read GA tags from a file instead of `git tag` (tests)
  -h, --help        Show this help
EOF
}

parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --tag)       [[ $# -ge 2 ]] || die "--tag needs a value"; TAG="$2"; shift 2 ;;
            --root)      [[ $# -ge 2 ]] || die "--root needs a value"; ROOT="$2"; shift 2 ;;
            --tags-file) [[ $# -ge 2 ]] || die "--tags-file needs a value"; TAGS_FILE="$2"; shift 2 ;;
            --write)     WRITE=1; shift ;;
            --dry-run)   WRITE=0; shift ;;
            -h|--help)   usage; exit 0 ;;
            *) die "unknown argument: $1" ;;
        esac
    done
    [[ -n "${TAG}" ]] || { usage >&2; die "--tag is required"; }
    [[ "${TAG}" =~ ^[0-9]+\.[0-9]+\.0$ ]] || die "not a GA tag (expected x.y.0): ${TAG}"
    [[ -d "${ROOT}/.github" ]] || die "not a repository root: ${ROOT}"
}

# Sorted GA tags, oldest first.
ga_tags() {
    local list
    if [[ -n "${TAGS_FILE}" ]]; then
        list="$(cat "${TAGS_FILE}")"
    else
        list="$(git -C "${ROOT}" tag --list)"
    fi
    printf '%s\n' "${list}" | grep -E '^[0-9]+\.[0-9]+\.0$' | sort -V || true
}

# Print the release branch two GA tags behind $1; return 1 without history.
eol_branch_for_tag() {
    local tag="$1" list idx eol_tag
    list="$(ga_tags)"
    if ! printf '%s\n' "${list}" | grep -qxF -- "${tag}"; then
        list="$(printf '%s\n%s\n' "${list}" "${tag}" | grep -E '^[0-9]+\.[0-9]+\.0$' | sort -V -u)"
    fi
    idx="$(printf '%s\n' "${list}" | grep -nxF -- "${tag}" | cut -d: -f1)"
    [[ -n "${idx}" ]] || die "tag ${tag} not found"
    (( idx >= 3 )) || return 1
    eol_tag="$(printf '%s\n' "${list}" | sed -n "$((idx - 2))p")"
    printf 'release-%s\n' "${eol_tag%.0}"
}

# Swap $1 for $2, respecting dry-run and recording whether anything changed.
commit_file() {
    local file="$1" tmp="$2" rel
    rel="${file#"${ROOT}"/}"
    if cmp -s "${file}" "${tmp}"; then
        log "unchanged  ${rel}"
        return 0
    fi
    CHANGED=1
    log "update     ${rel}"
    if (( WRITE )); then
        mv "${tmp}" "${file}"
    else
        diff -u "${file}" "${tmp}" || true
    fi
}

# Drop "- <branch>" from a YAML matrix list.
edit_nightly_matrix() {
    local file="$1" branch="$2" tmp
    tmp="${WORK}/nightly.yaml.new"
    if ! grep -qE "^[[:space:]]*-[[:space:]]*${branch}[[:space:]]*$" "${file}"; then
        log "no nightly matrix entry for ${branch} (already removed)"
        return 0
    fi
    awk -v b="${branch}" '
        $0 ~ "^[[:space:]]*-[[:space:]]*" b "[[:space:]]*$" { next }
        { print }
    ' "${file}" > "${tmp}"
    commit_file "${file}" "${tmp}"
}

# Drop "- from_branch: X" / "to_branch: Y" pairs mentioning $branch.
# Leaves the file untouched (with a warning) if this would empty the matrix.
edit_upgrade_matrix() {
    local file="$1" branch="$2" tmp remaining
    tmp="${WORK}/$(basename "${file}").new"
    if ! grep -qE "^[[:space:]]*(-[[:space:]]*)?(from|to)_branch:[[:space:]]*${branch}[[:space:]]*$" "${file}"; then
        log "no upgrade path for ${branch} in $(basename "${file}") (already removed)"
        return 0
    fi
    awk -v b="${branch}" '
        { line[NR] = $0 }
        END {
            removed = 0
            for (i = 1; i <= NR; i++) {
                if (line[i] ~ /^[[:space:]]*-[[:space:]]*from_branch:/ && line[i+1] ~ /^[[:space:]]*to_branch:/) {
                    f = line[i];   sub(/.*from_branch:[[:space:]]*/, "", f); sub(/[[:space:]]+$/, "", f)
                    t = line[i+1]; sub(/.*to_branch:[[:space:]]*/, "", t);   sub(/[[:space:]]+$/, "", t)
                    if (f == b || t == b) { i++; removed++; continue }
                }
                print line[i]
            }
            if (removed == 0) exit 3
        }
    ' "${file}" > "${tmp}" || die "failed to rewrite ${file}"
    remaining="$(grep -cE '^[[:space:]]*-[[:space:]]*from_branch:' "${tmp}" || true)"
    if [[ "${remaining}" -eq 0 ]]; then
        warn "$(basename "${file}") would have an empty matrix; left unchanged, fix it by hand"
        rm -f "${tmp}"
        return 0
    fi
    commit_file "${file}" "${tmp}"
}

# Append an "enabled: false" rule for the branch to packageRules.
# Appended last on purpose: in Renovate, later matching rules win.
edit_renovate() {
    local file="$1" branch="$2" tmp
    tmp="${WORK}/renovate.json.new"
    if grep -qF "\"matchBaseBranches\": [\"${branch}\"]" "${file}"; then
        log "renovate already disables ${branch}"
        return 0
    fi
    awk -v b="${branch}" '
        { line[NR] = $0 }
        END {
            depth = 0; endline = 0
            for (i = 1; i <= NR && !endline; i++) {
                if (depth == 0 && line[i] !~ /"packageRules"[[:space:]]*:[[:space:]]*\[/) continue
                n = length(line[i]); instr = 0
                for (j = 1; j <= n; j++) {
                    c = substr(line[i], j, 1)
                    if (c == "\"") instr = !instr
                    else if (!instr) {
                        if (c == "[") depth++
                        else if (c == "]") { depth--; if (depth == 0) { endline = i; break } }
                    }
                }
            }
            if (!endline) exit 5
            if (line[endline-1] !~ /^[[:space:]]*\}[[:space:]]*$/) exit 4
            for (i = 1; i < endline; i++) {
                if (i == endline - 1) print line[i] ","; else print line[i]
            }
            print "    {"
            print "      \"description\": \"" b " is end-of-life: no dependency updates\","
            print "      \"matchBaseBranches\": [\"" b "\"],"
            print "      \"enabled\": false"
            print "    }"
            for (i = endline; i <= NR; i++) print line[i]
        }
    ' "${file}" > "${tmp}" || die "could not insert packageRules entry in ${file}"
    if command -v jq >/dev/null 2>&1; then
        jq -e . "${tmp}" >/dev/null || die "renovate.json would be invalid JSON"
    fi
    commit_file "${file}" "${tmp}"
}

write_outputs() {
    local eol_branch="$1" changed="$2"
    printf 'EOL_BRANCH=%s\nCHANGED=%s\n' "${eol_branch}" "${changed}"
    if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
        {
            printf 'eol_branch=%s\n' "${eol_branch}"
            printf 'eol_version=%s\n' "${eol_branch#release-}"
            printf 'changed=%s\n' "${changed}"
        } >> "${GITHUB_OUTPUT}"
    fi
    if [[ -n "${GITHUB_ENV:-}" ]]; then
        printf 'TAG=%s\nEOL_BRANCH=%s\n' "${TAG}" "${eol_branch}" >> "${GITHUB_ENV}"
    fi
}

main() {
    parse_args "$@"

    local eol_branch changed=false
    if ! eol_branch="$(eol_branch_for_tag "${TAG}")"; then
        log "not enough GA history for ${TAG}; nothing to do"
        write_outputs "" "false"
        exit 0
    fi
    log "tag=${TAG} eol=${eol_branch}"

    edit_nightly_matrix "${ROOT}/.github/workflows/nightly.yaml" "${eol_branch}"
    edit_upgrade_matrix "${ROOT}/.github/workflows/nightly-upgrade-test.yaml" "${eol_branch}"
    edit_upgrade_matrix "${ROOT}/.github/workflows/nightly-postgresql-upgrade-test.yaml" "${eol_branch}"
    edit_renovate "${ROOT}/.github/renovate.json" "${eol_branch}"

    (( CHANGED )) && changed=true
    if [[ "${changed}" == "false" ]]; then
        log "nothing to do"
    elif (( ! WRITE )); then
        log "dry-run: no files written (use --write to apply)"
    fi
    write_outputs "${eol_branch}" "${changed}"
}

main "$@"