#!/bin/bash

set -o nounset
set -o pipefail

ROOT=$(cd $(dirname "${BASH_SOURCE}")/.. && pwd -P)
source "${ROOT}/hack/lib/lib.sh"

GOIMPORT="gofmt -s -d -w"
find_files | xargs $GOIMPORT
if [ $? -ne 0 ]; then
	echo "Failed to format"
else
	echo "Format successfully"
fi
