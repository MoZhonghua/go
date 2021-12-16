// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sym

import "cmd/internal/goobj"

// 一个package对应一个.a文件，所有.go编译为一个_go_.o, 每个.s文件编译为一个.o，打包到.a
type Library struct {
	Objref      string
	Srcref      string
	File        string
	Pkg         string
	Shlib       string
	Fingerprint goobj.FingerprintType
	Autolib     []goobj.ImportedPkg
	Imports     []*Library
	Main        bool
	Units       []*CompilationUnit

	Textp       []LoaderSym // text syms defined in this library
	// 注意是有Dupok()属性的text sym，不是指text sym本身和其他sym重复
	DupTextSyms []LoaderSym // dupok text syms defined in this library
}

func (l Library) String() string {
	return l.Pkg
}
