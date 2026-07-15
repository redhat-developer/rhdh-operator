#!/bin/bash

set -euo pipefail

: "${OPM:?OPM environment variable not set}"
: "${BUNDLE_IMGS:?BUNDLE_IMGS environment variable not set}"
: "${OUTPUT_FILE:?OUTPUT_FILE environment variable not set}"

if [[ -f /etc/containers/registries.conf ]] && grep -q '^\[registries' /etc/containers/registries.conf 2>/dev/null; then
	printf 'unqualified-search-registries = ["docker.io"]\n' > /tmp/opm-registries.conf
	CONTAINERS_REGISTRIES_CONF=/tmp/opm-registries.conf "${OPM}" render "${BUNDLE_IMGS}" --output yaml >> "${OUTPUT_FILE}"
else
	"${OPM}" render "${BUNDLE_IMGS}" --output yaml >> "${OUTPUT_FILE}"
fi
