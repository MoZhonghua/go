#! /bin/bash

set -e

function asm() {
	export PATH=/home/mozhonghua/go/src/github.com/golang/go/bin:$PATH
	export GOROOT=/home/mozhonghua/go/src/github.com/golang/go

	go build -o ../pkg/tool/linux_amd64/asm2 ./cmd/asm/

	../pkg/tool/linux_amd64/asm2 "$@"
}

SELF_DIR=$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")

(
	cd $SELF_DIR && asm -V
)
