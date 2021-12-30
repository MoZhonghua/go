#! /bin/bash

set -e

function objdump() {
	export PATH=/data/go/bin:$PATH
	export GOROOT=/data/go

	go build -o ../pkg/tool/linux_amd64/objdump ./cmd/objdump/

	../pkg/tool/linux_amd64/objdump "$@"
}

SELF_DIR=$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")

(
	cd $SELF_DIR && objdump "$@"
)
