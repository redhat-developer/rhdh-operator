#!/bin/bash
#
# Plugin Infrastructure Setup Script for RHDH with Orchestrator
#

set -euo pipefail

action="apply" # Default action
branch="main"  # Default branch
cicd=false   # Default CICD mode
olm_version="auto" # auto | v0 | v1

# Parse command-line options
while [[ $# -gt 0 ]]; do
  case "$1" in
    --with-cicd)
      cicd=true
      shift
      ;;
    apply|delete)
      action="$1"
      shift
      ;;
    --branch)
      branch="$2"
      shift 2
      ;;
    --olm-version)
      olm_version="$2"
      shift 2
      ;;
    *)
      echo "Unknown option: $1"
      exit 1
      ;;
  esac
done

script_dir="$(dirname "$(realpath "$0")")"

detect_olm_v1_crd() {
  kubectl get crd clusterextensions.olm.operatorframework.io &>/dev/null
}

detect_olm_v1_clustercatalog_crd() {
  kubectl get crd clustercatalogs.olm.operatorframework.io &>/dev/null
}

resolve_olm_version() {
  case "${olm_version}" in
    v0)
      RESOLVED_OLM_VERSION="v0"
      echo "Using OLM v0 manifests (forced via --olm-version v0)"
      ;;
    v1)
      if ! detect_olm_v1_crd || ! detect_olm_v1_clustercatalog_crd; then
        echo "Error: OLM v1 requested but ClusterExtension or ClusterCatalog CRD not found on this cluster"
        exit 1
      fi
      RESOLVED_OLM_VERSION="v1"
      echo "Using OLM v1 manifests (forced via --olm-version v1)"
      ;;
    auto)
      if detect_olm_v1_crd && detect_olm_v1_clustercatalog_crd; then
        RESOLVED_OLM_VERSION="v1"
        echo "Auto-detected OLM v1 (ClusterExtension and ClusterCatalog CRDs found)"
      else
        RESOLVED_OLM_VERSION="v0"
        echo "Auto-detected OLM v0 (OLM v1 CRDs not found)"
      fi
      ;;
    *)
      echo "Error: Unknown --olm-version '${olm_version}'. Use auto, v0, or v1."
      exit 1
      ;;
  esac
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

apply_operator_manifest() {
  local v0_file="$1"
  if [[ "${RESOLVED_OLM_VERSION}" == "v1" ]]; then
    apply_manifest "olm-v1/${v0_file}"
  else
    apply_manifest "${v0_file}"
  fi
}

wait_clusterextension_installed() {
  local name="$1"
  local timeout="${2:-600s}"
  echo "Waiting for ClusterExtension/${name} Installed (timeout ${timeout})..."
  kubectl wait "clusterextension/${name}" --for=condition=Installed --timeout="${timeout}"
}

configure_gitops_operator_env() {
  if [[ "${RESOLVED_OLM_VERSION}" != "v1" ]]; then
    return 0
  fi

  local ns="openshift-gitops-operator"
  echo "Applying GitOps operator environment workaround for OLM v1..."
  local deployment=""
  local attempt
  for attempt in $(seq 1 30); do
    for candidate in \
      openshift-gitops-operator-controller-manager \
      openshift-gitops-operator-controller-manager-v5 \
      gitops-operator-controller-manager; do
      if kubectl get deployment "${candidate}" -n "${ns}" &>/dev/null; then
        deployment="${candidate}"
        break 2
      fi
    done
    deployment="$(kubectl get deployment -n "${ns}" -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)"
    [[ -n "${deployment}" ]] && break
    sleep 10
  done

  if [[ -z "${deployment}" ]]; then
    echo "Warning: GitOps operator deployment not found in ${ns}; skipping env configuration"
    return 0
  fi

  kubectl set env "deployment/${deployment}" -n "${ns}" \
    DISABLE_DEFAULT_ARGOCD_INSTANCE=true \
    ARGOCD_CLUSTER_CONFIG_NAMESPACES="${ns}"
}

resolve_olm_version

# Execution
if [[ "$action" == "apply" ]]; then
  if [[ "${RESOLVED_OLM_VERSION}" == "v1" ]]; then
    apply_manifest "olm-v1/installer-clusterrole.yaml"
  fi

  apply_operator_manifest "serverless.yaml"
  if [[ "${RESOLVED_OLM_VERSION}" == "v1" ]]; then
    wait_clusterextension_installed "serverless-operator"
  fi

  echo "Waiting for CRDs to be established..."
  kubectl wait --for=condition=Established crd --all --timeout=60s
  apply_manifest "knative.yaml"
  apply_operator_manifest "serverless-logic.yaml"
  if [[ "${RESOLVED_OLM_VERSION}" == "v1" ]]; then
    wait_clusterextension_installed "logic-operator"
  fi

  if [[ "$cicd" == true ]]; then
    echo "CICD enabled. Executing CICD-specific logic..."
    apply_operator_manifest "argocd.yaml"
    if [[ "${RESOLVED_OLM_VERSION}" == "v1" ]]; then
      wait_clusterextension_installed "openshift-gitops-operator"
      configure_gitops_operator_env
    fi
    echo "Waiting for CRDs to be established..."
    kubectl wait --for=condition=Established crd --all --timeout=60s
    apply_manifest "argocd-cr.yaml"
    apply_operator_manifest "pipeline.yaml"
    if [[ "${RESOLVED_OLM_VERSION}" == "v1" ]]; then
      wait_clusterextension_installed "openshift-pipelines-operator"
    fi
  fi
elif [[ "$action" == "delete" ]]; then
  if [[ "$cicd" == true ]]; then
    apply_operator_manifest "pipeline.yaml"
    apply_manifest "argocd-cr.yaml"
    apply_operator_manifest "argocd.yaml"
  fi
  apply_operator_manifest "serverless-logic.yaml"
  apply_manifest "knative.yaml"
  apply_operator_manifest "serverless.yaml"
  if [[ "${RESOLVED_OLM_VERSION}" == "v1" ]]; then
    apply_manifest "olm-v1/installer-clusterrole.yaml"
  fi
else
  echo "Action '$action' is not supported. Use 'apply' (default) or 'delete'."
  exit 1
fi
