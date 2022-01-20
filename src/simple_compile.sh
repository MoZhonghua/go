#! /bin/bash

set -e

function buildgo() {
	export PATH=/data/go/bin:$PATH
	export GOROOT=/data/go

	cp ../pkg/tool/linux_amd64/{compile.old,compile}
	go build -o ../pkg/tool/linux_amd64/scompile ./cmd/compile/internal/simplegc/

	echo "------build simple compile ok-------------"

	../pkg/tool/linux_amd64/scompile "$@"
}

SELF_DIR=$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")

(
	cd $SELF_DIR && buildgo -V
)
