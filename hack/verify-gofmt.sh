#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

ROOT=$(cd $(dirname "${BASH_SOURCE}")/.. && pwd -P)
source $ROOT/hack/lib/lib.sh

GOFMT="gofmt -s"
bad_files=$(find_files | xargs $GOFMT -l)
if [[ -n "${bad_files}" ]]; then
  echo "!!! '$GOFMT -w' needs to be run on the following files: "
  echo "${bad_files}"
  echo "run 'git ls-files -m | grep .go | xargs -n1 gofmt -s -w' to format your own code"
  exit 1
fi
