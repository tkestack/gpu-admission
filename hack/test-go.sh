#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

TIMEOUT=${TIMEOUT:-5m}

go test -v -test.timeout=${TIMEOUT} ./...
