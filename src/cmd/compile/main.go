// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"cmd/compile/internal/amd64"
	"cmd/compile/internal/base"
	"cmd/compile/internal/gc"
	"cmd/compile/internal/ssagen"
	"fmt"
	"internal/buildcfg"
	"log"
	"os"
)

var archInits = map[string]func(*ssagen.ArchInfo){
	"amd64": amd64.Init,
	/*
		"386":      x86.Init,
		"arm":      arm.Init,
		"arm64":    arm64.Init,
		"mips":     mips.Init,
		"mipsle":   mips.Init,
		"mips64":   mips64.Init,
		"mips64le": mips64.Init,
		"ppc64":    ppc64.Init,
		"ppc64le":  ppc64.Init,
		"riscv64":  riscv64.Init,
		"s390x":    s390x.Init,
		"wasm":     wasm.Init,
	*/
}

func main() {
	// disable timestamps for reproducible output
	log.SetFlags(0)
	log.SetPrefix("compile: ")

	buildcfg.Check()
	archInit, ok := archInits[buildcfg.GOARCH]
	if !ok {
		fmt.Fprintf(os.Stderr, "compile: unknown architecture %q\n", buildcfg.GOARCH)
		os.Exit(2)
	}

	gc.Main(archInit)
	base.Exit(0)
}
