#! /bin/bash

set -e

function gogo() {
	export PATH=/home/mozhonghua/go/src/github.com/golang/go/bin:$PATH
	export GOROOT=/home/mozhonghua/go/src/github.com/golang/go

	go build -o ../pkg/tool/linux_amd64/doc ./cmd/doc/

	echo "----build doc ok-------"
}

SELF_DIR=$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")

(
	cd $SELF_DIR && gogo "$@"
)
