#! /bin/bash

set -e

function buildgo() {
	export PATH=/data/go/bin:$PATH
	export GOROOT=/data/go

	cp ../bin/{go.old,go}
	go build -o ../bin/go2 ./cmd/go/
	mv -f ../bin/{go2,go}

	echo "------build go ok-------------"

	../bin/go "$@"
}

SELF_DIR=$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")

(
	cd $SELF_DIR && buildgo version
)
