#! /bin/bash

set -e

function asm() {
	export PATH=/home/mozhonghua/go/src/github.com/golang/go/bin:$PATH
	export GOROOT=/home/mozhonghua/go/src/github.com/golang/go

	cp ../pkg/tool/linux_amd64/asm.old ../pkg/tool/linux_amd64/asm

	go build -o ../pkg/tool/linux_amd64/asm ./cmd/asm/

	../pkg/tool/linux_amd64/asm "$@"
}

SELF_DIR=$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")

(
	cd $SELF_DIR && asm -V
)
