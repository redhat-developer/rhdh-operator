#!/bin/bash
#
# Copyright Red Hat, Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
# This script simulates the Konflux build process locally using Hermeto.
# It can either build the dependency cache or build a container image.
set -euo pipefail

#######################################
# Constants
#######################################
readonly LOCAL_CACHE_BASEDIR='./hermeto-cache/'
readonly HERMETO_IMAGE='quay.io/konflux-ci/hermeto:0.60.1'

# Target platform for cross-builds (e.g., linux/arm64, linux/amd64)
TARGET_PLATFORM="${TARGET_PLATFORM:-}"

#######################################
# Normalizes architecture names to Linux conventions used by RPM repos.
#######################################
normalize_arch() {
  local arch="$1"
  case "${arch}" in
    arm64)  echo "aarch64" ;;
    amd64)  echo "x86_64" ;;
    *)      echo "${arch}" ;;
  esac
}

#######################################
# Derives the architecture name from TARGET_PLATFORM.
# Falls back to native architecture if TARGET_PLATFORM is not set.
#######################################
get_target_arch() {
  if [[ -z "${TARGET_PLATFORM}" ]]; then
    normalize_arch "$(uname -m)"
    return
  fi

  local platform_arch="${TARGET_PLATFORM#*/}"
  normalize_arch "${platform_arch}"
}

TARGET_ARCH="$(get_target_arch)"

#######################################
# Prints usage information and exits.
#######################################
usage() {
  cat << EOF

Usage: Simulates the Konflux build process by building a hermeto cache using
  dependencies found in the given component directory, then builds a container
  image using the hermeto cache with network disabled.

Required:
  -d, --directory <path>   The directory of the component to build

Options:
  -i, --image <name>      Container image name (e.g., quay.io/example/image:tag)
                          Required to build image unless --no-image is specified
  --no-cache              Skip cache build (use existing cache)
  --no-image              Skip image build (only build cache)
  -h, --help              Show this help message

Environment variables:
  TARGET_PLATFORM         Target platform for podman (e.g., linux/arm64, linux/amd64).
                          If not set, builds for the native platform.

Examples (assume you are in the root of the rhdh-operator repository):
  $0 -d . --no-image                                # Build cache only
  $0 -d . -i quay.io/example/image:tag              # Build cache and image
  $0 -d . -i quay.io/example/image:tag --no-cache   # Build image only (cache must exist)

Cross-platform build (ARM on x86), requires qemu-user-static:
  TARGET_PLATFORM=linux/arm64 $0 -d . -i quay.io/example/image:tag
EOF
  exit 1
}

#######################################
# Check for GNU sed on macOS
#######################################
check_gnu_sed() {
  if [[ "$OSTYPE" == "darwin"* ]]; then
    if ! sed --version 2>/dev/null | grep -q "GNU sed"; then
      echo "Error: GNU sed is required on macOS." >&2
      echo "Install it with: brew install gnu-sed" >&2
      echo "Then add to your PATH: export PATH=\"\$(brew --prefix)/opt/gnu-sed/libexec/gnubin:\$PATH\"" >&2
      exit 1
    fi
  fi
}

#######################################
# Transforms a Dockerfile to inject Hermeto/cachi2 configuration.
#######################################
transform_containerfile() {
  local containerfile="$1"
  local transformed_containerfile="$2"

  cp "${containerfile}" "${transformed_containerfile}"

  # Configure dnf/microdnf to use the cachi2 repo
  sed -i "/RUN *\(dnf\|microdnf\) install/i RUN rm -r /etc/yum.repos.d/* && cp /cachi2/output/deps/rpm/${TARGET_ARCH}/repos.d/hermeto.repo /etc/yum.repos.d/" \
    "${transformed_containerfile}"

  # Prepend cachi2 env sourcing to every RUN command
  sed -i 's/^\s*RUN /RUN . \/cachi2\/cachi2.env \&\& /' "$transformed_containerfile"
}

#######################################
# Builds the dependency cache using Hermeto.
#######################################
build_cache() {
  local component_dir="$1"
  local local_cache_dir="$2"
  local local_cache_output_dir="$3"
  local platform_args=()

  if [[ -n "${TARGET_PLATFORM}" ]]; then
    platform_args=("--platform" "${TARGET_PLATFORM}")
    echo "Building cache for platform: ${TARGET_PLATFORM} (arch: ${TARGET_ARCH})"
  fi

  mkdir -p "${local_cache_output_dir}"

  podman pull "${platform_args[@]}" "${HERMETO_IMAGE}"

  podman run --rm \
    "${platform_args[@]}" \
    -v "${component_dir}:/source:z" \
    -v "${local_cache_dir}:/cachi2:z" \
    -w /source \
    "${HERMETO_IMAGE}" \
    --log-level DEBUG \
    fetch-deps \
    --source . \
    --output /cachi2/output \
    '[{"type": "rpm", "path": "."}, {"type": "gomod", "path": "."}]'

  podman run --rm \
    "${platform_args[@]}" \
    -v "${component_dir}:/source:z" \
    -v "${local_cache_dir}:/cachi2:z" \
    -w /source \
    "${HERMETO_IMAGE}" \
    generate-env --format env --output /cachi2/cachi2.env /cachi2/output

  podman run --rm \
    "${platform_args[@]}" \
    -v "${component_dir}:/source:z" \
    -v "${local_cache_dir}:/cachi2:z" \
    -w /source \
    "${HERMETO_IMAGE}" \
    inject-files /cachi2/output
  return 0
}

#######################################
# Builds a container image using the hermeto cache.
#######################################
build_image() {
  local component_dir="$1"
  local local_cache_dir="$2"
  local image="$3"
  local platform_args=()

  if [[ -n "${TARGET_PLATFORM}" ]]; then
    platform_args=("--platform" "${TARGET_PLATFORM}")
    echo "Building image for platform: ${TARGET_PLATFORM} (arch: ${TARGET_ARCH})"
  fi

  if [[ ! -d "${local_cache_dir}" ]]; then
    echo "Local cache dir does not exist. Please run the script without --no-cache first."
    echo "example: $0 -d ${component_dir} -i <image>"
    exit 1
  fi

  transform_containerfile \
    "${component_dir}/Dockerfile" \
    "${component_dir}/Dockerfile.hermeto"

  # Prevent podman from injecting host RHEL subscriptions into the container
  EMPTY_DIR=$(mktemp -d)
  trap 'rm -rf "${EMPTY_DIR}" || true' EXIT

  podman build -t "${image}" \
    "${platform_args[@]}" \
    --network none \
    --no-cache \
    -f "${component_dir}/Dockerfile.hermeto" \
    -v "${local_cache_dir}:/cachi2" \
    -v /dev/null:/run/secrets/redhat.repo \
    -v "${EMPTY_DIR}:/run/secrets/rhsm:z" \
    -v "${EMPTY_DIR}:/run/secrets/etc-pki-entitlement:z" \
    "${component_dir}"
}

#######################################
# Main entry point
#######################################
main() {
  check_gnu_sed

  local component_dir=""
  local image=""
  local no_cache=false
  local no_image=false

  while [[ $# -gt 0 ]]; do
    case "$1" in
      -d|--directory)
        if [[ -z "${2:-}" ]]; then
          echo "Error: -d/--directory requires a path argument" >&2
          usage
        fi
        component_dir="$2"
        shift 2
        ;;
      -i|--image)
        if [[ -z "${2:-}" ]]; then
          echo "Error: -i/--image requires an image name argument" >&2
          usage
        fi
        image="$2"
        shift 2
        ;;
      --no-cache)
        no_cache=true
        shift
        ;;
      --no-image)
        no_image=true
        shift
        ;;
      -h|--help)
        usage
        ;;
      *)
        echo "Error: Unknown option: $1" >&2
        usage
        ;;
    esac
  done

  if [[ -z "${component_dir}" ]]; then
    echo "Error: Directory is required. Use -d or --directory to specify." >&2
    usage
  fi

  if [[ "${no_cache}" == true && "${no_image}" == true ]]; then
    echo "Error: Nothing to do - both cache and image builds are disabled" >&2
    usage
  fi

  if [[ -z "${image}" ]]; then
    no_image=true
  fi

  mkdir -p "${LOCAL_CACHE_BASEDIR}"

  local resolved_component_dir
  local local_cache_dir
  local local_cache_output_dir

  resolved_component_dir="$(realpath "${component_dir}")"
  local_cache_dir="$(realpath "${LOCAL_CACHE_BASEDIR}")/$(basename "${resolved_component_dir}")"
  local_cache_output_dir="${local_cache_dir}/output"

  echo "Component dir: ${resolved_component_dir}"
  echo "Local cache dir: ${local_cache_dir}"

  if [[ "${no_cache}" == false ]]; then
    echo "Building cache..."
    build_cache "${resolved_component_dir}" "${local_cache_dir}" "${local_cache_output_dir}"
  else
    echo "Skipping cache build (--no-cache specified)"
  fi

  if [[ "${no_image}" == false ]]; then
    echo "Building image..."
    build_image "${resolved_component_dir}" "${local_cache_dir}" "${image}"
  else
    echo "Skipping image build (--no-image specified or -i/--image not provided)"
  fi
}

main "$@"
