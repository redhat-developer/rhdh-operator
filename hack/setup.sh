#!/usr/bin/env bash
#
# Setup Script for RHDH Authentication For Orchestrator CICD
#

function captureRHDHNamespace {
  default="rhdh"
  if [ "$use_default" == true ]; then
    rhdh_workspace="$default"
  else
    read -rp "Enter RHDH Instance namespace (default: $default): " value
    if [ -z "$value" ]; then
        rhdh_workspace="$default"
    else
        rhdh_workspace="$value"
    fi
  fi
  RHDH_NAMESPACE=$rhdh_workspace
}

function captureArgoCDNamespace {
  default="openshift-gitops-operator"
  if [ "$use_default" == true ]; then
    argocd_namespace="$default"
  else
    read -rp "Enter ArgoCD installation namespace (default: $default): " value

    if [ -z "$value" ]; then
        argocd_namespace="$default"
    else
        argocd_namespace="$value"
    fi
  fi
  ARGOCD_NAMESPACE=$argocd_namespace
}

function captureArgoCDURL {
  argocd_instances=$(oc get argocd -n "$ARGOCD_NAMESPACE" -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}')

  if [ -z "$argocd_instances" ]; then
      echo "No ArgoCD instances found in namespace $ARGOCD_NAMESPACE. Continuing without ArgoCD support"
  else
    if [ "$use_default" == true ]; then
          selected_instance=$(echo "$argocd_instances" | awk 'NR==1')
          echo "Select an ArgoCD instance: $selected_instance"
    else
      echo "Select an ArgoCD instance:"
      select instance in $argocd_instances; do
          if [ -n "$instance" ]; then
              selected_instance="$instance"
              break
          else
              echo "Invalid selection. Please choose a valid option."
          fi
      done
    fi
    argocd_route=$(oc get route -n "$ARGOCD_NAMESPACE" -l app.kubernetes.io/managed-by="$selected_instance" -ojsonpath='{.items[0].status.ingress[0].host}')
    echo "Found Route at $argocd_route"
    ARGOCD_URL=https://$argocd_route
  fi

}

function captureArgoCDCreds {
  if [ -n "$selected_instance" ]; then
    admin_password=$(oc get secret -n "$ARGOCD_NAMESPACE" "${selected_instance}"-cluster -ojsonpath='{.data.admin\.password}' | base64 -d)
    ARGOCD_USERNAME="admin"
    ARGOCD_PASSWORD=$admin_password
  fi
}

function checkPrerequisite {
  if ! command -v oc &> /dev/null; then
    echo "oc is required for this script to run. Exiting."
    exit 1
  fi
}

function createBackstageSecret {
  if 2>/dev/null 1>&2 oc get secret backstage-backend-auth-secret -n "$RHDH_NAMESPACE"; then
    oc delete secret backstage-backend-auth-secret -n "$RHDH_NAMESPACE"
  fi
  declare -A secretKeys
  if [ -n "$ARGOCD_USERNAME" ]; then
    secretKeys[ARGOCD_USERNAME]=$ARGOCD_USERNAME
  fi
  if [ -n "$ARGOCD_URL" ]; then
    secretKeys[ARGOCD_URL]=$ARGOCD_URL
  fi
  if [ -n "$ARGOCD_PASSWORD" ]; then
    secretKeys[ARGOCD_PASSWORD]=$ARGOCD_PASSWORD
  fi
  cmd="oc create secret generic backstage-backend-auth-secret -n $RHDH_NAMESPACE"
  for key in "${!secretKeys[@]}"; do
    cmd="${cmd} --from-literal=${key}=${secretKeys[$key]}"
  done
  eval "$cmd"
}

# Function to display usage instructions
display_usage() {
    echo "Usage: $0 [OPTIONS]"
    echo "Options:"
    echo "  -h, --help        Display usage instructions"
    echo "  --use-default     Specify to use all default values"
    exit 1
}

# Initialize variable
use_default=false

# Parse command-line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--help)
            display_usage
            ;;
        --use-default)
            use_default=true
            ;;
        *)
            echo "Error: Invalid option $1"
            display_usage
            ;;
    esac
    shift
done

function main {

  # Check if using default values or not
  if $use_default; then
      echo "Using default values."
  else
      echo "Not using default values."
  fi

  checkPrerequisite
  captureRHDHNamespace
  captureArgoCDNamespace
  captureArgoCDURL
  captureArgoCDCreds
  createBackstageSecret

  echo "Setup completed successfully!"
}

main