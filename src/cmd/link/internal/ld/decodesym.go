// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ld

import (
	"cmd/internal/objabi"
	"cmd/internal/sys"
	"cmd/link/internal/loader"
	"cmd/link/internal/sym"
	"debug/elf"
	"encoding/binary"
	"log"
)

// Decoding the type.* symbols.	 This has to be in sync with
// ../../runtime/type.go, or more specifically, with what
// cmd/compile/internal/reflectdata/reflect.go stuffs in these.

// tflag is documented in reflect/type.go.
//
// tflag values must be kept in sync with copies in:
//	cmd/compile/internal/reflectdata/reflect.go
//	cmd/link/internal/ld/decodesym.go
//	reflect/type.go
//	runtime/type.go
const (
	tflagUncommon  = 1 << 0
	tflagExtraStar = 1 << 1
)

// 解码整数
func decodeInuxi(arch *sys.Arch, p []byte, sz int) uint64 {
	switch sz {
	case 2:
		return uint64(arch.ByteOrder.Uint16(p))
	case 4:
		return uint64(arch.ByteOrder.Uint32(p))
	case 8:
		return arch.ByteOrder.Uint64(p)
	default:
		Exitf("dwarf: decode inuxi %d", sz)
		panic("unreachable")
	}
}

/*
type _type struct {
	size       uintptr
	ptrdata    uintptr // size of memory prefix holding all pointers
	hash       uint32
	tflag      tflag
	align      uint8
	fieldAlign uint8
	kind       uint8
	// function for comparing objects of this type
	// (ptr to object A, ptr to object B) -> ==?
	equal func(unsafe.Pointer, unsafe.Pointer) bool
	// gcdata stores the GC type data for the garbage collector.
	// If the KindGCProg bit is set in kind, gcdata is a GC program.
	// Otherwise it is a ptrmask bitmap. See mbitmap.go for details.
	gcdata    *byte
	str       nameOff
	ptrToThis typeOff
}
*/
func commonsize(arch *sys.Arch) int { return 4*arch.PtrSize + 8 + 8 } // runtime._type

/*
type structfield struct {
	name       name
	typ        *_type
	offsetAnon uintptr
}
*/
func structfieldSize(arch *sys.Arch) int { return 3 * arch.PtrSize } // runtime.structfield

/*
type uncommontype struct {
	pkgpath nameOff
	mcount  uint16 // number of methods
	xcount  uint16 // number of exported methods
	moff    uint32 // offset from this uncommontype to [mcount]method
	_       uint32 // unused
}
*/
func uncommonSize() int { return 4 + 2 + 2 + 4 + 4 } // runtime.uncommontype

// Type.commonType.kind
func decodetypeKind(arch *sys.Arch, p []byte) uint8 {
	return p[2*arch.PtrSize+7] & objabi.KindMask //  0x13 / 0x1f
}

// Type.commonType.kind
func decodetypeUsegcprog(arch *sys.Arch, p []byte) uint8 {
	return p[2*arch.PtrSize+7] & objabi.KindGCProg //  0x13 / 0x1f
}

// Type.commonType.size
func decodetypeSize(arch *sys.Arch, p []byte) int64 {
	return int64(decodeInuxi(arch, p, arch.PtrSize)) // 0x8 / 0x10
}

// Type.commonType.ptrdata
func decodetypePtrdata(arch *sys.Arch, p []byte) int64 {
	return int64(decodeInuxi(arch, p[arch.PtrSize:], arch.PtrSize)) // 0x8 / 0x10
}

// Type.commonType.tflag
func decodetypeHasUncommon(arch *sys.Arch, p []byte) bool {
	return p[2*arch.PtrSize+4]&tflagUncommon != 0
}

// Type.FuncType.dotdotdot
func decodetypeFuncDotdotdot(arch *sys.Arch, p []byte) bool {
	return uint16(decodeInuxi(arch, p[commonsize(arch)+2:], 2))&(1<<15) != 0
}

/*
type functype struct {
	typ      _type
	inCount  uint16
	outCount uint16
}
*/

// Type.FuncType.inCount
func decodetypeFuncInCount(arch *sys.Arch, p []byte) int {
	return int(decodeInuxi(arch, p[commonsize(arch):], 2))
}

func decodetypeFuncOutCount(arch *sys.Arch, p []byte) int {
	return int(uint16(decodeInuxi(arch, p[commonsize(arch)+2:], 2)) & (1<<15 - 1))
}

/*
type interfacetype struct {
	typ     _type
	pkgpath name // *byte, 不是string!
	mhdr    []imethod
}
*/
// InterfaceType.methods.length
func decodetypeIfaceMethodCount(arch *sys.Arch, p []byte) int64 {
	return int64(decodeInuxi(arch, p[commonsize(arch)+2*arch.PtrSize:], arch.PtrSize))
}

// Matches runtime/typekind.go and reflect.Kind.
const (
	kindArray     = 17
	kindChan      = 18
	kindFunc      = 19
	kindInterface = 20
	kindMap       = 21
	kindPtr       = 22
	kindSlice     = 23
	kindStruct    = 25
	kindMask      = (1 << 5) - 1
)

// 找到off对应的reloc: off指引用者需要写入重定位后地址的数据的偏移量, 就是某个指针在对象中的偏移量
func decodeReloc(ldr *loader.Loader, symIdx loader.Sym, relocs *loader.Relocs, off int32) loader.Reloc {
	for j := 0; j < relocs.Count(); j++ {
		rel := relocs.At(j)
		if rel.Off() == off {
			return rel
		}
	}
	return loader.Reloc{}
}

// 找到off对应的reloc指向的sym
func decodeRelocSym(ldr *loader.Loader, symIdx loader.Sym, relocs *loader.Relocs, off int32) loader.Sym {
	return decodeReloc(ldr, symIdx, relocs, off).Sym()
}

// decodetypeName decodes the name from a reflect.name.
func decodetypeName(ldr *loader.Loader, symIdx loader.Sym, relocs *loader.Relocs, off int) string {
	r := decodeRelocSym(ldr, symIdx, relocs, int32(off))
	if r == 0 {
		return ""
	}

	// 字段类型可以是name(*byte), nameOff(uint32), 都是指向一个name对象
	//  - R_ADDR: *byte
	//  - R_ADDROFF: uint32
	data := ldr.Data(r)

	// 第一个字节为标志位，然后是多个<len, bindata>
	nameLen, nameLenLen := binary.Uvarint(data[1:])
	return string(data[1+nameLenLen : 1+nameLenLen+int(nameLen)])
}

/*
	type u struct {
		functype
		u uncommontype
		in0 *_type
		in1 *_type
		...
		out0 *_type
		out1 *_type
	}
*/
// 返回func第i个入参的类型对应的sym
func decodetypeFuncInType(ldr *loader.Loader, arch *sys.Arch, symIdx loader.Sym, relocs *loader.Relocs, i int) loader.Sym {
	// fmt.Printf("decodetypeFuncInType: kind=%v\n", decodetypeKind(arch, ldr.Data(symIdx)))
	uadd := commonsize(arch) + 4 // inCount, outCount uint16
	if arch.PtrSize == 8 {
		uadd += 4 // 对齐8字节
	}

	if decodetypeHasUncommon(arch, ldr.Data(symIdx)) {
		uadd += uncommonSize()
	}
	return decodeRelocSym(ldr, symIdx, relocs, int32(uadd+i*arch.PtrSize))
}

// 返回func第i个出参的类型对应的sym
func decodetypeFuncOutType(ldr *loader.Loader, arch *sys.Arch, symIdx loader.Sym, relocs *loader.Relocs, i int) loader.Sym {
	return decodetypeFuncInType(ldr, arch, symIdx, relocs, i+decodetypeFuncInCount(arch, ldr.Data(symIdx)))
}

// arraytype.elem *_type
func decodetypeArrayElem(ldr *loader.Loader, arch *sys.Arch, symIdx loader.Sym) loader.Sym {
	relocs := ldr.Relocs(symIdx)
	return decodeRelocSym(ldr, symIdx, &relocs, int32(commonsize(arch))) // 0x1c / 0x30
}

func decodetypeArrayLen(ldr *loader.Loader, arch *sys.Arch, symIdx loader.Sym) int64 {
	data := ldr.Data(symIdx)
	return int64(decodeInuxi(arch, data[commonsize(arch)+2*arch.PtrSize:], arch.PtrSize))
}

func decodetypeChanElem(ldr *loader.Loader, arch *sys.Arch, symIdx loader.Sym) loader.Sym {
	relocs := ldr.Relocs(symIdx)
	return decodeRelocSym(ldr, symIdx, &relocs, int32(commonsize(arch))) // 0x1c / 0x30
}

func decodetypeMapKey(ldr *loader.Loader, arch *sys.Arch, symIdx loader.Sym) loader.Sym {
	relocs := ldr.Relocs(symIdx)
	return decodeRelocSym(ldr, symIdx, &relocs, int32(commonsize(arch))) // 0x1c / 0x30
}

func decodetypeMapValue(ldr *loader.Loader, arch *sys.Arch, symIdx loader.Sym) loader.Sym {
	relocs := ldr.Relocs(symIdx)
	return decodeRelocSym(ldr, symIdx, &relocs, int32(commonsize(arch))+int32(arch.PtrSize)) // 0x20 / 0x38
}

func decodetypePtrElem(ldr *loader.Loader, arch *sys.Arch, symIdx loader.Sym) loader.Sym {
	relocs := ldr.Relocs(symIdx)
	return decodeRelocSym(ldr, symIdx, &relocs, int32(commonsize(arch))) // 0x1c / 0x30
}

/*
type structtype struct {
	typ     _type
	pkgPath name
	fields  []structfield

	field0 structfield
	field1 structfield
	....
}
*/
func decodetypeStructFieldCount(ldr *loader.Loader, arch *sys.Arch, symIdx loader.Sym) int {
	data := ldr.Data(symIdx)
	return int(decodeInuxi(arch, data[commonsize(arch)+2*arch.PtrSize:], arch.PtrSize))
}

func decodetypeStructFieldArrayOff(ldr *loader.Loader, arch *sys.Arch, symIdx loader.Sym, i int) int {
	data := ldr.Data(symIdx)
	off := commonsize(arch) + 4*arch.PtrSize
	if decodetypeHasUncommon(arch, data) {
		off += uncommonSize()
	}
	off += i * structfieldSize(arch)
	return off
}

// 返回structtype.fields[i].name
func decodetypeStructFieldName(ldr *loader.Loader, arch *sys.Arch, symIdx loader.Sym, i int) string {
	off := decodetypeStructFieldArrayOff(ldr, arch, symIdx, i)
	relocs := ldr.Relocs(symIdx)
	return decodetypeName(ldr, symIdx, &relocs, off)
}

// 返回structtype.fields[i].type
func decodetypeStructFieldType(ldr *loader.Loader, arch *sys.Arch, symIdx loader.Sym, i int) loader.Sym {
	off := decodetypeStructFieldArrayOff(ldr, arch, symIdx, i)
	relocs := ldr.Relocs(symIdx)
	return decodeRelocSym(ldr, symIdx, &relocs, int32(off+arch.PtrSize))
}

// 返回structtype.fields[i].offsetAnon
func decodetypeStructFieldOffsAnon(ldr *loader.Loader, arch *sys.Arch, symIdx loader.Sym, i int) int64 {
	off := decodetypeStructFieldArrayOff(ldr, arch, symIdx, i)
	data := ldr.Data(symIdx)
	return int64(decodeInuxi(arch, data[off+2*arch.PtrSize:], arch.PtrSize))
}

// decodetypeStr returns the contents of an rtype's str field (a nameOff).
func decodetypeStr(ldr *loader.Loader, arch *sys.Arch, symIdx loader.Sym) string {
	relocs := ldr.Relocs(symIdx)
	str := decodetypeName(ldr, symIdx, &relocs, 4*arch.PtrSize+8) // nameOff类型字段
	data := ldr.Data(symIdx)
	if data[2*arch.PtrSize+4]&tflagExtraStar != 0 { // _type.tflag & tflagExtraStar
		return str[1:]
	}
	return str
}

// 返回_type.gcdata数据
func decodetypeGcmask(ctxt *Link, s loader.Sym) []byte {
	if ctxt.loader.SymType(s) == sym.SDYNIMPORT {
		symData := ctxt.loader.Data(s)
		// shlib中_type.gcdata已经重定向为vaddr, 已经没有relocs表，只能通过这个地址取数据
		addr := decodetypeGcprogShlib(ctxt, symData)
		ptrdata := decodetypePtrdata(ctxt.Arch, symData) // size of memory prefix holding ptr
		sect := findShlibSection(ctxt, ctxt.loader.SymPkg(s), addr)
		if sect != nil {
			bits := ptrdata / int64(ctxt.Arch.PtrSize)
			r := make([]byte, (bits+7)/8)
			// ldshlibsyms avoids closing the ELF file so sect.ReadAt works.
			// If we remove this read (and the ones in decodetypeGcprog), we
			// can close the file.
			_, err := sect.ReadAt(r, int64(addr-sect.Addr))
			if err != nil {
				log.Fatal(err)
			}
			return r
		}
		Exitf("cannot find gcmask for %s", ctxt.loader.SymName(s))
		return nil
	}

	relocs := ctxt.loader.Relocs(s)
	mask := decodeRelocSym(ctxt.loader, s, &relocs, 2*int32(ctxt.Arch.PtrSize)+8+1*int32(ctxt.Arch.PtrSize))
	return ctxt.loader.Data(mask)
}

// Type.commonType.gc
func decodetypeGcprog(ctxt *Link, s loader.Sym) []byte {
	if ctxt.loader.SymType(s) == sym.SDYNIMPORT {
		symData := ctxt.loader.Data(s)
		addr := decodetypeGcprogShlib(ctxt, symData)
		sect := findShlibSection(ctxt, ctxt.loader.SymPkg(s), addr)
		if sect != nil {
			// A gcprog is a 4-byte uint32 indicating length, followed by
			// the actual program.
			progsize := make([]byte, 4)
			_, err := sect.ReadAt(progsize, int64(addr-sect.Addr))
			if err != nil {
				log.Fatal(err)
			}
			progbytes := make([]byte, ctxt.Arch.ByteOrder.Uint32(progsize))
			_, err = sect.ReadAt(progbytes, int64(addr-sect.Addr+4))
			if err != nil {
				log.Fatal(err)
			}
			return append(progsize, progbytes...)
		}
		Exitf("cannot find gcmask for %s", ctxt.loader.SymName(s))
		return nil
	}
	relocs := ctxt.loader.Relocs(s)
	rs := decodeRelocSym(ctxt.loader, s, &relocs, 2*int32(ctxt.Arch.PtrSize)+8+1*int32(ctxt.Arch.PtrSize))
	return ctxt.loader.Data(rs)
}

// Find the elf.Section of a given shared library that contains a given address.
func findShlibSection(ctxt *Link, path string, addr uint64) *elf.Section {
	for _, shlib := range ctxt.Shlibs {
		if shlib.Path == path {
			for _, sect := range shlib.File.Sections[1:] { // skip the NULL section
				if sect.Addr <= addr && addr < sect.Addr+sect.Size {
					return sect
				}
			}
		}
	}
	return nil
}

func decodetypeGcprogShlib(ctxt *Link, data []byte) uint64 {
	return decodeInuxi(ctxt.Arch, data[2*int32(ctxt.Arch.PtrSize)+8+1*int32(ctxt.Arch.PtrSize):], ctxt.Arch.PtrSize)
}
