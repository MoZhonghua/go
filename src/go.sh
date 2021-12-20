#! /bin/bash

set -e

function buildgo() {
	export PATH=/home/mozhonghua/go/src/github.com/golang/go/bin:$PATH
	export GOROOT=/home/mozhonghua/go/src/github.com/golang/go

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
