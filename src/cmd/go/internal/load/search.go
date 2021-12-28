// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package load

import (
	"path/filepath"
	"strings"

	"cmd/go/internal/search"
)

// MatchPackage(pattern, cwd)(p) reports whether package p matches pattern in the working directory cwd.
// 注意不同的模式匹配的部分不一样:
//  - relative path: ./, ../ 是匹配package.Dir在cwd中的相对路径
//  - meta-package: 检查package.IsStandard来匹配
//  - 其他: 匹配package.ImportPath，而不是package.Dir
func MatchPackage(pattern, cwd string) func(*Package) bool {
	switch {
	case search.IsRelativePath(pattern): // ./abc, ../abc, ./...
		// Split pattern into leading pattern-free directory path
		// (including all . and .. elements) and the final pattern.
		var dir string
		i := strings.Index(pattern, "...")
		if i < 0 {
			dir, pattern = pattern, ""
		} else {
			// 注意上面已经判断是./..., 因此j一定>0
			j := strings.LastIndex(pattern[:i], "/")
			dir, pattern = pattern[:j], pattern[j+1:]
		}
		dir = filepath.Join(cwd, dir)
		if pattern == "" {
			return func(p *Package) bool { return p.Dir == dir }
		}
		matchPath := search.MatchPattern(pattern) // 把包含...的pattern转换为正则表达式，同时会特别处理vendor
		return func(p *Package) bool {
			// Compute relative path to dir and see if it matches the pattern.
			rel, err := filepath.Rel(dir, p.Dir)
			if err != nil {
				// Cannot make relative - e.g. different drive letters on Windows.
				return false
			}
			rel = filepath.ToSlash(rel)
			if rel == ".." || strings.HasPrefix(rel, "../") {
				return false
			}
			// 这里必须用相对路径和pattern做匹配
			return matchPath(rel)
		}
	case pattern == "all":
		return func(p *Package) bool { return true }
	case pattern == "std":
		return func(p *Package) bool { return p.Standard }
	case pattern == "cmd":
		return func(p *Package) bool { return p.Standard && strings.HasPrefix(p.ImportPath, "cmd/") }
	default:
		// /dir/dir2/abc
		// abc/xyz
		// abc/...
		matchPath := search.MatchPattern(pattern)
		return func(p *Package) bool { return matchPath(p.ImportPath) }
	}
}
