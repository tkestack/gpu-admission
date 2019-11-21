#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

ROOT=$(cd $(dirname "${BASH_SOURCE}")/.. && pwd -P)

# verify gofmt
${ROOT}/hack/verify-gofmt.sh
