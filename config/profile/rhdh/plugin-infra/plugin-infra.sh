#!/bin/bash
#
# Plugin Infrastructure Setup Script for RHDH with Orchestrator
#

set -e

action="apply" # Default action
branch="main"  # Default branch
cicd=false   # Default CICD mode

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
    *)
      echo "Unknown option: $1"
      exit 1
      ;;
  esac
done

apply_manifest() {
  local file="$1"
  local url="https://raw.githubusercontent.com/redhat-developer/rhdh-operator/${branch}/config/profile/rhdh/plugin-infra/${file}"
  local script_dir
  script_dir="$(dirname "$(realpath "$0")")"

  if [ -f "${script_dir}/${file}" ]; then
    echo "Using local file: ${file}"
    kubectl "$action" -f "${script_dir}/${file}"
  else
    echo "Local file not found. Fetching from URL: ${url}"
    curl -s "$url" | kubectl "$action" -f -
  fi
}

# Execution
if [ "$action" == "apply" ]; then
  apply_manifest "serverless.yaml"
  echo "Waiting for CRDs to be established..."
  kubectl wait --for=condition=Established crd --all --timeout=60s
  apply_manifest "knative.yaml"
  apply_manifest "serverless-logic.yaml"
  if [ "$cicd" == true ]; then
    echo "CICD enabled. Executing CICD-specific logic..."
    apply_manifest "argocd.yaml"
    kubectl wait --for=condition=Established crd --all --timeout=60s
    apply_manifest "argocd-cr.yaml"
    apply_manifest "pipeline.yaml"
  fi
elif [ "$action" == "delete" ]; then
  apply_manifest "serverless-logic.yaml"
  apply_manifest "knative.yaml"
  apply_manifest "serverless.yaml"
  if [ "$cicd" == true ]; then
    apply_manifest "argocd.yaml"
    apply_manifest "argocd-cr.yaml"
    apply_manifest "pipeline.yaml"
  fi
else
  echo "Action '$action' is not supported. Use 'apply' (default) or 'delete'."
  exit 1
fi

