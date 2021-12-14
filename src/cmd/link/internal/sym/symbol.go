// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sym

import (
	"cmd/internal/obj"
)

const (
	SymVerABI0        = 0
	SymVerABIInternal = 1

	// 每加载一个hostobj .o文件 ctxt.version+1，也就是分配独立的命名空间，因为查找时是<name, ver>来
	// 唯一确定sym的
	// go object的sym只能是0/1，也就是全部在同一个命名空间里
	SymVerStatic      = 10 // Minimum version used by static (file-local) syms
)

func ABIToVersion(abi obj.ABI) int {
	switch abi {
	case obj.ABI0:
		return SymVerABI0
	case obj.ABIInternal:
		return SymVerABIInternal
	}
	return -1
}

func VersionToABI(v int) (obj.ABI, bool) {
	switch v {
	case SymVerABI0:
		return obj.ABI0, true
	case SymVerABIInternal:
		return obj.ABIInternal, true
	}
	return ^obj.ABI(0), false
}
