#! /bin/bash

set -e

function dist() {
	env GO111MODULE=off GOROOT="" /usr/bin/go build -o ./cmd/dist/dist ./cmd/dist/

	echo "build dist ok"
	unset GOBIN
	unset GOPATH
	export GOENV=off
	export GOROOT=/home/mozhonghua/go/src/github.com/golang/go
	export GOROOT_BOOTSTRAP=/usr/lib/go
	export GO_LDSO=/lib64/ld-linux-x86-64.so.2
	#export GOMAXPROCS=1
	export PATH=/home/mozhonghua/go/src/github.com/golang/go/bin:$PATH

	./cmd/dist/dist "$@"
}

SELF_DIR=$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")

(
	cd $SELF_DIR && dist "$@"
)
