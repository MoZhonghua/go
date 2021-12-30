#! /bin/bash

set -e

function link() {
	export PATH=/data/go/bin:$PATH
	export GOROOT=/data/go

	go build -o ../pkg/tool/linux_amd64/pack ./cmd/pack/

	../pkg/tool/linux_amd64/pack "$@"
}

SELF_DIR=$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")

(
	cd $SELF_DIR && link -V
)
