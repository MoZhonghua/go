// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package objw

import (
	"cmd/compile/internal/base"
	"cmd/compile/internal/bitvec"
	"cmd/compile/internal/types"
	"cmd/internal/obj"
)

// Uint8 writes an unsigned byte v into s at offset off,
// and returns the next unused offset (i.e., off+1).
func Uint8(s *obj.LSym, off int, v uint8) int {
	return UintN(s, off, uint64(v), 1)
}

func Uint16(s *obj.LSym, off int, v uint16) int {
	return UintN(s, off, uint64(v), 2)
}

func Uint32(s *obj.LSym, off int, v uint32) int {
	return UintN(s, off, uint64(v), 4)
}

func Uintptr(s *obj.LSym, off int, v uint64) int {
	return UintN(s, off, v, types.PtrSize)
}

// UintN writes an unsigned integer v of size wid bytes into s at offset off,
// and returns the next unused offset.
//
// s.P[off:off+wid] = v
func UintN(s *obj.LSym, off int, v uint64, wid int) int {
	if off&(wid-1) != 0 {
		base.Fatalf("duintxxLSym: misaligned: v=%d wid=%d off=%d", v, wid, off)
	}

	s.WriteInt(base.Ctxt, int64(off), wid, int64(v))
	return off + wid
}

// s.P写入一个对x的指针，需要重定向(在s.R列表中添加一项)
// 在确定好每个LSym的位置(addr)后根据s.R重写地址
func SymPtr(s *obj.LSym, off int, x *obj.LSym, xoff int) int {
	off = int(types.Rnd(int64(off), int64(types.PtrSize)))
	s.WriteAddr(base.Ctxt, int64(off), types.PtrSize, x, int64(xoff))
	off += types.PtrSize
	return off
}

// weak两个含义:
//  - s -> x 的引用不会标记x为live，deadcode elimination可以删除x
//  - 如果x被删除，不需要重定向这个指针
func SymPtrWeak(s *obj.LSym, off int, x *obj.LSym, xoff int) int {
	off = int(types.Rnd(int64(off), int64(types.PtrSize)))
	s.WriteWeakAddr(base.Ctxt, int64(off), types.PtrSize, x, int64(xoff))
	off += types.PtrSize
	return off
}

// 最终重定向结果为x.Addr + off
func SymPtrOff(s *obj.LSym, off int, x *obj.LSym) int {
	s.WriteOff(base.Ctxt, int64(off), x, 0)
	off += 4
	return off
}

func SymPtrWeakOff(s *obj.LSym, off int, x *obj.LSym) int {
	s.WriteWeakOff(base.Ctxt, int64(off), x, 0)
	off += 4
	return off
}

// s放到base.Ctxt.Data []*LSym列表中, 最终出现在.data/.rodata section?
func Global(s *obj.LSym, width int32, flags int16) {
	if flags&obj.LOCAL != 0 {
		s.Set(obj.AttrLocal, true)
		flags &^= obj.LOCAL
	}
	base.Ctxt.Globl(s, int64(width), int(flags))
}

// Bitvec writes the contents of bv into s as sequence of bytes
// in little-endian order, and returns the next unused offset.
func BitVec(s *obj.LSym, off int, bv bitvec.BitVec) int {
	// Runtime reads the bitmaps as byte arrays. Oblige.
	for j := 0; int32(j) < bv.N; j += 8 {
		word := bv.B[j/32]
		off = Uint8(s, off, uint8(word>>(uint(j)%32)))
	}
	return off
}
