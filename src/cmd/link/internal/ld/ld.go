// Derived from Inferno utils/6l/obj.c and utils/6l/span.c
// https://bitbucket.org/inferno-os/inferno-os/src/master/utils/6l/obj.c
// https://bitbucket.org/inferno-os/inferno-os/src/master/utils/6l/span.c
//
//	Copyright © 1994-1999 Lucent Technologies Inc.  All rights reserved.
//	Portions Copyright © 1995-1997 C H Forsyth (forsyth@terzarima.net)
//	Portions Copyright © 1997-1999 Vita Nuova Limited
//	Portions Copyright © 2000-2007 Vita Nuova Holdings Limited (www.vitanuova.com)
//	Portions Copyright © 2004,2006 Bruce Ellis
//	Portions Copyright © 2005-2007 C H Forsyth (forsyth@terzarima.net)
//	Revisions Copyright © 2000-2007 Lucent Technologies Inc. and others
//	Portions Copyright © 2009 The Go Authors. All rights reserved.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.  IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package ld

import (
	"cmd/internal/goobj"
	"cmd/link/internal/loader"
	"cmd/link/internal/sym"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// go build -x -work . 可以看到生成importcfg文件
// -importcfg $WORK/b001/importcfg.link
// 文件内容如下:
/*
# import config
packagefile fmt=/home/mozhonghua/.cache/go-build/10/10e9e7f2882559931ee7d7cf8ab8cbaea3a343281f914064ae3738fc377399ec-d
packagefile io/ioutil=/home/mozhonghua/.cache/go-build/d1/d1481b0f5b92037ee7a1e0384d095472d5ee4115675ef1eb47f75436f288d034-d
*/
func (ctxt *Link) readImportCfg(file string) {
	ctxt.PackageFile = make(map[string]string)
	ctxt.PackageShlib = make(map[string]string)
	data, err := ioutil.ReadFile(file)
	if err != nil {
		log.Fatalf("-importcfg: %v", err)
	}

	for lineNum, line := range strings.Split(string(data), "\n") {
		lineNum++ // 1-based
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		var verb, args string
		if i := strings.Index(line, " "); i < 0 {
			verb = line
		} else {
			verb, args = line[:i], strings.TrimSpace(line[i+1:])
		}
		var before, after string
		if i := strings.Index(args, "="); i >= 0 {
			before, after = args[:i], args[i+1:]
		}
		switch verb {
		default:
			log.Fatalf("%s:%d: unknown directive %q", file, lineNum, verb)
		case "packagefile":
			if before == "" || after == "" {
				log.Fatalf(`%s:%d: invalid packagefile: syntax is "packagefile path=filename"`, file, lineNum)
			}
			ctxt.PackageFile[before] = after
		case "packageshlib":
			if before == "" || after == "" {
				log.Fatalf(`%s:%d: invalid packageshlib: syntax is "packageshlib path=filename"`, file, lineNum)
			}
			ctxt.PackageShlib[before] = after
		}
	}
}

// runtime => runtime
func pkgname(ctxt *Link, lib string) string {
	name := path.Clean(lib)

	// When using importcfg, we have the final package name.
	if ctxt.PackageFile != nil {
		return name
	}

	// runtime.a -> runtime, runtime.6 -> runtime
	pkg := name
	if len(pkg) >= 2 && pkg[len(pkg)-2] == '.' {
		pkg = pkg[:len(pkg)-2]
	}
	return pkg
}

// math/bits => ($WORK/b019/_pkg_.a, false)
// b019是按照顺序分配的临时变量，每个package一个?
func findlib(ctxt *Link, lib string) (string, bool) {
	name := path.Clean(lib)

	var pname string
	isshlib := false

	if ctxt.linkShared && ctxt.PackageShlib[name] != "" {
		pname = ctxt.PackageShlib[name]
		isshlib = true
	} else if ctxt.PackageFile != nil {
		pname = ctxt.PackageFile[name]
		if pname == "" {
			ctxt.Logf("cannot find package %s (using -importcfg)\n", name)
			return "", false
		}
	} else {
		if filepath.IsAbs(name) {
			pname = name
		} else {
			pkg := pkgname(ctxt, lib)
			// Add .a if needed; the new -importcfg modes
			// do not put .a into the package name anymore.
			// This only matters when people try to mix
			// compiles using -importcfg with links not using -importcfg,
			// such as when running quick things like
			// 'go tool compile x.go && go tool link x.o'
			// by hand against a standard library built using -importcfg.
			if !strings.HasSuffix(name, ".a") && !strings.HasSuffix(name, ".o") {
				name += ".a"
			}
			// try dot, -L "libdir", and then goroot.
			for _, dir := range ctxt.Libdir {
				if ctxt.linkShared {
					pname = filepath.Join(dir, pkg+".shlibname")
					if _, err := os.Stat(pname); err == nil {
						isshlib = true
						break
					}
				}
				pname = filepath.Join(dir, name)
				if _, err := os.Stat(pname); err == nil {
					break
				}
			}
		}
		pname = filepath.Clean(pname)
	}

	return pname, isshlib
}

func addlib(ctxt *Link, src, obj, lib string, fingerprint goobj.FingerprintType) *sym.Library {
	// src=internal/poll
	// obj=/tmp/go-build901049703/b030/_pkg_.a(_go_.o)
	// lib=sync/atomic
	// 同一个src调用多次，每个依赖lib调用一次
	pkg := pkgname(ctxt, lib)

	// fingerprint是编译src时计算出的依赖项的fingerprint，和已经加载进来的对比
	// already loaded?
	if l := ctxt.LibraryByPkg[pkg]; l != nil && !l.Fingerprint.IsZero() {
		// Normally, packages are loaded in dependency order, and if l != nil
		// l is already loaded with the actual fingerprint. In shared build mode,
		// however, packages may be added not in dependency order, and it is
		// possible that l's fingerprint is not yet loaded -- exclude it in
		// checking.
		checkFingerprint(l, l.Fingerprint, src, fingerprint)
		return l
	}

	pname, isshlib := findlib(ctxt, lib)

	if ctxt.Debugvlog > 1 {
		ctxt.Logf("addlib: %s %s pulls in %s isshlib %v\n", obj, src, pname, isshlib)
	}

	if isshlib {
		return addlibpath(ctxt, src, obj, "", pkg, pname, fingerprint)
	}
	return addlibpath(ctxt, src, obj, pname, pkg, "", fingerprint)
}

/*
 * add library to library list, return added library.
 *	srcref: src file referring to package
 *	objref: object file referring to package
 *	file: object file, e.g., /home/rsc/go/pkg/container/vector.a
 *	pkg: package import path, e.g. container/vector
 *	shlib: path to shared library, or .shlibname file holding path
 *	fingerprint: if not 0, expected fingerprint for import from srcref
 *	             fingerprint is 0 if the library is not imported (e.g. main)
 */
 // 只记录，还没真正读取.a/.so文件
func addlibpath(ctxt *Link, srcref, objref, file, pkg, shlib string, fingerprint goobj.FingerprintType) *sym.Library {
	// file是.a文件路径，一般是通过-importcfg文件指定
	// 被多个package引用时只会添加一次，Library.Objref/Srcref是第一个引用者
	if l := ctxt.LibraryByPkg[pkg]; l != nil {
		return l
	}

	if ctxt.Debugvlog > 1 {
		ctxt.Logf("addlibpath: srcref: %s objref: %s file: %s pkg: %s shlib: %s fingerprint: %x\n", srcref, objref, file, pkg, shlib, fingerprint)
	}

	l := &sym.Library{}
	ctxt.LibraryByPkg[pkg] = l
	ctxt.Library = append(ctxt.Library, l)
	l.Objref = objref
	l.Srcref = srcref
	l.File = file
	l.Pkg = pkg
	l.Fingerprint = fingerprint
	if shlib != "" {
		if strings.HasSuffix(shlib, ".shlibname") {
			data, err := ioutil.ReadFile(shlib)
			if err != nil {
				Errorf(nil, "cannot read %s: %v", shlib, err)
			}
			shlib = strings.TrimSpace(string(data))
		}
		l.Shlib = shlib
	}
	return l
}

func atolwhex(s string) int64 {
	n, _ := strconv.ParseInt(s, 0, 64)
	return n
}

// PrepareAddmoduledata returns a symbol builder that target-specific
// code can use to build up the linker-generated go.link.addmoduledata
// function, along with the sym for runtime.addmoduledata itself. If
// this function is not needed (for example in cases where we're
// linking a module that contains the runtime) the returned builder
// will be nil.
func PrepareAddmoduledata(ctxt *Link) (*loader.SymbolBuilder, loader.Sym) {
	// 只有生成结果是DSO时才需要处理，普通的.a和exe不需要initfunc
	if !ctxt.DynlinkingGo() {
		return nil, 0
	}

	// runtime.addmoduledata是一个汇编函数
	amd := ctxt.loader.LookupOrCreateSym("runtime.addmoduledata", 0)
	if ctxt.loader.SymType(amd) == sym.STEXT && ctxt.BuildMode != BuildModePlugin {
		// we're linking a module containing the runtime -> no need for
		// an init function
		return nil, 0
	}
	ctxt.loader.SetAttrReachable(amd, true)

	// Create a new init func text symbol. Caller will populate this
	// sym with arch-specific content.
	ifs := ctxt.loader.LookupOrCreateSym("go.link.addmoduledata", 0)
	initfunc := ctxt.loader.MakeSymbolUpdater(ifs)
	ctxt.loader.SetAttrReachable(ifs, true)
	ctxt.loader.SetAttrLocal(ifs, true)
	initfunc.SetType(sym.STEXT)

	// Add the init func and/or addmoduledata to Textp.
	if ctxt.BuildMode == BuildModePlugin {
		ctxt.Textp = append(ctxt.Textp, amd)
	}
	ctxt.Textp = append(ctxt.Textp, initfunc.Sym())

	// Create an init array entry
	// 这一项指向上面刚创建的initfunc，loader加载时会自动调用initfunc函数
	amdi := ctxt.loader.LookupOrCreateSym("go.link.addmoduledatainit", 0)
	initarray_entry := ctxt.loader.MakeSymbolUpdater(amdi)
	ctxt.loader.SetAttrReachable(amdi, true)
	ctxt.loader.SetAttrLocal(amdi, true)
	initarray_entry.SetType(sym.SINITARR)
	initarray_entry.AddAddr(ctxt.Arch, ifs)

	// initfunc的函数体由调用者填写。一般是调用amd指向的runtime.addmoduledata()函数
	// 注意DI应该是指向新加moduledata的指针
	return initfunc, amd
}
