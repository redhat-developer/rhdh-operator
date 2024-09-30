#!/bin/bash
#
#  Copyright (c) 2024 Red Hat, Inc.
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#
# Script to streamline installing an IIB image in an OpenShift cluster for testing.
#
# Requires: oc, jq

set -e

RED='\033[0;31m'
NC='\033[0m'

NAMESPACE_CATALOGSOURCE="openshift-marketplace"
NAMESPACE_SUBSCRIPTION="rhdh-operator"
OLM_CHANNEL="fast"

errorf() {
  echo -e "${RED}$1${NC}"
}

usage() {
echo "
This script streamlines testing IIB images by configuring an OpenShift cluster to enable it to use the specified IIB image
as a catalog source. The CatalogSource is created in the openshift-marketplace namespace,
and is named 'operatorName-channelName', eg., rhdh-fast

If IIB installation fails, see https://docs.engineering.redhat.com/display/CFC/Test and
follow steps in section 'Adding Brew Pull Secret'

Usage:
  $0 [OPTIONS]

Options:
  --latest                     : Install from iib quay.io/rhdh/iib:latest-\$OCP_VER-\$OCP_ARCH (eg., latest-v4.14-x86_64) [default]
  --next                       : Install from iib quay.io/rhdh/iib:next-\$OCP_VER-\$OCP_ARCH (eg., next-v4.14-x86_64)
  --install-operator <NAME>    : Install operator named \$NAME after creating CatalogSource

Examples:
  $0 \\
    --install-operator rhdh          # RC release in progess (from latest tag and stable branch )

  $0 \\
    --next --install-operator rhdh   # CI future release (from next tag and upstream main branch)
"
}

if [[ "$#" -lt 1 ]]; then usage; exit 0; fi

# minimum requirements
if [[ ! $(command -v oc) ]]; then
  errorf "Please install oc 4.10+ from an RPM or https://mirror.openshift.com/pub/openshift-v4/clients/ocp/"
  exit 1
fi
if [[ ! $(command -v jq) ]]; then
  errorf "Please install jq 1.2+ from an RPM or https://pypi.org/project/jq/"
  exit 1
fi

# Check we're logged into a cluster
if ! oc whoami > /dev/null 2>&1; then
  errorf "Not logged into an OpenShift cluster"
  exit 1
fi

# log into your OCP cluster before running this or you'll get null values for OCP vars!
OCP_VER="v$(oc version -o json | jq -r '.openshiftVersion' | sed -r -e "s#([0-9]+\.[0-9]+)\..+#\1#")"
OCP_ARCH="$(oc version -o json | jq -r '.serverVersion.platform' | sed -r -e "s#linux/##")"
if [[ $OCP_ARCH == "amd64" ]]; then OCP_ARCH="x86_64"; fi
# if logged in, this should return something like latest-v4.12-x86_64
UPSTREAM_IIB="quay.io/rhdh/iib:latest-${OCP_VER}-${OCP_ARCH}";

while [[ "$#" -gt 0 ]]; do
  case $1 in
    '--install-operator')
      # Create project if necessary
      if ! oc get project "$NAMESPACE_SUBSCRIPTION" > /dev/null 2>&1; then
        echo "Project $NAMESPACE_SUBSCRIPTION does not exist; creating it"
        oc create namespace "$NAMESPACE_SUBSCRIPTION"
      fi
      TO_INSTALL="$2"; shift 1;;
    '--next'|'--latest')
      # if logged in, this should return something like latest-v4.12-x86_64 or next-v4.12-x86_64
      UPSTREAM_IIB="quay.io/rhdh/iib:${1/--/}-${OCP_VER}-$OCP_ARCH";;
    '-h'|'--help') usage; exit 0;;
    *) echo "[ERROR] Unknown parameter is used: $1."; usage; exit 1;;
  esac
  shift 1
done

# check if the IIB we're going to install as a catalog source exists before trying to install it
if [[ ! $(command -v skopeo) ]]; then
  errorf "Please install skopeo 1.11+"
  exit 1
fi

# shellcheck disable=SC2086
UPSTREAM_IIB_MANIFEST="$(skopeo inspect docker://${UPSTREAM_IIB} --raw || exit 2)"
# echo "Got: $UPSTREAM_IIB_MANIFEST"
if [[ $UPSTREAM_IIB_MANIFEST == *"Error parsing image name "* ]] || [[ $UPSTREAM_IIB_MANIFEST == *"manifest unknown"* ]]; then
  echo "$UPSTREAM_IIB_MANIFEST"; exit 3
else
  echo "[INFO] Using iib from image $UPSTREAM_IIB"
  IIB_IMAGE="${UPSTREAM_IIB}"
fi

TMPDIR=$(mktemp -d)
# shellcheck disable=SC2064
trap "rm -fr $TMPDIR" EXIT

# Add ImageContentSourcePolicy to resolve references to images not on quay as if from quay.io
echo "[INFO] Adding ISCP to resolve references to images not on quay.io as if from quay.io"
ICSP_URL="quay.io/rhdh/"
ICSP_URL_PRE=${ICSP_URL%%/*}
# echo "[DEBUG] ${ICSP_URL_PRE}, ${ICSP_URL_PRE//./-}, ${ICSP_URL}"
echo "apiVersion: operator.openshift.io/v1alpha1
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
" > "$TMPDIR/ImageContentSourcePolicy_${ICSP_URL_PRE}.yml" && oc apply -f "$TMPDIR/ImageContentSourcePolicy_${ICSP_URL_PRE}.yml"

CATALOGSOURCE_NAME="${TO_INSTALL}-${OLM_CHANNEL}"
DISPLAY_NAME_SUFFIX="${TO_INSTALL}"

# Add CatalogSource for the IIB
if [ -z "$TO_INSTALL" ]; then
  IIB_NAME="${UPSTREAM_IIB##*:}"
  IIB_NAME="${IIB_NAME//_/-}"
  IIB_NAME="${IIB_NAME//./-}"
  IIB_NAME="$(echo "$IIB_NAME" | tr '[:upper:]' '[:lower:]')"
  CATALOGSOURCE_NAME="rhdh-iib-${IIB_NAME}-${OLM_CHANNEL}"
  DISPLAY_NAME_SUFFIX="${IIB_NAME}"
fi
echo "apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: ${CATALOGSOURCE_NAME}
  namespace: ${NAMESPACE_CATALOGSOURCE}
spec:
  sourceType: grpc
  image: ${IIB_IMAGE}
  publisher: IIB testing ${DISPLAY_NAME_SUFFIX}
  displayName: IIB testing catalog ${DISPLAY_NAME_SUFFIX}
" > "$TMPDIR"/CatalogSource.yml && oc apply -f "$TMPDIR"/CatalogSource.yml

if [ -z "$TO_INSTALL" ]; then
  echo "Done. Now log into the OCP web console as an admin, then go to Operators > OperatorHub, search for Red Hat Developer Hub, and install the Red Hat Developer Hub Operator."
  exit 0
fi

# Create OperatorGroup to allow installing all-namespaces operators in $NAMESPACE_SUBSCRIPTION
echo "Creating OperatorGroup to allow all-namespaces operators to be installed"
echo "apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: rhdh-operator-group
  namespace: ${NAMESPACE_SUBSCRIPTION}
" > "$TMPDIR"/OperatorGroup.yml && oc apply -f "$TMPDIR"/OperatorGroup.yml

# Create subscription for operator
echo "apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: $TO_INSTALL
  namespace: ${NAMESPACE_SUBSCRIPTION}
spec:
  channel: $OLM_CHANNEL
  installPlanApproval: Automatic
  name: $TO_INSTALL
  source: ${CATALOGSOURCE_NAME}
  sourceNamespace: ${NAMESPACE_CATALOGSOURCE}
" > "$TMPDIR"/Subscription.yml && oc apply -f "$TMPDIR"/Subscription.yml

CLUSTER_ROUTER_BASE=$(oc get route console -n openshift-console -o=jsonpath='{.spec.host}' | sed 's/^[^.]*\.//')
echo "

To install, go to:
https://console-openshift-console.${CLUSTER_ROUTER_BASE}/catalog/ns/${NAMESPACE_SUBSCRIPTION}?catalogType=OperatorBackedService

Or run this:

echo \"apiVersion: rhdh.redhat.com/v1alpha2
kind: Backstage
metadata:
  name: developer-hub
  namespace: rhdh-operator
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
\" | oc apply -f-

Once deployed, Developer Hub will be available at
https://backstage-developer-hub-rhdh-operator.${CLUSTER_ROUTER_BASE}
"