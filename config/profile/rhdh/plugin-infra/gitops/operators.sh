#!/bin/bash
#
# GitOps/Pipeline Infrastructure Setup Script
#

set -e

action="${1:-apply}" # Default action is 'apply'


gitops_pipelines() {
  kubectl "$action" -f - <<EOF
apiVersion: v1
kind: Namespace
metadata:
  name: orchestrator-gitops
---
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: orchestrator-gitops-group
  namespace: orchestrator-gitops
---
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: orchestrator-gitops-operator
  namespace: orchestrator-gitops
spec:
  config:
    env:
    - name: DISABLE_DEFAULT_ARGOCD_INSTANCE
      value: "true"
    - name: ARGOCD_CLUSTER_CONFIG_NAMESPACES
      value: "orchestrator-gitops"
  channel: latest
  installPlanApproval: Automatic
  name: openshift-gitops-operator
  source: redhat-operators
  sourceNamespace: openshift-marketplace
---
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: openshift-pipelines-operator-rh
  namespace: orchestrator-gitops
spec:
  channel: pipelines-1.17
  installPlanApproval: Automatic
  name: openshift-pipelines-operator-rh
  source: redhat-operators
  sourceNamespace: openshift-marketplace
EOF
}

# execution

if [ "$action" == "apply" ]; then
  gitops_pipelines
elif [ "$action" == "delete" ]; then
  gitops_pipelines
else
  echo "Action '$action' is not supported. Use 'apply' (default) or 'delete'."
  exit 1
fi