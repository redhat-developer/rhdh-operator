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
# Same path as CI (.github/actions/docker-build); keep cache out of the git tree.
readonly LOCAL_CACHE_DIR='/tmp/hermeto-cache/operator'

# Image tag comes from HERMETO_IMAGE in the Makefile (also used by CI).
HERMETO_IMAGE="${HERMETO_IMAGE:-}"

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
  HERMETO_IMAGE            Hermeto image (default: HERMETO_IMAGE from the Makefile)
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
# Default UID in an image (container user namespace).
#######################################
hermetic_container_uid() {
  local image="$1"
  local container_user="${2:-}"
  local platform_args=()
  local -a run_args

  if [[ -n "${TARGET_PLATFORM}" ]]; then
    platform_args=(--platform "${TARGET_PLATFORM}")
  fi

  run_args=(run --rm "${platform_args[@]}" --entrypoint id)
  if [[ -n "${container_user}" ]]; then
    run_args+=(--user "${container_user}")
  fi
  run_args+=("${image}" -u)

  podman "${run_args[@]}"
}

#######################################
# Map a container UID to the host UID used for volume access (rootless Podman).
#######################################
hermetic_host_uid_for_container() {
  local container_uid="$1"
  local subuid_line subuid_start subuid_count

  if [[ "${container_uid}" == "0" ]]; then
    echo "Error: use hermetic_probe_rootless_root_host_uid for container UID 0" >&2
    return 1
  fi

  subuid_line=$(grep "^${USER}:" /etc/subuid | head -1)
  if [[ -z "${subuid_line}" ]]; then
    echo "Error: no /etc/subuid entry for ${USER}" >&2
    return 1
  fi

  subuid_start=$(echo "${subuid_line}" | cut -d: -f2)
  subuid_count=$(echo "${subuid_line}" | cut -d: -f3)

  if [[ "${container_uid}" -lt 1 || "${container_uid}" -ge "${subuid_count}" ]]; then
    echo "Error: container UID ${container_uid} is outside the subuid range" >&2
    return 1
  fi

  echo $(( subuid_start + container_uid - 1 ))
}

#######################################
# Host UID for container UID 0 (first uid_map entry, not subuid arithmetic).
#######################################
hermetic_probe_rootless_root_host_uid() {
  local image="$1"
  local local_cache_dir="$2"
  local probe_name=".hermeto-uid-probe.$$"
  local probe_path="${local_cache_dir}/${probe_name}"
  local platform_args=()
  local prior_mode

  if [[ -n "${TARGET_PLATFORM}" ]]; then
    platform_args=(--platform "${TARGET_PLATFORM}")
  fi

  prior_mode=$(stat -c '%a' "${local_cache_dir}")
  chmod o+rwx "${local_cache_dir}"

  if ! podman run --rm "${platform_args[@]}" --user 0 \
    -v "${local_cache_dir}:/cachi2:z" --entrypoint touch "${image}" "/cachi2/${probe_name}"; then
    chmod "${prior_mode}" "${local_cache_dir}"
    echo "Error: could not probe host UID for root in ${image}" >&2
    exit 1
  fi

  chmod "${prior_mode}" "${local_cache_dir}"
  stat -c '%u' "${probe_path}"
  rm -f "${probe_path}"
}

#######################################
# Host UIDs used during podman build for stages that mount /cachi2.
#######################################
hermetic_build_host_uids() {
  local component_dir="$1"
  local local_cache_dir="$2"
  local dockerfile="${component_dir}/Dockerfile"
  local go_image ubi_image
  local builder_container_uid builder_host_uid root_host_uid

  if [[ ! -f "${dockerfile}" ]]; then
    echo "Error: ${dockerfile} not found" >&2
    exit 1
  fi

  go_image=$(awk '/^FROM / && /go-toolset/ { print $2; exit }' "${dockerfile}")
  ubi_image=$(awk '/^FROM / && /ubi10\/ubi:/ { print $2; exit }' "${dockerfile}")

  if [[ -z "${go_image}" || -z "${ubi_image}" ]]; then
    echo "Error: could not parse builder base images from ${dockerfile}" >&2
    exit 1
  fi

  builder_container_uid=$(hermetic_container_uid "${go_image}")
  builder_host_uid=$(hermetic_host_uid_for_container "${builder_container_uid}")
  root_host_uid=$(hermetic_probe_rootless_root_host_uid "${ubi_image}" "${local_cache_dir}")

  printf '%s\n%s\n' "${builder_host_uid}" "${root_host_uid}" | sort -un
}

#######################################
# Make prefetched cache usable for offline build without world-writable perms.
#######################################
prepare_cache_for_build() {
  local local_cache_dir="$1"
  local component_dir="$2"
  local uid gid parent_dir host_uid

  uid="$(id -u)"
  gid="$(id -g)"
  parent_dir="$(dirname "${local_cache_dir}")"

  # Hermeto runs in Podman and may leave root-owned files on the host mount.
  if podman unshare chown -R "${uid}:${gid}" "${local_cache_dir}" 2>/dev/null; then
    :
  elif chown -R "${uid}:${gid}" "${local_cache_dir}" 2>/dev/null; then
    :
  elif command -v sudo &>/dev/null && sudo chown -R "${uid}:${gid}" "${local_cache_dir}"; then
    :
  else
    echo "Error: could not take ownership of ${local_cache_dir} (try: podman unshare chown -R ${uid}:${gid} ...)" >&2
    exit 1
  fi

  # Traverse-only for non-owners so Podman-mapped UIDs can reach ACL-protected cache paths.
  if [[ -d "${parent_dir}" ]] && [[ "$(stat -c '%u' "${parent_dir}")" == "${uid}" ]]; then
    chmod 711 "${parent_dir}"
  fi

  if ! command -v setfacl &>/dev/null; then
    echo "Error: setfacl is required for hermetic builds (install the acl package)" >&2
    exit 1
  fi

  # Base mode: owner only. Podman build UIDs are granted explicitly below (do not chmod after setfacl).
  chmod -R "u+rwX,g-rwx,o-rwx" "${local_cache_dir}"

  while IFS= read -r host_uid; do
    setfacl -R -m "u:${host_uid}:rwX" "${local_cache_dir}"
    setfacl -R -d -m "u:${host_uid}:rwX" "${local_cache_dir}"
  done < <(hermetic_build_host_uids "${component_dir}" "${local_cache_dir}")
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

  prepare_cache_for_build "${local_cache_dir}" "${component_dir}"
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

  prepare_cache_for_build "${local_cache_dir}" "${component_dir}"

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

  local resolved_component_dir
  local local_cache_dir
  local local_cache_output_dir

  resolved_component_dir="$(realpath "${component_dir}")"
  local_cache_dir="${LOCAL_CACHE_DIR}"
  local_cache_output_dir="${local_cache_dir}/output"
  mkdir -p "${local_cache_output_dir}"

  if [[ -z "${HERMETO_IMAGE}" ]]; then
    HERMETO_IMAGE=$(sed -n 's/^HERMETO_IMAGE ?= //p' "${resolved_component_dir}/Makefile" | head -1)
  fi
  if [[ -z "${HERMETO_IMAGE}" ]]; then
    echo "Error: set HERMETO_IMAGE or define it in ${resolved_component_dir}/Makefile" >&2
    exit 1
  fi

  echo "Component dir: ${resolved_component_dir}"
  echo "Local cache dir: ${local_cache_dir}"
  echo "Hermeto image: ${HERMETO_IMAGE}"

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
