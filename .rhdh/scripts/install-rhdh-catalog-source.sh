#!/bin/bash
#
# Script to streamline installing an IIB image in an OpenShift or Kubernetes cluster for testing.
#
# Requires: oc (OCP) or kubectl (K8s), jq, yq, umoci, base64, opm, skopeo

set -euo pipefail

NC='\033[0m'

IS_OPENSHIFT=""

NAMESPACE_SUBSCRIPTION="rhdh-operator"
OLM_CHANNEL="fast"
UPSTREAM_IIB_OVERRIDE=""
INSTALL_PLAN_APPROVAL="Automatic"

function logf() {
  set -euo pipefail

  local prefix=$1
  local color=$2
  local msg=$3
  local fullMsg="[${prefix}] ${msg}"

  if [[ "$TERM" == *"color"* ]]; then
    echo -e "${color}${fullMsg}${NC}"
  else
    echo -e "${fullMsg}"
  fi
}

function infof() {
  logf "INFO" "${NC}" "$1"
}

function warnf() {
  logf "WARN" "\033[0;33m" "$1"
}

function debugf() {
  logf "DEBUG" "\033[0;90m" "$1"
}

function errorf() {
  logf "ERROR" "\033[0;31m" "$1"
}

function usage() {
  echo "
This script streamlines testing IIB images by configuring an OpenShift or Kubernetes cluster to enable it to use the specified IIB image
as a catalog source. The CatalogSource is created in the 'openshift-marketplace' namespace on OpenShift or 'olm' namespace on Kubernetes,
and is named 'operatorName-channelName', eg., rhdh-fast

If IIB installation fails, see https://docs.engineering.redhat.com/display/CFC/Test and
follow steps in section 'Adding Brew Pull Secret'

Usage:
  $0 [OPTIONS]

Options:
  -v 1.y                              : Install from iib quay.io/rhdh/iib:1.y-\$OCP_VER-\$OCP_ARCH (eg., 1.4-v4.14-x86_64)
  --latest                            : Install from iib quay.io/rhdh/iib:latest-\$OCP_VER-\$OCP_ARCH (eg., latest-v4.14-x86_64) [default]
  --next                              : Install from iib quay.io/rhdh/iib:next-\$OCP_VER-\$OCP_ARCH (eg., next-v4.14-x86_64)
  --catalog-source                    : Install from specified catalog source, like brew.registry.redhat.io/rh-osbs/iib-pub-pending:v4.18
  --install-operator <NAME>           : Install operator named \$NAME after creating CatalogSource
  --install-plan-approval <STRATEGY>  : Specify the install plan strategy for the subscription (default: Automatic)

Examples:
  $0 \\
    --install-operator rhdh          # RC release in progress (from latest tag and stable branch )

  $0 \\
    --next --install-operator rhdh   # CI future release (from next tag and upstream main branch)
"
}

function is_openshift() {
  set -euo pipefail

  oc get routes.route.openshift.io &> /dev/null || kubectl get routes.route.openshift.io &> /dev/null
}

function detect_ocp_and_set_env_var() {
  set -euo pipefail

  if [[ "${IS_OPENSHIFT}" = "" ]]; then
    IS_OPENSHIFT=$(is_openshift && echo 'true' || echo 'false')
  fi
}

# Wrapper function to call kubectl or oc
function invoke_cluster_cli() {
  set -euo pipefail

  local command=$1
  shift

  detect_ocp_and_set_env_var
  if [[ "${IS_OPENSHIFT}" = "true" ]]; then
    if command -v oc &> /dev/null; then
      oc "$command" "$@"
    else
      kubectl "$command" "$@"
    fi
  else
    kubectl "$command" "$@"
  fi
}

function ocp_install_regular_cluster() {
  set -euo pipefail

  # A regular cluster should support ImageContentSourcePolicy/ImageDigestMirrorSet resources
  ICSP_URL="quay.io/rhdh/"
  ICSP_URL_PRE=${ICSP_URL%%/*}

  # for 1.4+, use IDMS instead of ICSP
  # TODO https://issues.redhat.com/browse/RHIDP-4188 if we onboard 1.3 to Konflux, use IDMS for latest too
  if [[ "$IIB_IMAGE" == *"next"* ]]; then
    debugf "Adding ImageDigestMirrorSet to resolve unreleased images on registry.redhat.io from quay.io" >&2
    echo "---
apiVersion: config.openshift.io/v1
kind: ImageDigestMirrorSet
metadata:
  name: ${ICSP_URL_PRE//./-}
spec:
  imageDigestMirrors:
  - source: registry.redhat.io/rhdh/rhdh-hub-rhel9
    mirrors:
      - ${ICSP_URL}rhdh-hub-rhel9
  - source: registry.redhat.io/rhdh/rhdh-rhel9-operator
    mirrors:
      - ${ICSP_URL}rhdh-rhel9-operator
  - source: registry-proxy.engineering.redhat.com/rh-osbs/rhdh-rhdh-operator-bundle
    mirrors:
      - ${ICSP_URL}rhdh-operator-bundle
  " > "$TMPDIR/ImageDigestMirrorSet_${ICSP_URL_PRE}.yml" && oc apply -f "$TMPDIR/ImageDigestMirrorSet_${ICSP_URL_PRE}.yml" >&2
  else
    debugf "Adding ImageContentSourcePolicy to resolve references to images not on quay.io as if from quay.io" >&2
    # debugf "${ICSP_URL_PRE}, ${ICSP_URL_PRE//./-}, ${ICSP_URL}"
    echo "---
apiVersion: operator.openshift.io/v1alpha1
kind: ImageContentSourcePolicy
metadata:
  name: ${ICSP_URL_PRE//./-}
spec:
  repositoryDigestMirrors:
  ## 1. add mappings for Developer Hub bundle, operator, hub
  - mirrors:
    - ${ICSP_URL}rhdh-operator-bundle
    source: registry.redhat.io/rhdh/rhdh-operator-bundle
  - mirrors:
    - ${ICSP_URL}rhdh-operator-bundle
    source: registry.stage.redhat.io/rhdh/rhdh-operator-bundle
  - mirrors:
    - ${ICSP_URL}rhdh-operator-bundle
    source: registry-proxy.engineering.redhat.com/rh-osbs/rhdh-rhdh-operator-bundle

  - mirrors:
    - ${ICSP_URL}rhdh-rhel9-operator
    source: registry.redhat.io/rhdh/rhdh-rhel9-operator
  - mirrors:
    - ${ICSP_URL}rhdh-rhel9-operator
    source: registry.stage.redhat.io/rhdh/rhdh-rhel9-operator
  - mirrors:
    - ${ICSP_URL}rhdh-rhel9-operator
    source: registry-proxy.engineering.redhat.com/rh-osbs/rhdh-rhdh-rhel9-operator

  - mirrors:
    - ${ICSP_URL}rhdh-hub-rhel9
    source: registry.redhat.io/rhdh/rhdh-hub-rhel9
  - mirrors:
    - ${ICSP_URL}rhdh-hub-rhel9
    source: registry.stage.redhat.io/rhdh/rhdh-hub-rhel9
  - mirrors:
    - ${ICSP_URL}rhdh-hub-rhel9
    source: registry-proxy.engineering.redhat.com/rh-osbs/rhdh-rhdh-hub-rhel9

  ## 2. general repo mappings
  - mirrors:
    - ${ICSP_URL_PRE}
    source: registry.redhat.io
  - mirrors:
    - ${ICSP_URL_PRE}
    source: registry.stage.redhat.io
  - mirrors:
    - ${ICSP_URL_PRE}
    source: registry-proxy.engineering.redhat.com

  ### now add mappings to resolve internal references
  - mirrors:
    - registry.redhat.io
    source: registry.stage.redhat.io
  - mirrors:
    - registry.stage.redhat.io
    source: registry-proxy.engineering.redhat.com
  - mirrors:
    - registry.redhat.io
    source: registry-proxy.engineering.redhat.com
  " > "$TMPDIR/ImageContentSourcePolicy_${ICSP_URL_PRE}.yml" && oc apply -f "$TMPDIR/ImageContentSourcePolicy_${ICSP_URL_PRE}.yml" >&2
  fi

  printf "%s" "${IIB_IMAGE}"
}

function render_iib() {
  set -euo pipefail

  mkdir -p "${TMPDIR}/rhdh/rhdh"
  debugf "Rendering IIB $UPSTREAM_IIB as a local file..."
  opm render "$UPSTREAM_IIB" --output=yaml > "${TMPDIR}/rhdh/rhdh/render.yaml"
  if [ ! -s "${TMPDIR}/rhdh/rhdh/render.yaml" ]; then
    errorf "
[ERROR] 'opm render $UPSTREAM_IIB' returned an empty output, which likely means that this IIB Image does not contain any operators in it.
Please reach out to the RHDH Productization team.
"
    return 1
  fi
}

function k8s_check_bundle_manifest_default_config() {
  set -euo pipefail

  if [[ "${IS_OPENSHIFT}" = "true" ]]; then
    debugf "Skipping k8s_update_bundle_manifest_default_config handling on OCP cluster" >&2
    return 0
  fi

  local file="$1"
  if ! yq --exit-status 'tag == "!!map"' "$file" &>/dev/null; then
    # https://mikefarah.gitbook.io/yq/usage/tips-and-tricks#validating-yaml-files
    debugf "Skipping $file for k8s_update_bundle_manifest_default_config (for K8s compatibility): not a valid YAML file" >&2
    return 0
  fi
  if [[ $(yq '.kind' "$file") != "ConfigMap" ]]; then
    debugf "Skipping $file k8s_update_bundle_manifest_default_config (for K8s compatibility): not a ConfigMap manifest" >&2
    return 0
  fi

  if [[ ! "$(yq '.metadata.name' "$file")" =~ -default-config$ ]]; then
    debugf "Skipping $file for k8s_update_bundle_manifest_default_config (for K8s compatibility): not the operator default config ConfigMap" >&2
    return 0
  fi

  echo "ok"
}

function update_refs_in_iib_bundles() {
  set -euo pipefail

  local internal_registry_url="$1"
  local my_registry="$2"
  # 2. Render the IIB locally, modify any references to the internal registries with their mirrors on Quay
  # and push the updates to the internal cluster registry
  for bundleImg in $(grep -E '^image: .*operator-bundle' "${TMPDIR}/rhdh/rhdh/render.yaml" | awk '{print $2}' | uniq); do
    originalBundleImg="$bundleImg"
    digest="${originalBundleImg##*@sha256:}"
    bundleImg="${bundleImg/registry.stage.redhat.io/quay.io}"
    bundleImg="${bundleImg/registry.redhat.io/quay.io}"
    bundleImg="${bundleImg/registry-proxy.engineering.redhat.com\/rh-osbs\/rhdh-/quay.io\/rhdh\/}"
    debugf "$originalBundleImg => $bundleImg"
    if skopeo inspect "docker://$bundleImg" &> /dev/null; then
      newBundleImage="${my_registry}/rhdh/rhdh-operator-bundle:${digest}"
      newBundleImageAsInt="${internal_registry_url}/rhdh/rhdh-operator-bundle:${digest}"
      mkdir -p "bundles/$digest"

      debugf "Copying and unpacking image $bundleImg locally..."
      skopeo copy "docker://$bundleImg" "oci:./bundles/${digest}/src:latest"
      umoci unpack --image "./bundles/${digest}/src:latest" "./bundles/${digest}/unpacked" --rootless

      # Replace the occurrences in the .csv.yaml or .clusterserviceversion.yaml files
      debugf "Replacing refs to internal registry in bundle image $bundleImg..."
      for folder in manifests metadata; do
        for file in "./bundles/${digest}/unpacked/rootfs/${folder}"/*; do
          if [ -f "$file" ]; then
            debugf "replacing refs to internal registries in file '${file}'"
            sed -i 's#registry.redhat.io/rhdh#quay.io/rhdh#g' "$file"
            sed -i 's#registry.stage.redhat.io/rhdh#quay.io/rhdh#g' "$file"
            sed -i 's#registry-proxy.engineering.redhat.com/rh-osbs/rhdh-#quay.io/rhdh/#g' "$file"
          fi
        done
      done

      # repack the image with the changes
      debugf "Repacking image ./bundles/${digest}/src => ./bundles/${digest}/unpacked..."
      umoci repack --image "./bundles/${digest}/src:latest" "./bundles/${digest}/unpacked"

      # Push the bundle to the internal cluster registry
      debugf "Pushing updated image: ./bundles/${digest}/src => ${newBundleImage}..."
      skopeo copy --dest-tls-verify=false "oci:./bundles/${digest}/src:latest" "docker://${newBundleImage}"

      sed -i "s#${originalBundleImg}#${newBundleImageAsInt}#g" "${TMPDIR}/rhdh/rhdh/render.yaml"
    fi
  done

  # 3. Regenerate the IIB image with the local changes to the render.yaml file and build and push it from within the cluster
  debugf "Regenerating IIB Dockerfile with updated refs..."
  opm generate dockerfile rhdh/rhdh
}

function ocp_install_hosted_control_plane_cluster() {
  # Clusters with an hosted control plane do not propagate ImageContentSourcePolicy/ImageDigestMirrorSet resources
  # to the underlying nodes, causing an issue mirroring internal images effectively.
  # This function works around this by locally modifying the bundles (replacing all refs to the internal registries
  # with their mirrors on quay.io), rebuilding and pushing the images to the internal cluster registry.
  set -euo pipefail

  render_iib >&2

  # 1. Expose the internal cluster registry if not done already
  debugf "Exposing cluster registry..." >&2
  internal_registry_url="image-registry.openshift-image-registry.svc:5000"
  oc patch configs.imageregistry.operator.openshift.io/cluster --patch '{"spec":{"defaultRoute":true}}' --type=merge >&2
  my_registry=$(oc get route default-route -n openshift-image-registry --template='{{ .spec.host }}')
  skopeo login -u kubeadmin -p "$(oc whoami -t)" --tls-verify=false "$my_registry" >&2
  if oc -n openshift-marketplace get secret internal-reg-auth-for-rhdh &> /dev/null; then
    oc -n openshift-marketplace delete secret internal-reg-auth-for-rhdh >&2
  fi
  if oc -n openshift-marketplace get secret internal-reg-ext-auth-for-rhdh &> /dev/null; then
    oc -n openshift-marketplace delete secret internal-reg-ext-auth-for-rhdh >&2
  fi
  oc -n openshift-marketplace create secret docker-registry internal-reg-ext-auth-for-rhdh \
    --docker-server="${my_registry}" \
    --docker-username=kubeadmin \
    --docker-password="$(oc whoami -t)" \
    --docker-email="admin@internal-registry-ext.example.com" >&2
  oc -n openshift-marketplace create secret docker-registry internal-reg-auth-for-rhdh \
    --docker-server="${internal_registry_url}" \
    --docker-username=kubeadmin \
    --docker-password="$(oc whoami -t)" \
    --docker-email="admin@internal-registry.example.com" >&2
  for ns in rhdh-operator rhdh; do
    # To be able to push images under this scope in the internal image registry
    if ! oc get namespace "$ns" > /dev/null; then
      oc create namespace "$ns" >&2
    fi
    oc adm policy add-cluster-role-to-user system:image-signer system:serviceaccount:${ns}:default >&2 || true
  done
  oc policy add-role-to-user system:image-puller system:serviceaccount:openshift-marketplace:default -n openshift-marketplace >&2 || true
  oc policy add-role-to-user system:image-puller system:serviceaccount:rhdh-operator:default -n rhdh-operator >&2 || true

  # 3. Regenerate the IIB image with the local changes to the render.yaml file and build and push it from within the cluster
  update_refs_in_iib_bundles "$internal_registry_url" "$my_registry" >&2

  debugf "Submitting in-cluster build request for the updated IIB..." >&2
  if ! oc -n rhdh get buildconfig.build.openshift.io/iib >& /dev/null; then
    oc -n rhdh new-build --strategy docker --binary --name iib >&2
  fi
  oc -n rhdh patch buildconfig.build.openshift.io/iib -p '{"spec": {"strategy": {"dockerStrategy": {"dockerfilePath": "rhdh.Dockerfile"}}}}' >&2
  oc -n rhdh start-build iib --wait --follow --from-dir=rhdh >&2
  local imageStreamWithTag="rhdh/iib:${IIB_TAG}"
  oc tag rhdh/iib:latest "${imageStreamWithTag}" >&2

  local result="${internal_registry_url}/${imageStreamWithTag}"
  debugf "IIB built and pushed to internal cluster registry: $result..." >&2
  printf "%s" "${result}"
}

function k8s_install() {
  set -euo pipefail

  local namespace="rhdh-operator"
  local image="registry:2"
  local registry_name="local-registry"
  local username="registryuser"
  local password
  password=$(echo "$RANDOM" | base64 | head -c 20)

  render_iib >&2

  if ! invoke_cluster_cli get namespace "${namespace}" &> /dev/null; then
    invoke_cluster_cli create namespace "${namespace}" >&2
  fi

  # We cannot use ICSP/IDMS resources here => deploy a registry where the updated IIBs will be pushed to
  if invoke_cluster_cli -n "${namespace}" get secret "${registry_name}-auth-creds" &> /dev/null; then
    username=$(invoke_cluster_cli -n "${namespace}" get secret "${registry_name}-auth-creds" -o json | jq -r '.data.username' | base64 -d)
    password=$(invoke_cluster_cli -n "${namespace}" get secret "${registry_name}-auth-creds" -o json | jq -r '.data.password' | base64 -d)
  else
    debugf "Generating auth secret for mirror registry. FYI, those creds will be stored in a secret named '${registry_name}-auth-creds' in ${namespace} ..." >&2
    cat <<EOF | invoke_cluster_cli apply -f - >&2
apiVersion: v1
kind: Secret
type: Opaque
metadata:
  name: "${registry_name}-auth-creds"
  namespace: "${namespace}"
stringData:
  username: "${username}"
  password: "${password}"
EOF
  fi

  registry_htpasswd=$(htpasswd -Bbn "${username}" "${password}")
  cat <<EOF | invoke_cluster_cli apply -f - >&2
apiVersion: v1
kind: Secret
type: Opaque
metadata:
  name: "${registry_name}-auth"
  namespace: "${namespace}"
stringData:
  htpasswd: "${registry_htpasswd}"
EOF

  debugf "Creating the registry Deployment: deployment/${registry_name} ..." >&2
  cat <<EOF | invoke_cluster_cli apply -f - >&2
apiVersion: apps/v1
kind: Deployment
metadata:
  name: "${registry_name}"
  namespace: "${namespace}"
  labels:
    app: "${registry_name}"
spec:
  replicas: 1
  selector:
    matchLabels:
      app: "${registry_name}"
  template:
    metadata:
      labels:
        app: "${registry_name}"
    spec:
      containers:
        - image: "${image}"
          name: "${registry_name}"
          imagePullPolicy: IfNotPresent
          env:
            - name: REGISTRY_AUTH
              value: "htpasswd"
            - name: REGISTRY_AUTH_HTPASSWD_REALM
              value: "RHDH Private Registry"
            - name: REGISTRY_AUTH_HTPASSWD_PATH
              value: "/auth/htpasswd"
            - name: REGISTRY_STORAGE_DELETE_ENABLED
              value: "true"
          ports:
            - containerPort: 5000
          volumeMounts:
            - name: auth-vol
              mountPath: "/auth"
              readOnly: true
      # -----------------------------------------------------------------------
      volumes:
        - name: auth-vol
          secret:
            secretName: "${registry_name}-auth"
EOF

  debugf "Creating the registry Service: service/${registry_name} ..." >&2
  # NOTE: We need a NodePort here, for the kubelet to be able to pull insecurely from localhost.
  # We cannot use the internal Service DNS name (ending with .svc.cluster.local) because the Kubelet is responsible for
  # pulling the images and does not seem able to resolve such internal DNS names.
  cat <<EOF | invoke_cluster_cli apply -f - >&2
apiVersion: v1
kind: Service
metadata:
  name: "${registry_name}"
  namespace: "${namespace}"
  labels:
    app: "${registry_name}"
spec:
  type: NodePort
  ports:
    - port: 5000
      protocol: TCP
      targetPort: 5000
  selector:
    app: "${registry_name}"
EOF
  debugf "Waiting for service $registry_name in namespace $namespace to become ready..." >&2
  sleep 7
  local registrySvcNodePort
  registrySvcNodePort=$(invoke_cluster_cli -n "${namespace}" get service "$registry_name" -o jsonpath='{.spec.ports[0].nodePort}')
  if [[ -z "$registrySvcNodePort" ]]; then
    errorf "NodePort not allocated for service/$registry_name or Service not found"
    return 1
  fi

  debugf "Waiting for deployment $registry_name in namespace $namespace to become ready..." >&2
  if ! invoke_cluster_cli rollout status deployment/$registry_name -n $namespace --timeout=5m &>/dev/null; then
    errorf "Timed out waiting for deployment $registry_name to be ready." >&2
    return 1
  fi

  local registry_port_fwd_out="${TMPDIR}/k8s.registry_port_fwd.out.txt"
  invoke_cluster_cli port-forward "service/$registry_name" -n "$namespace" :5000 &> "${registry_port_fwd_out}" &
  local port_fwd_pid
  port_fwd_pid=$!
  debugf "Port-forwarding process: $port_fwd_pid" >&2
  sleep 7
  # Check if the port-forward is running
  if ! kill -0 $port_fwd_pid &> /dev/null; then
      errorf "Port-forwarding to the cluster registry failed to start. Logs:" >&2
      cat "${registry_port_fwd_out}"
      return 1
  fi
  trap '[[ -n "${port_fwd_pid:-}" ]] && kill ${port_fwd_pid} || true' EXIT

  local portFwdLocalPort
  portFwdLocalPort=$(grep -oP '127\.0\.0\.1:\K[0-9]+' "${registry_port_fwd_out}")
  if [[ -z "$portFwdLocalPort" ]]; then
      errorf "Failed to determine the local port. Logs:" >&2
      cat "${registry_port_fwd_out}"
      return 1
  fi
  debugf "Port-forwarding from localhost:${portFwdLocalPort} to the cluster registry..." >&2
  local internal_registry_url
  internal_registry_url="localhost:${registrySvcNodePort}"
  skopeo login -u "${username}" -p "${password}" --tls-verify=false "localhost:$portFwdLocalPort" >&2

  local kaniko_internal_registry_url
  kaniko_internal_registry_url="${registry_name}.${namespace}.svc.cluster.local:5000"

  invoke_cluster_cli -n olm create secret docker-registry internal-reg-ext-auth-for-rhdh \
      --docker-server="${internal_registry_url}" \
      --docker-username="${username}" \
      --docker-password="${password}" \
      --docker-email="admin@internal-registry-ext.example.com" \
      --dry-run=client -o=yaml | \
      invoke_cluster_cli apply -f - >&2
  invoke_cluster_cli -n olm create secret docker-registry internal-reg-auth-for-rhdh \
      --docker-server="${internal_registry_url}" \
      --docker-username="${username}" \
      --docker-password="${password}" \
      --docker-email="admin@internal-registry.example.com" \
      --dry-run=client -o=yaml | \
      invoke_cluster_cli apply -f - >&2

  # 3. Regenerate the IIB image with the local changes to the render.yaml file and build and push it from within the cluster
  update_refs_in_iib_bundles "$internal_registry_url" "localhost:$portFwdLocalPort" >&2

  local imageStreamWithTag="rhdh/iib:${IIB_TAG}"
  local result="${internal_registry_url}/${imageStreamWithTag}"
  local kanikoResult="${kaniko_internal_registry_url}/${imageStreamWithTag}"

  # 4. Rebuild the IIB image in the cluster using Kaniko
  debugf "Rebuilding the IIB Image using Kaniko in the cluster..." >&2
  invoke_cluster_cli -n "${namespace}" create secret docker-registry kaniko-registry-secret \
      --docker-server="${kaniko_internal_registry_url}" \
      --docker-username="${username}" \
      --docker-password="${password}" \
      --docker-email="admin@internal-registry-ext.kaniko.example.com" \
      --dry-run=client -o=yaml | \
      invoke_cluster_cli apply -f - >&2
  local timestamp
  local kanikoJobName
  local kanikoPod
  local kanikoLogsPid
  local localContext
  timestamp=$(date +%s)
  kanikoJobName="kaniko-build-${timestamp}"
  cat <<EOF | invoke_cluster_cli apply -f - >&2
apiVersion: batch/v1
kind: Job
metadata:
  name: "${kanikoJobName}"
  namespace: "${namespace}"
spec:
  backoffLimit: 0
  ttlSecondsAfterFinished: 3600
  template:
    spec:
      containers:
      - name: kaniko
        image: gcr.io/kaniko-project/executor:debug
        command: ["/bin/sh", "-c"]
        args:
        - |
            while [ ! -f /workspace/context.tar.gz ]; do echo 'Waiting for the build context archive...'; sleep 2; done
            /kaniko/executor --context=tar:///workspace/context.tar.gz --dockerfile=rhdh.Dockerfile --destination=$kanikoResult
        #resources:
        #  requests:
        #    cpu: 250m
        #    memory: 512Mi
        #  limits:
        #    cpu: 500m
        #    memory: 1Gi
        volumeMounts:
        - name: build-context
          mountPath: /workspace
        - name: registry-secret
          mountPath: /kaniko/.docker/
          readOnly: true
      restartPolicy: Never
      volumes:
      - name: build-context
        emptyDir: {}
      - name: registry-secret
        secret:
          secretName: kaniko-registry-secret
          items:
          - key: .dockerconfigjson
            path: config.json
EOF

  kanikoPod=$(invoke_cluster_cli -n "${namespace}" get pods --selector=job-name="${kanikoJobName}" -o jsonpath='{.items[0].metadata.name}')
  if [ -z "$kanikoPod" ]; then
    errorf "unable to determine the Kaniko Pod"
    return 1
  fi
  debugf "Waiting for Kaniko pod $kanikoPod to be ready..." >&2
  invoke_cluster_cli -n "${namespace}" wait --for=condition=Ready "pod/$kanikoPod" --timeout=60s >&2
  invoke_cluster_cli -n "${namespace}" logs -f "${kanikoPod}" >&2 &
  kanikoLogsPid=$!
  trap '[[ -n "${kanikoLogsPid:-}" ]] && kill ${kanikoLogsPid} &>/dev/null || true' EXIT

  localContext=context.tar.gz
  tar -czf "${localContext}" -C rhdh . >&2
  invoke_cluster_cli -n "${namespace}" cp "${localContext}" "${kanikoPod}:/workspace/${localContext}" >&2

  invoke_cluster_cli -n "${namespace}" wait --for=condition=complete "job/${kanikoJobName}" --timeout=300s >&2

  debugf "IIB built and pushed to internal cluster registry: $result..." >&2
  printf "%s" "${result}"
}

##########################################################################################
# Script start
##########################################################################################
if [[ "$#" -lt 1 ]]; then
  usage
  exit 0
fi

# minimum requirements
if ! command -v jq &> /dev/null; then
  errorf "Please install jq 1.2+ from an RPM or https://pypi.org/project/jq/"
  exit 1
fi
if ! command -v skopeo &> /dev/null; then
  errorf "Please install skopeo 1.11+"
  exit 1
fi

TMPDIR=$(mktemp -d)
pushd "${TMPDIR}" > /dev/null
debugf ">>> WORKING DIR: $TMPDIR <<<"
# shellcheck disable=SC2064
trap "rm -fr $TMPDIR || true" EXIT

detect_ocp_and_set_env_var
if [[ "${IS_OPENSHIFT}" = "true" ]]; then
  debugf "Detected an OpenShift cluster"
  if ! command -v oc &> /dev/null; then
    errorf "Please install oc 4.10+ from an RPM or https://mirror.openshift.com/pub/openshift-v4/clients/ocp/"
    exit 1
  fi
  # Check we're logged into a cluster
  if ! oc whoami &> /dev/null; then
    errorf "Not logged into an OpenShift cluster"
    exit 1
  fi
else
  if ! command -v oc &> /dev/null && ! command -v kubectl &> /dev/null; then
    errorf "Please install kubectl or invoke_cluster_cli 4.10+ (from an RPM or https://mirror.openshift.com/pub/openshift-v4/clients/ocp/)"
    exit 1
  fi
  debugf "Falling back to a standard K8s cluster"
  # Check that OLM is installed
  if ! invoke_cluster_cli get crd catalogsources.operators.coreos.com &> /dev/null; then
    errorf "
OLM not installed (CatalogSource CRD not found) or you don't have enough permissions.
Check that you are correctly logged into the cluster and that OLM is installed.
See https://olm.operatorframework.io/docs/getting-started/#installing-olm-in-your-cluster to install OLM."
    exit 1
  fi
fi

OCP_VER="v4.16"
OCP_ARCH="x86_64"
if [[ "${IS_OPENSHIFT}" = "true" ]]; then
  # log into your OCP cluster before running this or you'll get null values for OCP vars!
  ocpVerJson=$(invoke_cluster_cli version -o json)
  OCP_VER="v$(echo "$ocpVerJson" | jq -r '.openshiftVersion' | sed -r -e "s#([0-9]+\.[0-9]+)\..+#\1#")"
  if [[ $OCP_VER == "vnull" ]]; then # try releaseClientVersion = 4.16.14
    OCP_VER="v$(echo "$ocpVerJson" | jq -r '.releaseClientVersion' | sed -r -e "s#([0-9]+\.[0-9]+)\..+#\1#")"
  fi
  OCP_ARCH="$(echo "$ocpVerJson" | jq -r '.serverVersion.platform' | sed -r -e "s#linux/##")"
  if [[ $OCP_ARCH == "amd64" ]]; then
    OCP_ARCH="x86_64"
  fi
fi

# if logged in, this should return something like latest-v4.12-x86_64
IIB_TAG="latest-${OCP_VER}-${OCP_ARCH}"
TO_INSTALL=""

while [[ "$#" -gt 0 ]]; do
  case $1 in
    '--install-operator')
      TO_INSTALL="$2"
      shift 1
      ;;
    '--next'|'--latest')
      # if logged in, this should return something like latest-v4.12-x86_64 or next-v4.12-x86_64
      IIB_TAG="${1/--/}-${OCP_VER}-$OCP_ARCH"
      ;;
    '-v')
      IIB_TAG="${2}-${OCP_VER}-$OCP_ARCH"
      OLM_CHANNEL="fast-${2}"
      shift 1
      ;;
    '--catalog-source')
      UPSTREAM_IIB_OVERRIDE="$2"
      shift 1
      ;;
    '--install-plan-approval')
      if [[ "$2" != "Manual" && "$2" != "Automatic" ]]; then
        errorf "Unknown parameter used: $2. Must be Manual or Automatic."
        usage
        exit 1
      fi
      INSTALL_PLAN_APPROVAL="$2"
      shift 1
      ;;
    '-h'|'--help')
      usage
      exit 0
      ;;
    *)
      errorf "Unknown parameter is used: $1."
      usage
      exit 1
      ;;
  esac
  shift 1
done

if [[ $UPSTREAM_IIB_OVERRIDE ]]; then
  UPSTREAM_IIB="$UPSTREAM_IIB_OVERRIDE"
else
  UPSTREAM_IIB="quay.io/rhdh/iib:${IIB_TAG}"
fi

# shellcheck disable=SC2086
UPSTREAM_IIB_MANIFEST="$(skopeo inspect docker://${UPSTREAM_IIB} --raw || exit 2)"
if [[ $UPSTREAM_IIB_MANIFEST == *"Error parsing image name "* ]] || [[ $UPSTREAM_IIB_MANIFEST == *"manifest unknown"* ]]; then
  errorf "Problem with image $UPSTREAM_IIB: $UPSTREAM_IIB_MANIFEST"
  exit 3
else
  infof "Using IIB from image $UPSTREAM_IIB"
  IIB_IMAGE="${UPSTREAM_IIB}"
fi

# Add CatalogSource for the IIB
IIB_NAME="${UPSTREAM_IIB##*:}"
IIB_NAME="${IIB_NAME//_/-}"
IIB_NAME="${IIB_NAME//./-}"
IIB_NAME="$(echo "$IIB_NAME" | tr '[:upper:]' '[:lower:]')"
OPERATOR_NAME_TO_INSTALL=${TO_INSTALL:-rhdh}
if [[ $UPSTREAM_IIB_OVERRIDE == "brew.registry.redhat.io/rh-osbs/iib-pub-pending"* ]]; then
  CATALOGSOURCE_NAME="brew-registry-stage"
  DISPLAY_NAME_SUFFIX="brew-registry-stage"
else
  CATALOGSOURCE_NAME="${OPERATOR_NAME_TO_INSTALL}-${OLM_CHANNEL}"
  DISPLAY_NAME_SUFFIX="${IIB_NAME}"
fi

# Using the current working dir, otherwise tools like 'skopeo login' will attempt to write to /run, which
# might be restricted in CI environments.
export REGISTRY_AUTH_FILE="${TMPDIR}/.auth.json"

newIIBImage=${IIB_IMAGE}

if [[ "${IS_OPENSHIFT}" = "true" ]]; then
  # Defaulting to the hosted control plane behavior which has more chances to work
  CONTROL_PLANE_TECH=$(oc get infrastructure cluster -o jsonpath='{.status.controlPlaneTopology}' || \
    (warnf 'Could not determine the cluster type => defaulting to the hosted control plane behavior' >&2 && echo 'External'))
  IS_HOSTED_CONTROL_PLANE="false"
  if [[ "${CONTROL_PLANE_TECH}" == "External" ]]; then
    # 'External' indicates that the control plane is hosted externally to the cluster
    # and that its components are not visible within the cluster.
    IS_HOSTED_CONTROL_PLANE="true"
  fi

  if [[ "${IS_HOSTED_CONTROL_PLANE}" = "true" ]]; then
    infof "Detected an OpenShift cluster with a hosted control plane"
    if ! command -v umoci &> /dev/null; then
      errorf "Please install umoci 0.4+. See https://github.com/opencontainers/umoci?tab=readme-ov-file#install"
      exit 1
    fi
    if ! command -v opm &> /dev/null; then
      errorf "Please install opm v1.47+. See https://github.com/operator-framework/operator-registry/releases"
      exit 1
    fi
    newIIBImage=$(ocp_install_hosted_control_plane_cluster)
  else
    newIIBImage=$(ocp_install_regular_cluster)
  fi
else
  # K8s cluster with OLM installed
  infof "Detected a Kubernetes cluster"
  if ! command -v yq &> /dev/null; then
    errorf "Please install yq 4.44+. See https://github.com/mikefarah/yq/#install"
    exit 1
  fi
  if ! command -v umoci &> /dev/null; then
    errorf "Please install umoci 0.4+. See https://github.com/opencontainers/umoci?tab=readme-ov-file#install"
    exit 1
  fi
  if ! command -v opm &> /dev/null; then
    errorf "Please install opm v1.47+. See https://github.com/operator-framework/operator-registry/releases"
    exit 1
  fi
  newIIBImage=$(k8s_install)
fi

debugf "newIIBImage=${newIIBImage}"

NAMESPACE_CATALOGSOURCE="olm"
if [[ "${IS_OPENSHIFT}" = "true" ]]; then
  NAMESPACE_CATALOGSOURCE="openshift-marketplace"
fi

echo "apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: ${CATALOGSOURCE_NAME}
  namespace: ${NAMESPACE_CATALOGSOURCE}
spec:
  sourceType: grpc
  image: ${newIIBImage}
  secrets:
  - internal-reg-auth-for-rhdh
  - internal-reg-ext-auth-for-rhdh
  publisher: ${DISPLAY_NAME_SUFFIX}
  displayName: ${DISPLAY_NAME_SUFFIX}
" > "$TMPDIR"/CatalogSource.yml && invoke_cluster_cli apply -f "$TMPDIR"/CatalogSource.yml

OPERATOR_GROUP_MANIFEST="
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: ${OPERATOR_NAME_TO_INSTALL}-operator-group
  namespace: ${NAMESPACE_SUBSCRIPTION}
"

SUBSCRIPTION_MANIFEST="
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: ${OPERATOR_NAME_TO_INSTALL}
  namespace: ${NAMESPACE_SUBSCRIPTION}
spec:
  channel: $OLM_CHANNEL
  installPlanApproval: ${INSTALL_PLAN_APPROVAL}
  name: ${OPERATOR_NAME_TO_INSTALL}
  source: ${CATALOGSOURCE_NAME}
  sourceNamespace: ${NAMESPACE_CATALOGSOURCE}
"

if [ -z "${TO_INSTALL}" ]; then
  echo
  echo -n "Done. "
  if [[ "${IS_OPENSHIFT}" = "true" ]]; then
    echo "Now log into the OCP web console as an admin, then go to Operators > OperatorHub, search for Red Hat Developer Hub, and install the Red Hat Developer Hub Operator."
  else
    echo "To install the operator, you will need to create an OperatorGroup and a Subscription. You can do so with the following command:

cat <<EOF | kubectl -n ${NAMESPACE_SUBSCRIPTION} apply -f -
${OPERATOR_GROUP_MANIFEST}
---
${SUBSCRIPTION_MANIFEST}
EOF
"
  fi
  exit 0
fi

# Create project if necessary
if ! invoke_cluster_cli get namespace "$NAMESPACE_SUBSCRIPTION" > /dev/null 2>&1; then
  debugf "Namespace $NAMESPACE_SUBSCRIPTION does not exist; creating it"
  invoke_cluster_cli create namespace "$NAMESPACE_SUBSCRIPTION"
fi

# Create OperatorGroup to allow installing all-namespaces operators in $NAMESPACE_SUBSCRIPTION
debugf "Creating OperatorGroup to allow all-namespaces operators to be installed"
echo "${OPERATOR_GROUP_MANIFEST}" > "$TMPDIR"/OperatorGroup.yml && invoke_cluster_cli apply -f "$TMPDIR"/OperatorGroup.yml

# Create subscription for operator
echo "${SUBSCRIPTION_MANIFEST}" > "$TMPDIR"/Subscription.yml && invoke_cluster_cli apply -f "$TMPDIR"/Subscription.yml

if [[ "${IS_OPENSHIFT}" = "true" ]]; then
  OCP_CONSOLE_ROUTE_HOST=$(invoke_cluster_cli get route console -n openshift-console -o=jsonpath='{.spec.host}')
  CLUSTER_ROUTER_BASE=$(invoke_cluster_cli get ingress.config.openshift.io/cluster '-o=jsonpath={.spec.domain}')
  echo -n "

To install, go to:
https://${OCP_CONSOLE_ROUTE_HOST}/catalog/ns/${NAMESPACE_SUBSCRIPTION}?catalogType=OperatorBackedService

Or "
else
  echo -n "

To install on Kubernetes:

1. Register an account on registry.redhat.io if you don't already have one.

2. Create an image pull secret to enable pulling the RHDH Database image from registry.redhat.io:

kubectl -n ${NAMESPACE_SUBSCRIPTION} create secret docker-registry rh-pull-secret \\
    --docker-server=registry.redhat.io \\
    --docker-username=\"<user_name>\" \\
    --docker-password=\"<password>\" \\
    --docker-email=\"<email>\"

3. Add the pull secret to the namespace default service account:

kubectl -n ${NAMESPACE_SUBSCRIPTION} patch serviceaccount default -p '{\"imagePullSecrets\": [{\"name\": \"rh-pull-secret\"}]}'

4. And then "
fi

CLI_TOOL="kubectl"
if [[ "${IS_OPENSHIFT}" = "true" ]]; then
  CLI_TOOL="oc"
fi
CR_EXAMPLE="
cat <<EOF | ${CLI_TOOL} apply -f -
apiVersion: rhdh.redhat.com/v1alpha3
kind: Backstage
metadata:
  name: developer-hub
  namespace: ${NAMESPACE_SUBSCRIPTION}
spec:
  application:
    appConfig:
      mountPath: /opt/app-root/src
    extraFiles:
      mountPath: /opt/app-root/src
    replicas: 1
    route:
      enabled: true
  database:
    enableLocalDb: true
EOF"

echo "run this to create an RHDH instance:
${CR_EXAMPLE}
"

if [[ "${IS_OPENSHIFT}" = "true" ]]; then
  echo "
Once deployed, Developer Hub will be available at
https://backstage-developer-hub-${NAMESPACE_SUBSCRIPTION}.${CLUSTER_ROUTER_BASE}
"
fi
