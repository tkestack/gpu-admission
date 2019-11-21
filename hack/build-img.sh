#!/bin/bash

# RPM package deployment
set -o errexit
set -o nounset
set -o pipefail

GITCOMMIT=$(git log --oneline|wc -l|sed -e 's/^[ \t]*//')
VERSION=${VERSION:-unknown}  # MUST modify it for every branch!
ROOT=$(dirname "${BASH_SOURCE}")/..
IMAGE=${IMAGE:-"gpu-admission-${VERSION}:${GITCOMMIT}"}
GIT_VERSION_FILE="${ROOT}/.version-defs"

readonly LOCAL_OUTPUT_ROOT="${ROOT}/${OUT_DIR:-_output}"

source "${ROOT}/hack/lib/logging.sh"
source "${ROOT}/hack/lib/version.sh"

function api::build::ensure_tar() {
  if [[ -n "${TAR:-}" ]]; then
    return
  fi

  # Find gnu tar if it is available, bomb out if not.
  TAR=tar
  if which gtar &>/dev/null; then
      TAR=gtar
  else
      if which gnutar &>/dev/null; then
    TAR=gnutar
      fi
  fi
  if ! "${TAR}" --version | grep -q GNU; then
    echo "  !!! Cannot find GNU tar. Build on Linux or install GNU tar"
    echo "      on Mac OS X (brew install gnu-tar)."
    return 1
  fi
}

# The set of source targets to include in the api-build image
function api::build::source_targets() {
  local targets=(
      $(find . -mindepth 1 -maxdepth 1 -not \(        \
          \( -path ./_\* -o -path ./.git\* -o -path ./_output -o -path ./bin -o -path ./go \) -prune  \
        \))
  )
  echo "${targets[@]}"
}

function api::build::prepare_build() {
  api::build::ensure_tar || return 1

  mkdir -p "${LOCAL_OUTPUT_ROOT}"
  "${TAR}" czf "${LOCAL_OUTPUT_ROOT}/gpu-admission-source.tar.gz" --transform 's,^,/gpu-admission-'$VERSION'/,' $(api::build::source_targets)

  cp -R "${ROOT}/build/gpu-admission.spec" "${LOCAL_OUTPUT_ROOT}"
  cp "${ROOT}/Dockerfile" "${LOCAL_OUTPUT_ROOT}"
}

function api::build::generate() {
  api::log::status "Generating image..."
  (
    cd "${LOCAL_OUTPUT_ROOT}"
    docker build -t $IMAGE \
      --build-arg version=${VERSION} \
      --build-arg commit=${GITCOMMIT} \
      .
  )
}

if [[ -f ${GIT_VERSION_FILE} ]]; then
  api::version::load_version_vars "${GIT_VERSION_FILE}"
else
  api::version::get_version_vars
  api::version::save_version_vars "${ROOT}/.version-defs"
fi
api::build::prepare_build
api::build::generate

# vim: set ts=2 sw=2 tw=0 et :
