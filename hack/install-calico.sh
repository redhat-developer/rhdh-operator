#!/usr/bin/env bash
#
# Install Calico on a KinD (or any Kubernetes) cluster so that NetworkPolicies are
# actually enforced.
#
# KinD's default CNI (kindnet) does not implement NetworkPolicy enforcement, which
# means the NetworkPolicies shipped by the RHDH Operator are silently ignored and
# tests can pass even when the policies are wrong or incomplete.
#
# The KinD cluster must be created with:
#
#   networking:
#     disableDefaultCNI: true
#     podSubnet: "192.168.0.0/16"   # matches Calico's default IPv4 pool
#
# See hack/kind-config.yaml for a ready-to-use configuration.
#
# Usage:
#   ./hack/install-calico.sh
#   CALICO_VERSION=v3.31.7 ./hack/install-calico.sh
#
set -euo pipefail

# The workflows in this repo create KinD clusters with kind v0.26 (Kubernetes 1.32),
# so pin Calico to a release tested against that Kubernetes version.
CALICO_VERSION="${CALICO_VERSION:-v3.31.7}"
WAIT_TIMEOUT="${WAIT_TIMEOUT:-300s}"
MANIFEST_URL="https://raw.githubusercontent.com/projectcalico/calico/${CALICO_VERSION}/manifests/calico.yaml"

log() {
  echo "[install-calico] $*"
}

if kubectl get daemonset -n kube-system kindnet >/dev/null 2>&1; then
  echo "[install-calico] ERROR: kindnet is still installed in this cluster." >&2
  echo "[install-calico] Recreate the cluster with networking.disableDefaultCNI=true (see hack/kind-config.yaml)." >&2
  exit 1
fi

log "Installing Calico ${CALICO_VERSION} (NetworkPolicy enforcement)..."

manifest="$(mktemp)"
trap 'rm -f "${manifest}"' EXIT

curl -fsSL --retry 5 --retry-all-errors --retry-delay 2 "${MANIFEST_URL}" -o "${manifest}"

# Calico's manifest is large; server-side apply avoids the client-side
# "metadata.annotations: Too long" limitation.
kubectl apply --server-side --force-conflicts -f "${manifest}"

log "Waiting for calico-node DaemonSet to roll out..."
kubectl -n kube-system rollout status daemonset/calico-node --timeout="${WAIT_TIMEOUT}"

log "Waiting for calico-kube-controllers Deployment to roll out..."
kubectl -n kube-system rollout status deployment/calico-kube-controllers --timeout="${WAIT_TIMEOUT}"

log "Waiting for all nodes to become Ready..."
kubectl wait --for=condition=Ready nodes --all --timeout="${WAIT_TIMEOUT}"

log "Calico is ready; NetworkPolicies are now enforced."
