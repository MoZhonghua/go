#! /bin/bash

set -e

function link() {
	export PATH=/home/mozhonghua/go/src/github.com/golang/go/bin:$PATH
	export GOROOT=/home/mozhonghua/go/src/github.com/golang/go

	go build -o ../pkg/tool/linux_amd64/link2 ./cmd/link/

	echo "build link ok"

	../pkg/tool/linux_amd64/link2 "$@"
}

SELF_DIR=$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")

(
	cd $SELF_DIR && link -V
)
