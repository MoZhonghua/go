#! /bin/bash

set -e

env GO111MODULE=off GOROOT="" /usr/bin/go build -o dist .

echo "build dist ok"

unset GOBIN
export GOENV=off
export GOPATH=/home/mozhonghua/go
export GOROOT=/home/mozhonghua/go/src/github.com/golang/go
export GOROOT_BOOTSTRAP=/usr/lib/go
export GO_LDSO=/lib64/ld-linux-x86-64.so.2
export GOMAXPROCS=1
export PATH=/home/mozhonghua/go/src/github.com/golang/go/bin:$PATH

./dist "$@"
