#! /bin/bash

set -e

function buildgo() {
	export PATH=/data/go/bin:$PATH
	export GOROOT=/data/go

	go build -o ../bin/gofmt ./cmd/gofmt/

	echo "------build gofmt ok-------------"
}

SELF_DIR=$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")

(
	cd $SELF_DIR && buildgo
)
