// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package types

import (
	"cmd/compile/internal/base"
	"cmd/internal/src"
	"fmt"
	"strings"
)

// Declaration stack & operations

var blockgen int32 = 1 // max block number
var Block int32 = 1    // current block number
var blockIdent = 0

const debugScope = false

// A dsym stores a symbol's shadowed declaration so that it can be
// restored once the block scope ends.
type dsym struct {
	sym        *Sym // sym == nil indicates stack mark
	def        Object
	block      int32
	lastlineno src.XPos // last declaration for diagnostic
}

// dclstack maintains a stack of shadowed symbol declarations so that
// Popdcl can restore their declarations when a block scope ends.
var dclstack []dsym

// Pushdcl pushes the current declaration for symbol s (if any) so that
// it can be shadowed by a new declaration within a nested block scope.
func Pushdcl(s *Sym) {
	if debugScope {
		fmt.Printf("%sPushdcl: %v(pkg=%v)\n", strings.Repeat("  ", blockIdent), s.Name, s.Pkg.Path)
	}
	dclstack = append(dclstack, dsym{
		sym:        s,
		def:        s.Def,
		block:      s.Block,
		lastlineno: s.Lastlineno,
	})
}

// Popdcl pops the innermost block scope and restores all symbol declarations
// to their previous state.
func Popdcl() {
	if debugScope {
		fmt.Printf("%sPopdcl\n", strings.Repeat("  ", blockIdent))
	}
	for i := len(dclstack); i > 0; i-- {
		d := &dclstack[i-1]
		s := d.sym
		if s == nil {
			// pop stack mark
			Block = d.block
			dclstack = dclstack[:i-1]
			blockIdent--
			return
		}

		// 还原之前push的字段
		s.Def = d.def
		s.Block = d.block
		s.Lastlineno = d.lastlineno

		// Clear dead pointer fields.
		d.sym = nil
		d.def = nil
	}
	base.Fatalf("popdcl: no stack mark")
}

// Markdcl records the start of a new block scope for declarations.
func Markdcl() {
	if debugScope {
		fmt.Printf("%sMarkdcl\n", strings.Repeat("  ", blockIdent))
	}
	dclstack = append(dclstack, dsym{
		sym:   nil, // stack mark
		block: Block,
	})
	blockgen++
	Block = blockgen
	blockIdent++
}

// 全部处理完成后不应该有stack mark
func isDclstackValid() bool {
	for _, d := range dclstack {
		if d.sym == nil {
			return false
		}
	}
	return true
}

// PkgDef returns the definition associated with s at package scope.
func (s *Sym) PkgDef() Object {
	return *s.pkgDefPtr()
}

// SetPkgDef sets the definition associated with s at package scope.
func (s *Sym) SetPkgDef(n Object) {
	*s.pkgDefPtr() = n
}

func (s *Sym) pkgDefPtr() *Object {
	// Look for outermost saved declaration, which must be the
	// package scope definition, if present.
	for i := range dclstack {
		d := &dclstack[i]
		if s == d.sym {
			return &d.def
		}
	}

	// Otherwise, the declaration hasn't been shadowed within a
	// function scope.
	return &s.Def
}

func CheckDclstack() {
	if !isDclstackValid() {
		base.Fatalf("mark left on the dclstack")
	}
}
