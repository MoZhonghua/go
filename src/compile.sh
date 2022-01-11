#! /bin/bash

set -e

function buildgo() {
	export PATH=/data/go/bin:$PATH
	export GOROOT=/data/go

	cp ../pkg/tool/linux_amd64/{compile.old,compile}
	go build -o ../pkg/tool/linux_amd64/compile2 ./cmd/compile/
	mv -f ../pkg/tool/linux_amd64/{compile2,compile}

	echo "------build compile ok-------------"

	../pkg/tool/linux_amd64/compile "$@"
}

SELF_DIR=$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")

(
	cd $SELF_DIR && buildgo version
)
