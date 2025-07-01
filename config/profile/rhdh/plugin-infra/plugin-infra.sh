#!/bin/bash
#
# Plugin Infrastructure Setup Script for RHDH with Orchestrator
#

action="${1:-apply}" # Default action is 'apply'

serverless() {
  kubectl $action -f - <<EOF
apiVersion: v1
kind: Namespace
metadata:
  name: openshift-serverless
---
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: serverless-operator-group
  namespace: openshift-serverless
spec:
---
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: serverless-operator
  namespace: openshift-serverless
spec:
  channel: stable
  installPlanApproval: Automatic
  name: serverless-operator
  source: redhat-operators
  sourceNamespace: openshift-marketplace
EOF
}

knative() {
  kubectl $action -f - <<EOF
apiVersion: v1
kind: Namespace
metadata:
  name: knative-serving
---
apiVersion: v1
kind: Namespace
metadata:
  name: knative-eventing
EOF

  kubectl $action -f - <<EOF
apiVersion: operator.knative.dev/v1beta1
kind: KnativeEventing
metadata:
  name: knative-eventing
  namespace: knative-eventing
spec:
  Registry: {}
---
apiVersion: operator.knative.dev/v1beta1
kind: KnativeServing
metadata:
  name: knative-serving
  namespace: knative-serving
spec:
  controller-custom-certs:
    name: ""
    type: ""
  registry: {}
EOF
}

serverless_logic() {
  kubectl $action -f - <<EOF
apiVersion: v1
kind: Namespace
metadata:
  name: openshift-serverless-logic
---
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: openshift-serverless-logic
  namespace: openshift-serverless-logic
spec:
---
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: logic-operator-rhel8
  namespace: openshift-serverless-logic
spec:
  channel: alpha  #  channel of an operator package to subscribe to
  installPlanApproval: Automatic #  whether the update should be installed automatically
  name: logic-operator-rhel8  #  name of the operator package
  source: redhat-operators  #  name of the catalog source
  sourceNamespace: openshift-marketplace
  startingCSV: logic-operator-rhel8.v1.36.0  # The initial version of the operator
EOF
}

# execution

if [ "$action" == "apply" ]; then
  serverless
  echo "Waiting for CRDs to be established..."
  kubectl wait --for=condition=Established crd --all --timeout=60s
  knative
  serverless_logic
elif [ "$action" == "delete" ]; then
  serverless_logic
  knative
  serverless
else
  echo "Action '$action' is not supported. Use 'apply' (default) or 'delete'."
fi
