#! /bin/bash

set -e

function objdump() {
	export PATH=/home/mozhonghua/go/src/github.com/golang/go/bin:$PATH
	export GOROOT=/home/mozhonghua/go/src/github.com/golang/go

	go build -o ../pkg/tool/linux_amd64/objdump ./cmd/objdump/

	../pkg/tool/linux_amd64/objdump "$@"
}

SELF_DIR=$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")

(
	cd $SELF_DIR && objdump "$@"
)
