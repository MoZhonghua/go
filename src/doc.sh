#! /bin/bash

set -e

function gogo() {
	export PATH=/data/go/bin:$PATH
	export GOROOT=/data/go

	go build -o ../pkg/tool/linux_amd64/doc ./cmd/doc/

	echo "----build doc ok-------"
}

SELF_DIR=$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")

(
	cd $SELF_DIR && gogo "$@"
)
