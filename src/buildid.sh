#! /bin/bash

set -e

function run() {
	export PATH=/data/go/bin:$PATH
	export GOROOT=/data/go

	go build -o ../pkg/tool/linux_amd64/buildid ./cmd/buildid/

	../pkg/tool/linux_amd64/buildid "$@"
}

SELF_DIR=$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")

(
	cd $SELF_DIR && run "$@"
)
