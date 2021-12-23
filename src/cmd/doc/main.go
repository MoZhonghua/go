// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Doc (usually run as go doc) accepts zero, one or two arguments.
//
// Zero arguments:
//	go doc
// Show the documentation for the package in the current directory.
//
// One argument:
//	go doc <pkg>
//	go doc <sym>[.<methodOrField>]
//	go doc [<pkg>.]<sym>[.<methodOrField>]
//	go doc [<pkg>.][<sym>.]<methodOrField>
// The first item in this list that succeeds is the one whose documentation
// is printed. If there is a symbol but no package, the package in the current
// directory is chosen. However, if the argument begins with a capital
// letter it is always assumed to be a symbol in the current directory.
//
// Two arguments:
//	go doc <pkg> <sym>[.<methodOrField>]
//
// Show the documentation for the package, symbol, and method or field. The
// first argument must be a full package path. This is similar to the
// command-line usage for the godoc command.
//
// For commands, unless the -cmd flag is present "go doc command"
// shows only the package-level docs for the package.
//
// The -src flag causes doc to print the full source code for the symbol, such
// as the body of a struct, function or method.
//
// The -all flag causes doc to print all documentation for the package and
// all its visible symbols. The argument must identify a package.
//
// For complete documentation, run "go help doc".
package main

// 特别注意这里的两个Import，一个是Import, 另一个是ImportDir, 用的是go/build包
//  - build.Import("x", "/abs/y"): 等价于在路径"/abs/y"下的一个.go文件中有import "x"语句
//  - build.ImportDir("/abs/x"): 等价于如果要导入"/abs/x"下的包，应该怎么写import语句

import (
	"bytes"
	"flag"
	"fmt"
	"go/build"
	"go/token"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

var (
	unexported bool // -u flag
	matchCase  bool // -c flag
	showAll    bool // -all flag
	showCmd    bool // -cmd flag
	showSrc    bool // -src flag
	short      bool // -short flag
	debug      bool // -d flag
)

// usage is a replacement usage function for the flags package.
func usage() {
	fmt.Fprintf(os.Stderr, "Usage of [go] doc:\n")
	fmt.Fprintf(os.Stderr, "\tgo doc\n")
	fmt.Fprintf(os.Stderr, "\tgo doc <pkg>\n")
	fmt.Fprintf(os.Stderr, "\tgo doc <sym>[.<methodOrField>]\n")
	fmt.Fprintf(os.Stderr, "\tgo doc [<pkg>.]<sym>[.<methodOrField>]\n")
	fmt.Fprintf(os.Stderr, "\tgo doc [<pkg>.][<sym>.]<methodOrField>\n")
	fmt.Fprintf(os.Stderr, "\tgo doc <pkg> <sym>[.<methodOrField>]\n")
	fmt.Fprintf(os.Stderr, "For more information run\n")
	fmt.Fprintf(os.Stderr, "\tgo help doc\n\n")
	fmt.Fprintf(os.Stderr, "Flags:\n")
	flag.PrintDefaults()
	os.Exit(2)
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("doc: ")
	// 在处理参数之前，计算出扫描的根目录列表
	dirsInit()

	err := do(os.Stdout, flag.CommandLine, os.Args[1:])
	if err != nil {
		log.Fatal(err)
	}
}

// do is the workhorse, broken out of main to make testing easier.
func do(writer io.Writer, flagSet *flag.FlagSet, args []string) (err error) {
	flagSet.Usage = usage
	unexported = false
	matchCase = false
	flagSet.BoolVar(&unexported, "u", false, "show unexported symbols as well as exported")
	flagSet.BoolVar(&matchCase, "c", false, "symbol matching honors case (paths not affected)")
	flagSet.BoolVar(&showAll, "all", false, "show all documentation for package")
	flagSet.BoolVar(&showCmd, "cmd", false, "show symbols with package docs even if package is a command")
	flagSet.BoolVar(&showSrc, "src", false, "show source code for symbol")
	flagSet.BoolVar(&short, "short", false, "one-line representation for each symbol")
	flagSet.BoolVar(&debug, "d", false, "debug output")
	flagSet.Parse(args)

	if debug {
		mainMod, vendorEnabled, _ := vendorEnabled()
		fmt.Printf("mainMod=%+v\n", mainMod)
		fmt.Printf("vendorEnabled=%v\n", vendorEnabled)
		fmt.Printf("usingModules=%v\n", usingModules)
		for _, r := range codeRoots() {
			fmt.Printf("root: %v\n", r.dir)
		}
	}

	var paths []string
	var symbol, method string
	// Loop until something is printed.
	dirs.Reset()

	// 遍历所有可能的pacakge: 名字符合参数且可以build.Import或者build.ImportDir的package
	// 然后根据参数输出all/pkg/sym/method doc，如果有输出，则完成。否则尝试下一个package
	for i := 0; ; i++ {
		buildPackage, userPath, sym, more := parseArgs(flagSet.Args())
		if i > 0 && !more { // Ignore the "more" bit on the first iteration.
			return failMessage(paths, symbol, method)
		}
		if buildPackage == nil {
			return fmt.Errorf("no such package: %s", userPath)
		}
		symbol, method = parseSymbol(sym) // sym可能为空，此时是匹配所有

		// userPath是指用户参数中当做pkg的部分，比如go doc template.Template
		// buildPackage.Dir=html/template
		// userPath=template
		pkg := parsePackage(writer, buildPackage, userPath)

		paths = append(paths, pkg.prettyPath())

		defer func() {
			pkg.flush()
			e := recover()
			if e == nil {
				return
			}
			pkgError, ok := e.(PackageError)
			if ok {
				err = pkgError
				return
			}
			panic(e)
		}()

		// The builtin package needs special treatment: its symbols are lower
		// case but we want to see them, always.
		if pkg.build.ImportPath == "builtin" {
			unexported = true
		}

		// We have a package.
		if showAll && symbol == "" {
			pkg.allDoc()
			return
		}

		switch {
		case symbol == "":
			// go doc json
			// <documentation>
			// ....
			// package xzy
			// 输出xyz的全局文档<documentation>，同时打印所有exported类型、变量、函数(不带文档)
			pkg.packageDoc() // The package exists, so we got some output.
			return
		case method == "":
			// go doc json.Number
			// 输出Number类型文档，同时列出所有成员函数列表
			if pkg.symbolDoc(symbol) {
				return
			}
		default:
			// go doc json.Number.Int64
			// 输出func (n Number) Int64()文档
			if pkg.methodDoc(symbol, method) {
				return
			}
			// go doc url.URL.Path
			if pkg.fieldDoc(symbol, method) {
				return
			}
		}
	}
}

// failMessage creates a nicely formatted error message when there is no result to show.
func failMessage(paths []string, symbol, method string) error {
	var b bytes.Buffer
	if len(paths) > 1 {
		b.WriteString("s")
	}
	b.WriteString(" ")
	for i, path := range paths {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(path)
	}
	if method == "" {
		return fmt.Errorf("no symbol %s in package%s", symbol, &b)
	}
	return fmt.Errorf("no method or field %s.%s in package%s", symbol, method, &b)
}

// 如果有pkg，比如:
//  - go doc pkg Foo
//  - go doc pkg.Foo
//  - go doc pkg.Foo.Bar
// * Import("pkg", PWD)，如果成功则用这个pkg，注意此时总是more=false
// * BFS所有根目录，如果找到<dir>/pkg这样目录，ImportDir("<dir>/pkg")，如果成功则返回这个
//   pkg注意此时总是more=true

// 如果没有pkg:
// - go doc Foo: ImportDir(".")
// - go doc: ImportDir("<PWD>")

// 如果参数中没有"/", 则最后兜底的是ImportDir("<PWD>"), 否则直接报错

// parseArgs analyzes the arguments (if any) and returns the package
// it represents, the part of the argument the user used to identify
// the path (or "" if it's the current package) and the symbol
// (possibly with a .method) within that package.
// parseSymbol is used to analyze the symbol itself.
// The boolean final argument reports whether it is possible that
// there may be more directories worth looking at. It will only
// be true if the package path is a partial match for some directory
// and there may be more matches. For example, if the argument
// is rand.Float64, we must scan both crypto/rand and math/rand
// to find the symbol, and the first call will return crypto/rand, true.
func parseArgs(args []string) (pkg *build.Package, path, symbol string, more bool) {
	wd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	if len(args) == 0 {
		// Easy: current directory.
		return importDir(wd), "", "", false
	}
	arg := args[0]
	// We have an argument. If it is a directory name beginning with . or ..,
	// use the absolute path name. This discriminates "./errors" from "errors"
	// if the current directory contains a non-standard errors package.
	if isDotSlash(arg) {
		arg = filepath.Join(wd, arg)
	}

	switch len(args) {
	default:
		usage()
	case 1:
		// Done below.
	case 2:
		// go doc xyz Foo
		// Package must be findable and importable.
		pkg, err := Import(args[0], wd, build.ImportComment)
		if err == nil {
			return pkg, args[0], args[1], false
		}
		for {
			// arg = xyz
			// 从所有扫描的路径中查找/xyz结尾的路径, findable
			packagePath, ok := findNextPackage(arg)
			if !ok {
				break
			}
			// 尝试Import路径, importable
			if pkg, err := ImportDir(packagePath, build.ImportComment); err == nil {
				return pkg, arg, args[1], true // more=true，可能有多个
			}
		}
		return nil, args[0], args[1], false
	}
	// Usual case: one argument.
	// If it contains slashes, it begins with either a package path
	// or an absolute directory.
	// First, is it a complete package path as it is? If so, we are done.
	// This avoids confusion over package paths that have other
	// package paths as their prefix.
	var importErr error
	if filepath.IsAbs(arg) {
		pkg, importErr = ImportDir(arg, build.ImportComment)
		if importErr == nil {
			return pkg, arg, "", false
		}
	} else {
		pkg, importErr = Import(arg, wd, build.ImportComment)
		if importErr == nil {
			return pkg, arg, "", false
		}
	}
	// Another disambiguator: If the argument starts with an upper
	// case letter, it can only be a symbol in the current directory.
	// Kills the problem caused by case-insensitive file systems
	// matching an upper case name as a package name.
	if !strings.ContainsAny(arg, `/\`) && token.IsExported(arg) {
		// go doc Foo
		pkg, err := ImportDir(".", build.ImportComment)
		if err == nil {
			return pkg, "", arg, false
		}
	}
	// If it has a slash, it must be a package path but there is a symbol.
	// It's the last package path we care about.
	slash := strings.LastIndex(arg, "/")
	// There may be periods in the package path before or after the slash
	// and between a symbol and method.
	// Split the string at various periods to see what we find.
	// In general there may be ambiguities but this should almost always
	// work.
	var period int
	// slash+1: if there's no slash, the value is -1 and start is 0; otherwise
	// start is the byte after the slash.

	for start := slash + 1; start < len(arg); start = period + 1 {
		// go doc xyz.Foo
		// go doc abc/xyz.Foo
		// go doc json.Number.Int64
		period = strings.Index(arg[start:], ".")
		symbol := ""
		if period < 0 {
			period = len(arg)
		} else {
			period += start
			symbol = arg[period+1:] // symbol=Foo
		}
		// Have we identified a package already?
		pkg, err := Import(arg[0:period], wd, build.ImportComment)
		if err == nil {
			return pkg, arg[0:period], symbol, false
		}
		// See if we have the basename or tail of a package, as in json for encoding/json
		// or ivy/value for robpike.io/ivy/value.
		pkgName := arg[:period]
		for {
			path, ok := findNextPackage(pkgName)
			if !ok {
				break
			}
			if pkg, err = ImportDir(path, build.ImportComment); err == nil {
				return pkg, arg[0:period], symbol, true
			}
		}
		dirs.Reset() // Next iteration of for loop must scan all the directories again.
	}
	// If it has a slash, we've failed.
	if slash >= 0 {
		// build.Import should always include the path in its error message,
		// and we should avoid repeating it. Unfortunately, build.Import doesn't
		// return a structured error. That can't easily be fixed, since it
		// invokes 'go list' and returns the error text from the loaded package.
		// TODO(golang.org/issue/34750): load using golang.org/x/tools/go/packages
		// instead of go/build.
		importErrStr := importErr.Error()
		if strings.Contains(importErrStr, arg[:period]) {
			log.Fatal(importErrStr)
		} else {
			log.Fatalf("no such package %s: %s", arg[:period], importErrStr)
		}
	}

	// Guess it's a symbol in the current directory.
	return importDir(wd), "", arg, false
}

// dotPaths lists all the dotted paths legal on Unix-like and
// Windows-like file systems. We check them all, as the chance
// of error is minute and even on Windows people will use ./
// sometimes.
var dotPaths = []string{
	`./`,
	`../`,
	`.\`,
	`..\`,
}

// isDotSlash reports whether the path begins with a reference
// to the local . or .. directory.
func isDotSlash(arg string) bool {
	if arg == "." || arg == ".." {
		return true
	}
	for _, dotPath := range dotPaths {
		if strings.HasPrefix(arg, dotPath) {
			return true
		}
	}
	return false
}

// importDir is just an error-catching wrapper for build.ImportDir.
func importDir(dir string) *build.Package {
	pkg, err := ImportDir(dir, build.ImportComment)
	if err != nil {
		log.Fatal(err)
	}
	return pkg
}

// parseSymbol breaks str apart into a symbol and method.
// Both may be missing or the method may be missing.
// If present, each must be a valid Go identifier.
func parseSymbol(str string) (symbol, method string) {
	if str == "" {
		return
	}
	elem := strings.Split(str, ".")
	switch len(elem) {
	case 1:
	case 2:
		method = elem[1]
	default:
		log.Printf("too many periods in symbol specification")
		usage()
	}
	symbol = elem[0]
	return
}

// isExported reports whether the name is an exported identifier.
// If the unexported flag (-u) is true, isExported returns true because
// it means that we treat the name as if it is exported.
func isExported(name string) bool {
	return unexported || token.IsExported(name)
}

// findNextPackage returns the next full file name path that matches the
// (perhaps partial) package path pkg. The boolean reports if any match was found.
func findNextPackage(pkg string) (string, bool) {
	if filepath.IsAbs(pkg) {
		if dirs.offset == 0 {
			dirs.offset = -1
			return pkg, true
		}
		return "", false
	}
	if pkg == "" || token.IsExported(pkg) { // Upper case symbol cannot be a package name.
		return "", false
	}
	pkg = path.Clean(pkg)
	pkgSuffix := "/" + pkg
	for {
		d, ok := dirs.Next()
		if !ok {
			return "", false
		}
		if d.importPath == pkg || strings.HasSuffix(d.importPath, pkgSuffix) {
			return d.dir, true
		}
	}
}

var buildCtx = build.Default

// splitGopath splits $GOPATH into a list of roots.
func splitGopath() []string {
	return filepath.SplitList(buildCtx.GOPATH)
}

// Import is shorthand for Default.Import.
func Import(path, srcDir string, mode build.ImportMode) (*build.Package, error) {
	pkg, err := build.Import(path, srcDir, mode)
	if debug {
		_, file, line, _ := runtime.Caller(1)
		base := filepath.Base(file)
		fmt.Printf("try build.Import: pkg=%v, src=%v; ok=%v (%v:%v)\n", path, srcDir, err == nil, base, line)
	}
	return pkg, err
}

// ImportDir is shorthand for Default.ImportDir.
func ImportDir(dir string, mode build.ImportMode) (*build.Package, error) {
	pkg, err := build.ImportDir(dir, mode)
	if debug {
		_, file, line, _ := runtime.Caller(1)
		base := filepath.Base(file)
		fmt.Printf("try build.ImportDir: %v; ok=%v (%v:%v)\n", dir, err == nil, base, line)
	}
	return pkg, err
}
