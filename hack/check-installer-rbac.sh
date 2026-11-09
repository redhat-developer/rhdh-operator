#!/bin/bash
#
# Verify that the OLM v1 installer ClusterRole rendered by
# .rhdh/scripts/prepare-restricted-environment.sh can grant every permission
# the bundle asks for.
#
# OLM v1 installs a bundle with a dedicated installer ServiceAccount, and the
# API server refuses to create RBAC that grants permissions the installer does
# not hold itself ("attempting to grant RBAC permissions not currently held").
# The installer role is written out by hand in the prepare script, so it drifts
# whenever the operator's RBAC grows. This check keeps the two in sync.
#
# Usage:
#   ./hack/check-installer-rbac.sh
#
# Requirements:
#   - yq (https://github.com/mikefarah/yq)
#
# Exit codes:
#   0 - The installer role covers every bundle permission
#   1 - Permissions are missing, or the role/CSVs cannot be parsed
#

# The yq expressions below use $ variables that yq expands. The shell must not.
# shellcheck disable=SC2016

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
PREPARE_SCRIPT="${REPO_ROOT}/.rhdh/scripts/prepare-restricted-environment.sh"
BUNDLE_CSVS=(
    "${REPO_ROOT}/bundle/rhdh/manifests/backstage-operator.clusterserviceversion.yaml"
    "${REPO_ROOT}/bundle/backstage.io/manifests/backstage-operator.clusterserviceversion.yaml"
)
# ClusterRoles shipped in the bundle (editor/viewer/metrics): the installer
# creates them too, so it needs to hold their rules as well.
BUNDLE_CLUSTER_ROLES=("${REPO_ROOT}"/bundle/*/manifests/*clusterrole*.yaml)

RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'

if ! command -v yq &> /dev/null; then
    echo -e "${RED}ERROR:${NC} yq is required (https://github.com/mikefarah/yq)"
    exit 1
fi

tmp_dir="$(mktemp -d)"
trap 'rm -rf "${tmp_dir}"' EXIT

installer_role="${tmp_dir}/installer-role.yaml"
required="${tmp_dir}/required.txt"
granted="${tmp_dir}/granted.txt"

# Print one "group|resource|verb" line per rule selected by $2 in file $1.
rules_to_triples() {
    local file=$1
    local selector=$2
    {
        yq -r "(${selector})"' | (.apiGroups // [""])[] as $g | (.resources // [])[] as $r | .verbs[] as $v | "\($g)|\($r)|\($v)"' "${file}"
        yq -r "(${selector})"' | (.nonResourceURLs // [])[] as $u | .verbs[] as $v | "nonresource|\($u)|\($v)"' "${file}"
    } | sort -u
}

# The role is written by a heredoc in the prepare script. Take the rules from
# there instead of duplicating them in this check.
awk '
  /cat <</ && /clusterRole\.yaml/ { in_role = 1; next }
  in_role && /^EOF$/ { exit }
  in_role { print }
' "${PREPARE_SCRIPT}" > "${installer_role}"

if [[ ! -s "${installer_role}" ]]; then
    echo -e "${RED}ERROR:${NC} could not find the installer ClusterRole in ${PREPARE_SCRIPT}"
    exit 1
fi

# The bundle CSV is the contract OLM applies: clusterPermissions become a
# ClusterRole and permissions a namespaced Role, both created by the installer.
: > "${required}"
for csv in "${BUNDLE_CSVS[@]}"; do
    [[ -f "${csv}" ]] || continue
    rules_to_triples "${csv}" \
        '(.spec.install.spec.clusterPermissions // [])[].rules[], (.spec.install.spec.permissions // [])[].rules[]' \
        >> "${required}"
done

for role in "${BUNDLE_CLUSTER_ROLES[@]}"; do
    [[ -f "${role}" ]] || continue
    rules_to_triples "${role}" '.rules[]' >> "${required}"
done
sort -u -o "${required}" "${required}"

if [[ ! -s "${required}" ]]; then
    echo -e "${RED}ERROR:${NC} no bundle permissions found in ${BUNDLE_CSVS[*]}"
    exit 1
fi

rules_to_triples "${installer_role}" '.rules[]' > "${granted}"

missing="$(comm -23 "${required}" "${granted}")"
if [[ -n "${missing}" ]]; then
    echo -e "${RED}The OLM v1 installer ClusterRole cannot grant:${NC}"
    while IFS='|' read -r group resource verb; do
        printf '  %s %s [%s]\n' "${group:-core}" "${resource}" "${verb}"
    done <<< "${missing}"
    echo
    echo "Add the missing rules to the installer ClusterRole in"
    echo "  ${PREPARE_SCRIPT#"${REPO_ROOT}/"}"
    echo "so OLM v1 can install the bundle. Without them the operator ClusterRole"
    echo "creation is refused and the operator runs with no permissions."
    exit 1
fi

echo -e "${GREEN}Installer ClusterRole covers every bundle permission.${NC}"
