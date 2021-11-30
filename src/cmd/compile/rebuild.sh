#! /bin/bash

GO=${1:-go}

OUT=/home/mozhonghua/go/src/github.com/golang/go/pkg/tool/linux_amd64/compile

"$GO" build -o "/home/mozhonghua/go/src/github.com/golang/go/pkg/tool/linux_amd64/compile" .
