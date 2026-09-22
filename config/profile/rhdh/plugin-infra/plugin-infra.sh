#!/bin/bash
#
# Plugin Infrastructure Setup Script for RHDH with Orchestrator
#

set -euo pipefail

action="apply" # Default action
branch="main"  # Default branch
cicd=false   # Default CICD mode
olm_version="auto" # Default OLM version (v0, v1, or auto-detect)
resolved_olm_version=""

PLUGIN_INFRA_LABEL_VALUE="plugin-infra"
REDHAT_OPERATORS_CATALOG_LABEL="olm.operatorframework.io/metadata.name=openshift-redhat-operators"
SERVERLESS_OPERATOR_NAME="serverless-operator"
LOGIC_OPERATOR_NAME="logic-operator"

# Parse command-line options
while [[ $# -gt 0 ]]; do
  case "$1" in
    --with-cicd)
      cicd=true
      shift
      ;;
    --olm-version)
      olm_version="${2:?--olm-version requires v0, v1, or auto}"
      shift 2
      ;;
    apply|delete)
      action="$1"
      shift
      ;;
    --branch)
      branch="$2"
      shift 2
      ;;
    *)
      echo "Unknown option: $1"
      exit 1
      ;;
  esac
done

case "${olm_version}" in
  v0|v1|auto) ;;
  *)
    echo "Unknown --olm-version value: ${olm_version}. Must be v0, v1, or auto."
    exit 1
    ;;
esac

script_dir="$(dirname "$(realpath "$0")")"

kubectl_get() {
  kubectl get "$@" 2>&1
}

resource_exists() {
  local err exit_code
  err="$(kubectl_get "$@" 2>&1)"
  exit_code=$?
  if [[ ${exit_code} -eq 0 ]]; then
    return 0
  fi
  if grep -qiE '(not found|NotFound)' <<<"${err}"; then
    return 1
  fi
  echo "${err}" >&2
  exit 1
}

crd_exists() {
  resource_exists "crd/${1}"
}

is_helm_managed() {
  local api_resource="$1"
  local name="$2"
  local namespace="${3:-}"
  local release_name
  local args=("${api_resource}/${name}" -o "jsonpath={.metadata.annotations.meta\\.helm\\.sh/release-name}")

  if [[ -n "${namespace}" ]]; then
    args=(-n "${namespace}" "${args[@]}")
  fi

  release_name="$(kubectl get "${args[@]}" 2>/dev/null || true)"
  [[ -n "${release_name}" ]]
}

is_plugin_infra_managed() {
  local api_resource="$1"
  local name="$2"
  local namespace="${3:-}"
  local managed_by
  local resource="${api_resource}/${name}"

  if [[ -n "${namespace}" ]]; then
    managed_by="$(kubectl get -n "${namespace}" "${resource}" \
      -o jsonpath="{.metadata.labels.rhdh\.redhat\.com/managed-by}" 2>/dev/null || true)"
  else
    managed_by="$(kubectl get "${resource}" \
      -o jsonpath="{.metadata.labels.rhdh\.redhat\.com/managed-by}" 2>/dev/null || true)"
  fi

  [[ "${managed_by}" == "${PLUGIN_INFRA_LABEL_VALUE}" ]]
}

should_skip_resource_mutation() {
  local api_resource="$1"
  local name="$2"
  local namespace="${3:-}"

  if ! resource_exists "${api_resource}/${name}" ${namespace:+-n "${namespace}"}; then
    return 1
  fi

  if is_helm_managed "${api_resource}" "${name}" "${namespace}"; then
    echo "Skipping ${api_resource}/${name}: managed by Helm"
    return 0
  fi

  if is_plugin_infra_managed "${api_resource}" "${name}" "${namespace}"; then
    return 1
  fi

  echo "Skipping ${api_resource}/${name}: not managed by plugin-infra"
  return 0
}

detect_olm_v1_crd() {
  crd_exists "clusterextensions.olm.operatorframework.io"
}

detect_olm_v1_clustercatalog_crd() {
  crd_exists "clustercatalogs.olm.operatorframework.io"
}

detect_olm_v0_subscription_crd() {
  crd_exists "subscriptions.operators.coreos.com"
}

detect_olm_v0_operatorgroup_crd() {
  crd_exists "operatorgroups.operators.coreos.com"
}

has_v0_operators_installed() {
  if ! detect_olm_v0_subscription_crd; then
    return 1
  fi

  if resource_exists "subscription/${SERVERLESS_OPERATOR_NAME}" -n openshift-serverless 2>/dev/null; then
    return 0
  fi

  if resource_exists "subscription/${LOGIC_OPERATOR_NAME}" -n openshift-serverless-logic 2>/dev/null; then
    return 0
  fi

  return 1
}

redhat_operators_catalog_exists() {
  local catalogs err exit_code

  catalogs="$(kubectl get clustercatalog -l "${REDHAT_OPERATORS_CATALOG_LABEL}" -o name 2>&1)"
  exit_code=$?
  if [[ ${exit_code} -eq 0 && -n "${catalogs}" ]]; then
    return 0
  fi
  if [[ ${exit_code} -ne 0 ]]; then
    if grep -qiE '(not found|NotFound)' <<<"${catalogs}"; then
      :
    else
      echo "${catalogs}" >&2
      exit 1
    fi
  fi

  catalogs="$(kubectl get clustercatalog openshift-redhat-operators -o name 2>&1)"
  exit_code=$?
  if [[ ${exit_code} -eq 0 && -n "${catalogs}" ]]; then
    return 0
  fi
  if [[ ${exit_code} -ne 0 ]]; then
    if grep -qiE '(not found|NotFound)' <<<"${catalogs}"; then
      return 1
    fi
    echo "${catalogs}" >&2
    exit 1
  fi

  return 1
}

verify_redhat_operators_catalog() {
  if redhat_operators_catalog_exists; then
    return 0
  fi

  echo "OLM v1 requires a ClusterCatalog labeled ${REDHAT_OPERATORS_CATALOG_LABEL}."
  echo "On OpenShift this catalog is platform-provided; it is not created by plugin-infra.sh."
  exit 1
}

resolve_olm_version() {
  if [[ "${olm_version}" == "v0" ]]; then
    resolved_olm_version="v0"
    echo "Using OLM v0 (forced via --olm-version)"
    return
  fi

  if [[ "${olm_version}" == "v1" ]]; then
    if ! detect_olm_v1_crd; then
      echo "OLM v1 requested but ClusterExtension CRD not found on this cluster"
      exit 1
    fi
    if ! detect_olm_v1_clustercatalog_crd; then
      echo "OLM v1 requested but ClusterCatalog CRD not found on this cluster"
      exit 1
    fi
    verify_redhat_operators_catalog
    resolved_olm_version="v1"
    echo "Using OLM v1 (forced via --olm-version)"
    return
  fi

  if detect_olm_v1_crd && detect_olm_v1_clustercatalog_crd; then
    if redhat_operators_catalog_exists; then
      # Check if operators are already installed via v0
      if has_v0_operators_installed; then
        resolved_olm_version="v0"
        echo "Auto-detected OLM v0 (operators already installed via v0 Subscriptions)"
        echo "  Note: To migrate to OLM v1, first run 'plugin-infra.sh delete' to remove v0 installations"
      else
        resolved_olm_version="v1"
        echo "Auto-detected OLM v1 (ClusterExtension and ClusterCatalog CRDs found)"
      fi
    else
      resolved_olm_version="v0"
      echo "Auto-detected OLM v0 (openshift-redhat-operators ClusterCatalog not found)"
    fi
  else
    resolved_olm_version="v0"
    echo "Auto-detected OLM v0 (OLM v1 CRDs not found)"
  fi
}

apply_manifest() {
  local file="$1"
  local url="https://raw.githubusercontent.com/redhat-developer/rhdh-operator/${branch}/config/profile/rhdh/plugin-infra/${file}"
  local path="${script_dir}/${file}"

  if [[ -f "${path}" ]]; then
    echo "Using local file: ${file}"
    kubectl "$action" -f "${path}"
  else
    echo "Local file not found. Fetching from URL: ${url}"
    curl -s "$url" | kubectl "$action" -f -
  fi
}

apply_v1_operator_manifest() {
  local file="$1"
  local extension_name="$2"

  if should_skip_resource_mutation clusterextension "${extension_name}"; then
    return 0
  fi

  apply_manifest "${file}"
}

delete_plugin_infra_v0_operator() {
  local namespace="$1"
  local operatorgroup_name="$2"
  local subscription_name="$3"

  if detect_olm_v0_subscription_crd; then
    if ! should_skip_resource_mutation subscription "${subscription_name}" "${namespace}"; then
      kubectl delete "subscription/${subscription_name}" -n "${namespace}" --ignore-not-found
    fi
  fi

  if detect_olm_v0_operatorgroup_crd; then
    if ! should_skip_resource_mutation operatorgroup "${operatorgroup_name}" "${namespace}"; then
      kubectl delete "operatorgroup/${operatorgroup_name}" -n "${namespace}" --ignore-not-found
    fi
  fi

  if ! should_skip_resource_mutation namespace "${namespace}"; then
    kubectl delete "namespace/${namespace}" --ignore-not-found
  fi
}

wait_for_clusterextension() {
  local name="$1"
  echo "Waiting for ClusterExtension/${name} to be Installed..."
  kubectl wait "clusterextension/${name}" --for=condition=Installed --timeout=300s
}

wait_for_clusterextension_if_relevant() {
  local name="$1"

  if ! resource_exists "clusterextension/${name}"; then
    return 0
  fi

  if is_helm_managed clusterextension "${name}"; then
    echo "ClusterExtension/${name} is managed by Helm; skipping wait"
    return 0
  fi

  wait_for_clusterextension "${name}"
}

wait_for_clusterextension_deleted() {
  local name="$1"
  local timeout="${2:-300}"

  if ! detect_olm_v1_crd; then
    return 0
  fi

  if ! resource_exists "clusterextension/${name}"; then
    return 0
  fi

  echo "Waiting for ClusterExtension/${name} to be removed..."
  kubectl wait --for=delete "clusterextension/${name}" --timeout="${timeout}s"
}

delete_plugin_infra_v1_operator() {
  local extension_name="$1"
  local namespace="$2"
  local sa_name="$3"
  local crb_name="$4"

  if detect_olm_v1_crd; then
    if resource_exists "clusterextension/${extension_name}"; then
      if should_skip_resource_mutation clusterextension "${extension_name}"; then
        :
      else
        kubectl delete "clusterextension/${extension_name}" --ignore-not-found
        wait_for_clusterextension_deleted "${extension_name}"
      fi
    fi
  fi

  if ! should_skip_resource_mutation serviceaccount "${sa_name}" "${namespace}"; then
    kubectl delete "serviceaccount/${sa_name}" -n "${namespace}" --ignore-not-found
  fi

  if ! should_skip_resource_mutation clusterrolebinding "${crb_name}"; then
    kubectl delete "clusterrolebinding/${crb_name}" --ignore-not-found
  fi

  if ! should_skip_resource_mutation namespace "${namespace}"; then
    kubectl delete "namespace/${namespace}" --ignore-not-found
  fi
}

delete_orchestrator_v1_operators() {
  delete_plugin_infra_v1_operator \
    "${LOGIC_OPERATOR_NAME}" \
    "openshift-serverless-logic" \
    "serverless-logic-operator-installer" \
    "logic-operator-installer-binding"
  delete_plugin_infra_v1_operator \
    "${SERVERLESS_OPERATOR_NAME}" \
    "openshift-serverless" \
    "serverless-operator-installer" \
    "serverless-operator-installer-binding"
}

delete_orchestrator_v0_operators() {
  delete_plugin_infra_v0_operator \
    "openshift-serverless-logic" \
    "openshift-serverless-logic" \
    "${LOGIC_OPERATOR_NAME}"
  delete_plugin_infra_v0_operator \
    "openshift-serverless" \
    "serverless-operator-group" \
    "${SERVERLESS_OPERATOR_NAME}"
}

resolve_olm_version

if [[ "${resolved_olm_version}" == "v1" ]]; then
  serverless_manifest="serverless-v1.yaml"
  serverless_logic_manifest="serverless-logic-v1.yaml"
else
  serverless_manifest="serverless.yaml"
  serverless_logic_manifest="serverless-logic.yaml"
fi

# Execution
if [[ "$action" == "apply" ]]; then
  if [[ "${resolved_olm_version}" == "v1" ]]; then
    apply_v1_operator_manifest "${serverless_manifest}" "${SERVERLESS_OPERATOR_NAME}"
    wait_for_clusterextension_if_relevant "${SERVERLESS_OPERATOR_NAME}"
  else
    apply_manifest "${serverless_manifest}"
  fi

  echo "Waiting for CRDs to be established..."
  kubectl wait --for=condition=Established crd --all --timeout=60s
  apply_manifest "knative.yaml"

  if [[ "${resolved_olm_version}" == "v1" ]]; then
    apply_v1_operator_manifest "${serverless_logic_manifest}" "${LOGIC_OPERATOR_NAME}"
    wait_for_clusterextension_if_relevant "${LOGIC_OPERATOR_NAME}"
  else
    apply_manifest "${serverless_logic_manifest}"
  fi

  if [[ "$cicd" == true ]]; then
    echo "CICD enabled. Executing CICD-specific logic..."
    apply_manifest "argocd.yaml"
    kubectl wait --for=condition=Established crd --all --timeout=60s
    apply_manifest "argocd-cr.yaml"
    apply_manifest "pipeline.yaml"
  fi
elif [[ "$action" == "delete" ]]; then
  delete_orchestrator_v1_operators
  apply_manifest "knative.yaml"
  delete_orchestrator_v0_operators
  if [[ "$cicd" == true ]]; then
    apply_manifest "argocd.yaml"
    apply_manifest "argocd-cr.yaml"
    apply_manifest "pipeline.yaml"
  fi
else
  echo "Action '$action' is not supported. Use 'apply' (default) or 'delete'."
  exit 1
fi
