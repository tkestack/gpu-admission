#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

ROOT=$(cd $(dirname "${BASH_SOURCE}")/.. && pwd -P)
source $ROOT/hack/lib/lib.sh

echo $(find_files) | xargs -n 1 -I {} goimports -w {}
