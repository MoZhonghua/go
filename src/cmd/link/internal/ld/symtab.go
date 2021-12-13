// Inferno utils/6l/span.c
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
	"cmd/internal/obj"
	"cmd/internal/objabi"
	"cmd/link/internal/loader"
	"cmd/link/internal/sym"
	"debug/elf"
	"fmt"
	"internal/buildcfg"
	"path/filepath"
	"strings"
)

// objdump -h /usr/bin/go 不会输出最后的.symtab和.strtab两个section
// readlef -S /usr/bin/go 会输出全部的section

// 所有kind=SSYMTAB的sym，最终输出到".gosymtab" seciton中
// 实际只有runtime.symtab这个sym，大小为0

// .gosymtab不是ELF的symtab。ELF symtab在ELF文件最后，不是在section中，link后期调用
//  asmElfSym()直接输出

// sh = section header

// ELF中symtab中每一项如有下字段
//  - st_name: 指向symbol string table偏移量，符号名称，relocation时用这个名字匹配
//  - st_value: 多种含义，在exe/dso中通常是vaddr
//  - st_shndx: 所在的section索引
// st_value + st_shndx就能够计算出最终地址

// binding就三种: 说的是定义sym，引用sym是通过st_index==SHN_UNDEF来表示
// - LOCAL: Local symbols are not visible outside the object file containing their definition.
// - GLOBAL: Global symbols are visible to all object files being combined
// - WEAK: Weak symbols resemble global symbols, but their definitions have lower precedence

// symbol type 主要用的就4种
//  - STT_OBJECT: 数据
//  - STT_FUNC: 函数
//  - STT_SECTION: elf section
//  - STT_FILE: A file symbol has STB_LOCAL binding, its section index is SHN_ABS

// symbol的st_shndx字段有几个特殊值:
//  - SHN_ABS: The symbol has an absolute value that will not change because of relocation
//  - SHN_UNDEF: 表示这个sym未定义，link时通过relocation修改所有引用这个符号的数据指向sym真正地址

// st_value的含义:
//  - 普通的.o文件中，是sym在所属section中偏移量，方便link-relocation
//  - .so和.exe中，是sym的vaddr，更方便load-relocation

// elf rel/rela section的link和info两个字段含义:
//  - info: reloc项修改是哪个section的数据。每个reloc项只有offset字段，哪个section这里决定
//  - link: reloc项指向的哪个symtab section中的sym。每个reloc项只有sym index, 指向哪个symtab section这里决定
// elf relocation项包含三个字段
//  - offset: 在sect.info指向的section中偏移量，reloc在这里写入数据
//  - info: sym index | reloc type. sym index是在sect.link指向的symtab section中的索引
//  - addend

// dynamic-linked pie有interp指定dynamic linker来完成
// 注意-buildmode=pie生成的exe也是dynamic-linked
//  file /tmp/main
//    * ELF 64-bit LSB pie executable, x86-64, version 1 (SYSV), dynamically linked, interpreter /lib64/ld-linux-x86-64.so.2
//  ldd /tmp/main
//    * statically linked
//  ldd输出非常容易让人误解!!

/*
st_shndx: Every symbol table entry is "defined'' in relation to some section; this member holds the
relevant section header table index

	Half=16, Word=32, Xword=64, Addr=64
	typedef struct {
	        Elf64_Word      st_name;
	        unsigned char   st_info;  // (bind<<4) + (type&0xf)
	        unsigned char   st_other; // visibility
	        Elf64_Half      st_shndx; // section header table index
	        Elf64_Addr      st_value; // addr?
	        Elf64_Xword     st_size;
	} Elf64_Sym;
*/

/*
objdump -t /usr/bin/go
0000000000401000 l     F .text	0000000000000000 runtime.text
0000000000401e40 l     F .text	000000000000022d cmpbody
00000000004020a0 l     F .text	000000000000013e memeqbody
0000000000402220 l     F .text	0000000000000117 indexbytebody

0000000000401000: 地址, 对应st_value值
l: local
F: function
.text: section
000000000000022d: size
cmpbody: name
*/

func putelfstr(s string) int {
	if len(Elfstrdat) == 0 && s != "" {
		// first entry must be empty string
		putelfstr("")
	}

	off := len(Elfstrdat)
	Elfstrdat = append(Elfstrdat, s...)
	Elfstrdat = append(Elfstrdat, 0)
	return off
}

func putelfsyment(out *OutBuf, off int, addr int64, size int64, info uint8, shndx elf.SectionIndex, other int) {
	if elf64 {
		out.Write32(uint32(off))
		out.Write8(info)
		out.Write8(uint8(other))
		out.Write16(uint16(shndx))
		out.Write64(uint64(addr))
		out.Write64(uint64(size))
		symSize += ELF64SYMSIZE
	} else {
		out.Write32(uint32(off))  // name
		out.Write32(uint32(addr)) // value
		out.Write32(uint32(size)) // size
		out.Write8(info)          // info
		out.Write8(uint8(other)) // other
		out.Write16(uint16(shndx)) // shndx
		symSize += ELF32SYMSIZE
	}
}

func putelfsym(ctxt *Link, x loader.Sym, typ elf.SymType, curbind elf.SymBind) {
	ldr := ctxt.loader
	addr := ldr.SymValue(x) // Value通常是地址或者Offset
	size := ldr.SymSize(x)

	xo := x
	if ldr.OuterSym(x) != 0 {
		xo = ldr.OuterSym(x)
	}
	xot := ldr.SymType(xo)
	xosect := ldr.SymSect(xo)

	var elfshnum elf.SectionIndex
	if xot == sym.SDYNIMPORT || xot == sym.SHOSTOBJ || xot == sym.SUNDEFEXT {
		elfshnum = elf.SHN_UNDEF
		size = 0
	} else {
		if xosect == nil {
			ldr.Errorf(x, "missing section in putelfsym")
			return
		}
		if xosect.Elfsect == nil {
			ldr.Errorf(x, "missing ELF section in putelfsym")
			return
		}
		elfshnum = xosect.Elfsect.(*ElfShdr).shnum
	}

	sname := ldr.SymExtname(x)
	// 普通情况:  funcname<ABI0> => funcname.abi0
	// shared额外: funcname<ABInternal> => funcname.abiinternal
	sname = mangleABIName(ctxt, ldr, x, sname)

	// One pass for each binding: elf.STB_LOCAL, elf.STB_GLOBAL,
	// maybe one day elf.STB_WEAK.
	bind := elf.STB_GLOBAL
	if ldr.IsFileLocal(x) && !isStaticTmp(sname) || ldr.AttrVisibilityHidden(x) || ldr.AttrLocal(x) {
		// Static tmp is package local, but a package can be shared among multiple DSOs.
		// They need to have a single view of the static tmp that are writable.
		bind = elf.STB_LOCAL
	}

	// In external linking mode, we have to invoke gcc with -rdynamic
	// to get the exported symbols put into the dynamic symbol table.
	// To avoid filling the dynamic table with lots of unnecessary symbols,
	// mark all Go symbols local (not global) in the final executable.
	// But when we're dynamically linking, we need all those global symbols.
	if !ctxt.DynlinkingGo() && ctxt.IsExternal() && !ldr.AttrCgoExportStatic(x) && elfshnum != elf.SHN_UNDEF {
		bind = elf.STB_LOCAL
	}

	if ctxt.LinkMode == LinkExternal && elfshnum != elf.SHN_UNDEF {
		addr -= int64(xosect.Vaddr)
	}
	other := int(elf.STV_DEFAULT)
	if ldr.AttrVisibilityHidden(x) {
		// TODO(mwhudson): We only set AttrVisibilityHidden in ldelf, i.e. when
		// internally linking. But STV_HIDDEN visibility only matters in object
		// files and shared libraries, and as we are a long way from implementing
		// internal linking for shared libraries and only create object files when
		// externally linking, I don't think this makes a lot of sense.
		other = int(elf.STV_HIDDEN)
	}
	if ctxt.IsPPC64() && typ == elf.STT_FUNC && ldr.AttrShared(x) && ldr.SymName(x) != "runtime.duffzero" && ldr.SymName(x) != "runtime.duffcopy" {
		// On ppc64 the top three bits of the st_other field indicate how
		// many instructions separate the global and local entry points. In
		// our case it is two instructions, indicated by the value 3.
		// The conditions here match those in preprocess in
		// cmd/internal/obj/ppc64/obj9.go, which is where the
		// instructions are inserted.
		other |= 3 << 5
	}

	// When dynamically linking, we create Symbols by reading the names from
	// the symbol tables of the shared libraries and so the names need to
	// match exactly. Tools like DTrace will have to wait for now.
	if !ctxt.DynlinkingGo() {
		// Rewrite · to . for ASCII-only tools like DTrace (sigh)
		sname = strings.Replace(sname, "·", ".", -1)
	}

	if ctxt.DynlinkingGo() && bind == elf.STB_GLOBAL && curbind == elf.STB_LOCAL && ldr.SymType(x) == sym.STEXT {
		// When dynamically linking, we want references to functions defined
		// in this module to always be to the function object, not to the
		// PLT. We force this by writing an additional local symbol for every
		// global function symbol and making all relocations against the
		// global symbol refer to this local symbol instead (see
		// (*sym.Symbol).ElfsymForReloc). This is approximately equivalent to the
		// ELF linker -Bsymbolic-functions option, but that is buggy on
		// several platforms.
		putelfsyment(ctxt.Out, putelfstr("local."+sname), addr, size, elf.ST_INFO(elf.STB_LOCAL, typ), elfshnum, other)
		ldr.SetSymLocalElfSym(x, int32(ctxt.numelfsym))
		ctxt.numelfsym++
		return
	} else if bind != curbind {
		return
	}

	putelfsyment(ctxt.Out, putelfstr(sname), addr, size, elf.ST_INFO(bind, typ), elfshnum, other)
	// 记录Sym => symbol entry index
	ldr.SetSymElfSym(x, int32(ctxt.numelfsym))
	ctxt.numelfsym++
}

func putelfsectionsym(ctxt *Link, out *OutBuf, s loader.Sym, shndx elf.SectionIndex) {
	putelfsyment(out, 0, 0, 0, elf.ST_INFO(elf.STB_LOCAL, elf.STT_SECTION), shndx, 0)
	ctxt.loader.SetSymElfSym(s, int32(ctxt.numelfsym))
	ctxt.numelfsym++
}

func genelfsym(ctxt *Link, elfbind elf.SymBind) {
	ldr := ctxt.loader

	// objdump -t /usr/bin/go
	//   0000000000401000 l     F .text  0000000000000000 runtime.text
	// runtime.text marker symbol(s).
	s := ldr.Lookup("runtime.text", 0)
	putelfsym(ctxt, s, elf.STT_FUNC, elfbind)
	for k, sect := range Segtext.Sections[1:] { // 正常情况下code segment只有.text一个section
		n := k + 1
		if sect.Name != ".text" || (ctxt.IsAIX() && ctxt.IsExternal()) {
			// On AIX, runtime.text.X are symbols already in the symtab.
			break
		}
		s = ldr.Lookup(fmt.Sprintf("runtime.text.%d", n), 0)
		if s == 0 {
			break
		}
		if ldr.SymType(s) != sym.STEXT {
			panic("unexpected type for runtime.text symbol")
		}
		putelfsym(ctxt, s, elf.STT_FUNC, elfbind)
	}

	// Text symbols.
	for _, s := range ctxt.Textp {
		putelfsym(ctxt, s, elf.STT_FUNC, elfbind)
	}

	// runtime.etext marker symbol.
	s = ldr.Lookup("runtime.etext", 0) // end of text, size=0, value=end addr of text
	if ldr.SymType(s) == sym.STEXT {
		putelfsym(ctxt, s, elf.STT_FUNC, elfbind)
	}

	shouldBeInSymbolTable := func(s loader.Sym) bool {
		if ldr.AttrNotInSymbolTable(s) {
			return false
		}
		// FIXME: avoid having to do name inspections here.
		// NB: the restrictions below on file local symbols are a bit
		// arbitrary -- if it turns out we need nameless static
		// symbols they could be relaxed/removed.
		sn := ldr.SymName(s)
		if (sn == "" || sn[0] == '.') && ldr.IsFileLocal(s) {
			panic(fmt.Sprintf("unexpected file local symbol %d %s<%d>\n",
				s, sn, ldr.SymVersion(s)))
		}
		if (sn == "" || sn[0] == '.') && !ldr.IsFileLocal(s) {
			return false
		}
		return true
	}

	// Data symbols.
	for s := loader.Sym(1); s < loader.Sym(ldr.NSym()); s++ {
		if !ldr.AttrReachable(s) {
			continue
		}
		st := ldr.SymType(s)
		if st >= sym.SELFRXSECT && st < sym.SXREF {
			typ := elf.STT_OBJECT
			if st == sym.STLSBSS {
				if ctxt.IsInternal() {
					continue
				}
				typ = elf.STT_TLS
			}
			if !shouldBeInSymbolTable(s) {
				continue
			}
			putelfsym(ctxt, s, typ, elfbind)
			continue
		}
		if st == sym.SHOSTOBJ || st == sym.SDYNIMPORT || st == sym.SUNDEFEXT {
			putelfsym(ctxt, s, ldr.SymElfType(s), elfbind)
		}
	}
}

// 在link后期，输出ELF文件时调用
func asmElfSym(ctxt *Link) {

	// the first symbol entry is reserved
	putelfsyment(ctxt.Out, 0, 0, 0, elf.ST_INFO(elf.STB_LOCAL, elf.STT_NOTYPE), 0, 0)

	// 为每个dwarf section创建一个symbol entry
	dwarfaddelfsectionsyms(ctxt)

	// Some linkers will add a FILE sym if one is not present.
	// Avoid having the working directory inserted into the symbol table.
	// It is added with a name to avoid problems with external linking
	// encountered on some versions of Solaris. See issue #14957.
	putelfsyment(ctxt.Out, putelfstr("go.go"), 0, 0, elf.ST_INFO(elf.STB_LOCAL, elf.STT_FILE), elf.SHN_ABS, 0)
	ctxt.numelfsym++

	// 用不同的bind生成两遍?
	bindings := []elf.SymBind{elf.STB_LOCAL, elf.STB_GLOBAL}
	for _, elfbind := range bindings {
		if elfbind == elf.STB_GLOBAL {
			elfglobalsymndx = ctxt.numelfsym
		}
		genelfsym(ctxt, elfbind)
	}
}

func putplan9sym(ctxt *Link, ldr *loader.Loader, s loader.Sym, char SymbolType) {
	t := int(char)
	if ldr.IsFileLocal(s) {
		t += 'a' - 'A'
	}
	l := 4
	addr := ldr.SymValue(s)
	if ctxt.IsAMD64() && !flag8 {
		ctxt.Out.Write32b(uint32(addr >> 32))
		l = 8
	}

	ctxt.Out.Write32b(uint32(addr))
	ctxt.Out.Write8(uint8(t + 0x80)) /* 0x80 is variable length */

	name := ldr.SymName(s)
	ctxt.Out.WriteString(name)
	ctxt.Out.Write8(0)

	symSize += int32(l) + 1 + int32(len(name)) + 1
}

func asmbPlan9Sym(ctxt *Link) {
	ldr := ctxt.loader

	// Add special runtime.text and runtime.etext symbols.
	s := ldr.Lookup("runtime.text", 0)
	if ldr.SymType(s) == sym.STEXT {
		putplan9sym(ctxt, ldr, s, TextSym)
	}
	s = ldr.Lookup("runtime.etext", 0)
	if ldr.SymType(s) == sym.STEXT {
		putplan9sym(ctxt, ldr, s, TextSym)
	}

	// Add text symbols.
	for _, s := range ctxt.Textp {
		putplan9sym(ctxt, ldr, s, TextSym)
	}

	shouldBeInSymbolTable := func(s loader.Sym) bool {
		if ldr.AttrNotInSymbolTable(s) {
			return false
		}
		name := ldr.RawSymName(s) // TODO: try not to read the name
		if name == "" || name[0] == '.' {
			return false
		}
		return true
	}

	// Add data symbols and external references.
	for s := loader.Sym(1); s < loader.Sym(ldr.NSym()); s++ {
		if !ldr.AttrReachable(s) {
			continue
		}
		t := ldr.SymType(s)
		if t >= sym.SELFRXSECT && t < sym.SXREF { // data sections handled in dodata
			if t == sym.STLSBSS {
				continue
			}
			if !shouldBeInSymbolTable(s) {
				continue
			}
			char := DataSym
			if t == sym.SBSS || t == sym.SNOPTRBSS {
				char = BSSSym
			}
			putplan9sym(ctxt, ldr, s, char)
		}
	}
}

type byPkg []*sym.Library

func (libs byPkg) Len() int {
	return len(libs)
}

func (libs byPkg) Less(a, b int) bool {
	return libs[a].Pkg < libs[b].Pkg // Library对应一个package => 一个.a => 一个_go_.o + N个asm.o
}

func (libs byPkg) Swap(a, b int) {
	libs[a], libs[b] = libs[b], libs[a]
}

// Create a table with information on the text sections.
// Return the symbol of the table, and number of sections.
func textsectionmap(ctxt *Link) (loader.Sym, uint32) {
	ldr := ctxt.loader
	t := ldr.CreateSymForUpdate("runtime.textsectionmap", 0)
	t.SetType(sym.SRODATA)
	nsections := int64(0)

	for _, sect := range Segtext.Sections {
		if sect.Name == ".text" { // 代码被拆分为多个section?
			nsections++
		} else {
			break
		}
	}
	t.Grow(3 * nsections * int64(ctxt.Arch.PtrSize))

	off := int64(0)
	n := 0

	// The vaddr for each text section is the difference between the section's
	// Vaddr and the Vaddr for the first text section as determined at compile
	// time.

	// The symbol for the first text section is named runtime.text as before.
	// Additional text sections are named runtime.text.n where n is the
	// order of creation starting with 1. These symbols provide the section's
	// address after relocation by the linker.

	// 要知道重定向后的地址，唯一的办法是构造一个Sym，然后在Sym中写入一个指向X地址的
	// relocation数据。当linker完成relocation之后，就可以根据Sym中的数据得到X的真正地址

	textbase := Segtext.Sections[0].Vaddr
	for _, sect := range Segtext.Sections {
		if sect.Name != ".text" {
			break
		}
		off = t.SetUint(ctxt.Arch, off, sect.Vaddr-textbase)
		off = t.SetUint(ctxt.Arch, off, sect.Length)
		if n == 0 {
			s := ldr.Lookup("runtime.text", 0)
			if s == 0 {
				ctxt.Errorf(s, "Unable to find symbol runtime.text\n")
			}
			off = t.SetAddr(ctxt.Arch, off, s)

		} else {
			s := ldr.Lookup(fmt.Sprintf("runtime.text.%d", n), 0)
			if s == 0 {
				ctxt.Errorf(s, "Unable to find symbol runtime.text.%d\n", n)
			}
			off = t.SetAddr(ctxt.Arch, off, s)
		}
		n++
	}
	return t.Sym(), uint32(n)
}

// 遍历所有sym
//  - 根据sym类型设置SetAttrNotInSymbolTable()
//  - 为每个sym重新计算一个合适的kind，返回数组，index就是SymIdx
//  - 一些sym设置carrier sym
// 设置runtime.firstmoduledata各个字段
func (ctxt *Link) symtab(pcln *pclntab) []sym.SymKind {
	ldr := ctxt.loader

	if !ctxt.IsAIX() {
		switch ctxt.BuildMode {
		case BuildModeCArchive, BuildModeCShared:
			s := ldr.Lookup(*flagEntrySymbol, sym.SymVerABI0)
			if s != 0 {
				addinitarrdata(ctxt, ldr, s)
			}
		}
	}

	// Define these so that they'll get put into the symbol table.
	// data.c:/^address will provide the actual values.
	//  都定义为local symbol, size=0, value=0，后面需要更新value为对应对象地址
	ctxt.xdefine("runtime.rodata", sym.SRODATA, 0)
	ctxt.xdefine("runtime.erodata", sym.SRODATA, 0)
	ctxt.xdefine("runtime.types", sym.SRODATA, 0)
	ctxt.xdefine("runtime.etypes", sym.SRODATA, 0)
	ctxt.xdefine("runtime.noptrdata", sym.SNOPTRDATA, 0)
	ctxt.xdefine("runtime.enoptrdata", sym.SNOPTRDATA, 0)
	ctxt.xdefine("runtime.data", sym.SDATA, 0)
	ctxt.xdefine("runtime.edata", sym.SDATA, 0)
	ctxt.xdefine("runtime.bss", sym.SBSS, 0)
	ctxt.xdefine("runtime.ebss", sym.SBSS, 0)
	ctxt.xdefine("runtime.noptrbss", sym.SNOPTRBSS, 0)
	ctxt.xdefine("runtime.enoptrbss", sym.SNOPTRBSS, 0)
	ctxt.xdefine("runtime.end", sym.SBSS, 0)
	ctxt.xdefine("runtime.epclntab", sym.SRODATA, 0)
	ctxt.xdefine("runtime.esymtab", sym.SRODATA, 0)

	// garbage collection symbols
	s := ldr.CreateSymForUpdate("runtime.gcdata", 0)
	s.SetType(sym.SRODATA)
	s.SetSize(0)
	ctxt.xdefine("runtime.egcdata", sym.SRODATA, 0)

	s = ldr.CreateSymForUpdate("runtime.gcbss", 0)
	s.SetType(sym.SRODATA)
	s.SetSize(0)
	ctxt.xdefine("runtime.egcbss", sym.SRODATA, 0)

	// pseudo-symbols to mark locations of type, string, and go string data.
	var symtype, symtyperel loader.Sym
	if !ctxt.DynlinkingGo() {
		if ctxt.UseRelro() && (ctxt.BuildMode == BuildModeCArchive || ctxt.BuildMode == BuildModeCShared || ctxt.BuildMode == BuildModePIE) {
			s = ldr.CreateSymForUpdate("type.*", 0)
			s.SetType(sym.STYPE)
			s.SetSize(0)
			symtype = s.Sym()

			s = ldr.CreateSymForUpdate("typerel.*", 0)
			s.SetType(sym.STYPERELRO)
			s.SetSize(0)
			symtyperel = s.Sym()
		} else {
			s = ldr.CreateSymForUpdate("type.*", 0)
			s.SetType(sym.STYPE)
			s.SetSize(0)
			symtype = s.Sym()
			symtyperel = s.Sym()
		}
		// 注意名字为type.*，做为一个carrier sym, 每个sym.STYPE只能有一个Carrier Symbol
		setCarrierSym(sym.STYPE, symtype)
		setCarrierSym(sym.STYPERELRO, symtyperel)
	}

	groupSym := func(name string, t sym.SymKind) loader.Sym {
		s := ldr.CreateSymForUpdate(name, 0)
		s.SetType(t)
		s.SetSize(0)
		s.SetLocal(true)
		setCarrierSym(t, s.Sym())
		return s.Sym()
	}
	var (
		symgostring = groupSym("go.string.*", sym.SGOSTRING)
		symgofunc   = groupSym("go.func.*", sym.SGOFUNC)
		symgcbits   = groupSym("runtime.gcbits.*", sym.SGCBITS)
	)

	var symgofuncrel loader.Sym
	if !ctxt.DynlinkingGo() {
		if ctxt.UseRelro() {
			symgofuncrel = groupSym("go.funcrel.*", sym.SGOFUNCRELRO)
		} else {
			symgofuncrel = symgofunc
		}
	}

	symt := ldr.CreateSymForUpdate("runtime.symtab", 0)
	symt.SetType(sym.SSYMTAB)
	symt.SetSize(0)
	symt.SetLocal(true)

	// assign specific types so that they sort together.
	// within a type they sort by size, so the .* symbols
	// just defined above will be first.
	// hide the specific symbols.
	nsym := loader.Sym(ldr.NSym())
	symGroupType := make([]sym.SymKind, nsym)
	for s := loader.Sym(1); s < nsym; s++ {
		if !ctxt.IsExternal() && ldr.IsFileLocal(s) && !ldr.IsFromAssembly(s) && ldr.SymPkg(s) != "" {
			ldr.SetAttrNotInSymbolTable(s, true)
		}
		if !ldr.AttrReachable(s) || ldr.AttrSpecial(s) || (ldr.SymType(s) != sym.SRODATA && ldr.SymType(s) != sym.SGOFUNC) {
			continue
		}

		name := ldr.SymName(s)
		switch {
		case strings.HasPrefix(name, "type."):
			if !ctxt.DynlinkingGo() {
				// 普通exe中的symtab不包含type.XX，只有一个type.*
				ldr.SetAttrNotInSymbolTable(s, true)
			}
			if ctxt.UseRelro() {
				symGroupType[s] = sym.STYPERELRO
				if symtyperel != 0 {
					ldr.SetCarrierSym(s, symtyperel)
				}
			} else {
				symGroupType[s] = sym.STYPE
				// TODO(mzh): check with wrong variable
				if symtype != 0 {
					ldr.SetCarrierSym(s, symtype)
				}
			}

		case strings.HasPrefix(name, "go.importpath.") && ctxt.UseRelro():
			// Keep go.importpath symbols in the same section as types and
			// names, as they can be referred to by a section offset.
			symGroupType[s] = sym.STYPERELRO

		case strings.HasPrefix(name, "go.string."):
			symGroupType[s] = sym.SGOSTRING
			ldr.SetAttrNotInSymbolTable(s, true)
			ldr.SetCarrierSym(s, symgostring)

		case strings.HasPrefix(name, "runtime.gcbits."):
			symGroupType[s] = sym.SGCBITS
			ldr.SetAttrNotInSymbolTable(s, true)
			ldr.SetCarrierSym(s, symgcbits)

		case strings.HasSuffix(name, "·f"):
			if !ctxt.DynlinkingGo() {
				ldr.SetAttrNotInSymbolTable(s, true)
			}
			if ctxt.UseRelro() {
				symGroupType[s] = sym.SGOFUNCRELRO
				if symgofuncrel != 0 {
					ldr.SetCarrierSym(s, symgofuncrel)
				}
			} else {
				symGroupType[s] = sym.SGOFUNC
				ldr.SetCarrierSym(s, symgofunc)
			}

		case strings.HasPrefix(name, "gcargs."),
			strings.HasPrefix(name, "gclocals."),
			strings.HasPrefix(name, "gclocals·"),
			ldr.SymType(s) == sym.SGOFUNC && s != symgofunc,
			strings.HasSuffix(name, ".opendefer"),
			strings.HasSuffix(name, ".arginfo0"),
			strings.HasSuffix(name, ".arginfo1"):
			symGroupType[s] = sym.SGOFUNC
			ldr.SetAttrNotInSymbolTable(s, true)
			ldr.SetCarrierSym(s, symgofunc)
			align := int32(4)
			if a := ldr.SymAlign(s); a < align {
				ldr.SetSymAlign(s, align)
			} else {
				align = a
			}
			// 指用来GC的数据的大小
			liveness += (ldr.SymSize(s) + int64(align) - 1) &^ (int64(align) - 1)
		}
	}

	if ctxt.BuildMode == BuildModeShared {
		abihashgostr := ldr.CreateSymForUpdate("go.link.abihash."+filepath.Base(*flagOutfile), 0)
		abihashgostr.SetType(sym.SRODATA)
		hashsym := ldr.LookupOrCreateSym("go.link.abihashbytes", 0)
		abihashgostr.AddAddr(ctxt.Arch, hashsym)
		abihashgostr.AddUint(ctxt.Arch, uint64(ldr.SymSize(hashsym)))
	}
	// 创建pkg fingerprint
	if ctxt.BuildMode == BuildModePlugin || ctxt.CanUsePlugins() {
		for _, l := range ctxt.Library {
			s := ldr.CreateSymForUpdate("go.link.pkghashbytes."+l.Pkg, 0)
			s.SetType(sym.SRODATA)
			s.SetSize(int64(len(l.Fingerprint)))
			s.SetData(l.Fingerprint[:])
			str := ldr.CreateSymForUpdate("go.link.pkghash."+l.Pkg, 0)
			str.SetType(sym.SRODATA)
			str.AddAddr(ctxt.Arch, s.Sym())
			str.AddUint(ctxt.Arch, uint64(len(l.Fingerprint)))
		}
	}

	textsectionmapSym, nsections := textsectionmap(ctxt)

	// Information about the layout of the executable image for the
	// runtime to use. Any changes here must be matched by changes to
	// the definition of moduledata in runtime/symtab.go.
	// This code uses several global variables that are set by pcln.go:pclntab.
	// 普通的exe中 ctxt.Moduledata 指向 runtime.firstmoduledata 变量
	// 这里实际就是在设置变量值，只是绝大多数都是relocation
	moduledata := ldr.MakeSymbolUpdater(ctxt.Moduledata)
	// The pcHeader
	moduledata.AddAddr(ctxt.Arch, pcln.pcheader)
	// The function name slice // slice是<data, size, cap>三个字段组成
	moduledata.AddAddr(ctxt.Arch, pcln.funcnametab)
	moduledata.AddUint(ctxt.Arch, uint64(ldr.SymSize(pcln.funcnametab)))
	moduledata.AddUint(ctxt.Arch, uint64(ldr.SymSize(pcln.funcnametab)))
	// The cutab slice
	moduledata.AddAddr(ctxt.Arch, pcln.cutab)
	moduledata.AddUint(ctxt.Arch, uint64(ldr.SymSize(pcln.cutab)))
	moduledata.AddUint(ctxt.Arch, uint64(ldr.SymSize(pcln.cutab)))
	// The filetab slice
	moduledata.AddAddr(ctxt.Arch, pcln.filetab)
	moduledata.AddUint(ctxt.Arch, uint64(ldr.SymSize(pcln.filetab)))
	moduledata.AddUint(ctxt.Arch, uint64(ldr.SymSize(pcln.filetab)))
	// The pctab slice
	moduledata.AddAddr(ctxt.Arch, pcln.pctab)
	moduledata.AddUint(ctxt.Arch, uint64(ldr.SymSize(pcln.pctab)))
	moduledata.AddUint(ctxt.Arch, uint64(ldr.SymSize(pcln.pctab)))
	// The pclntab slice
	moduledata.AddAddr(ctxt.Arch, pcln.pclntab)
	moduledata.AddUint(ctxt.Arch, uint64(ldr.SymSize(pcln.pclntab)))
	moduledata.AddUint(ctxt.Arch, uint64(ldr.SymSize(pcln.pclntab)))
	// The ftab slice
	moduledata.AddAddr(ctxt.Arch, pcln.pclntab)
	moduledata.AddUint(ctxt.Arch, uint64(pcln.nfunc+1))
	moduledata.AddUint(ctxt.Arch, uint64(pcln.nfunc+1))
	// findfunctab
	moduledata.AddAddr(ctxt.Arch, pcln.findfunctab)
	// minpc, maxpc
	moduledata.AddAddr(ctxt.Arch, pcln.firstFunc)
	moduledata.AddAddrPlus(ctxt.Arch, pcln.lastFunc, ldr.SymSize(pcln.lastFunc))

	// pointers to specific parts of the module
	// 基本模式为:
	//   1. 在runtime中定义变量，firstmoduledata
	//   2. ld中通过查找firstmoduledata，并生成一个updater，然后重写内容
	//   3. 对于所有需要通过link后才能确定的字段，通过relocation设置, 填入指向这个Sym的R_ADDR或者R_ADDROFF
	//      - Sym本身有数据，此时Value就是Sym最终的地址, 比如pcln里的各个数据
	//      - Sym本身没数据(size=0)，此时Value实际就是一个Int值，比如runtime.text
	moduledata.AddAddr(ctxt.Arch, ldr.Lookup("runtime.text", 0)) // .text
	moduledata.AddAddr(ctxt.Arch, ldr.Lookup("runtime.etext", 0))
	moduledata.AddAddr(ctxt.Arch, ldr.Lookup("runtime.noptrdata", 0)) // .noptrdata
	moduledata.AddAddr(ctxt.Arch, ldr.Lookup("runtime.enoptrdata", 0))
	moduledata.AddAddr(ctxt.Arch, ldr.Lookup("runtime.data", 0)) // .data
	moduledata.AddAddr(ctxt.Arch, ldr.Lookup("runtime.edata", 0))
	moduledata.AddAddr(ctxt.Arch, ldr.Lookup("runtime.bss", 0)) // .bss
	moduledata.AddAddr(ctxt.Arch, ldr.Lookup("runtime.ebss", 0))
	moduledata.AddAddr(ctxt.Arch, ldr.Lookup("runtime.noptrbss", 0)) // .noptrbss
	moduledata.AddAddr(ctxt.Arch, ldr.Lookup("runtime.enoptrbss", 0))
	moduledata.AddAddr(ctxt.Arch, ldr.Lookup("runtime.end", 0))

	// gcprog for .data and .bss
	// 这两个不是区域的起始结束地址，就是指向正常Sym
	moduledata.AddAddr(ctxt.Arch, ldr.Lookup("runtime.gcdata", 0))
	moduledata.AddAddr(ctxt.Arch, ldr.Lookup("runtime.gcbss", 0))

	// Value(runtime.types) = Sect(runtime.types).vaddr
	// type.xxx数据不用放到所属section的最前面。因为runtime.types也不是指
	// 全部types数据的起始位置，而是type.xxx所在section的起始位置，因此
	// firstmoduledata.types + R_ADDROFF能正确指向type.xxx
	moduledata.AddAddr(ctxt.Arch, ldr.Lookup("runtime.types", 0))
	moduledata.AddAddr(ctxt.Arch, ldr.Lookup("runtime.etypes", 0))

	if ctxt.IsAIX() && ctxt.IsExternal() {
		// 只是增加了relocation，没有写入。 reloc的off=0, size=0，也不需要更新
		// Add R_XCOFFREF relocation to prevent ld's garbage collection of
		// runtime.rodata, runtime.erodata and runtime.epclntab.
		addRef := func(name string) {
			r, _ := moduledata.AddRel(objabi.R_XCOFFREF)
			r.SetSym(ldr.Lookup(name, 0))
			r.SetSiz(uint8(ctxt.Arch.PtrSize))
		}
		addRef("runtime.rodata")
		addRef("runtime.erodata")
		addRef("runtime.epclntab")
	}

	// text section information
	moduledata.AddAddr(ctxt.Arch, textsectionmapSym)
	moduledata.AddUint(ctxt.Arch, uint64(nsections))
	moduledata.AddUint(ctxt.Arch, uint64(nsections))

	// The typelinks slice
	typelinkSym := ldr.Lookup("runtime.typelink", 0)
	ntypelinks := uint64(ldr.SymSize(typelinkSym)) / 4
	moduledata.AddAddr(ctxt.Arch, typelinkSym)
	moduledata.AddUint(ctxt.Arch, ntypelinks)
	moduledata.AddUint(ctxt.Arch, ntypelinks)
	// The itablinks slice
	itablinkSym := ldr.Lookup("runtime.itablink", 0)
	nitablinks := uint64(ldr.SymSize(itablinkSym)) / uint64(ctxt.Arch.PtrSize)
	moduledata.AddAddr(ctxt.Arch, itablinkSym)
	moduledata.AddUint(ctxt.Arch, nitablinks)
	moduledata.AddUint(ctxt.Arch, nitablinks)
	// The ptab slice
	if ptab := ldr.Lookup("go.plugin.tabs", 0); ptab != 0 && ldr.AttrReachable(ptab) {
		ldr.SetAttrLocal(ptab, true)
		if ldr.SymType(ptab) != sym.SRODATA {
			panic(fmt.Sprintf("go.plugin.tabs is %v, not SRODATA", ldr.SymType(ptab)))
		}
		nentries := uint64(len(ldr.Data(ptab)) / 8) // sizeof(nameOff) + sizeof(typeOff)
		moduledata.AddAddr(ctxt.Arch, ptab)
		moduledata.AddUint(ctxt.Arch, nentries)
		moduledata.AddUint(ctxt.Arch, nentries)
	} else {
		moduledata.AddUint(ctxt.Arch, 0)
		moduledata.AddUint(ctxt.Arch, 0)
		moduledata.AddUint(ctxt.Arch, 0)
	}
	if ctxt.BuildMode == BuildModePlugin {
		addgostring(ctxt, ldr, moduledata, "go.link.thispluginpath", objabi.PathToPrefix(*flagPluginPath))

		pkghashes := ldr.CreateSymForUpdate("go.link.pkghashes", 0)
		pkghashes.SetLocal(true)
		pkghashes.SetType(sym.SRODATA)

		for i, l := range ctxt.Library {
			// pkghashes[i].name
			addgostring(ctxt, ldr, pkghashes, fmt.Sprintf("go.link.pkgname.%d", i), l.Pkg)
			// pkghashes[i].linktimehash
			addgostring(ctxt, ldr, pkghashes, fmt.Sprintf("go.link.pkglinkhash.%d", i), string(l.Fingerprint[:]))
			// pkghashes[i].runtimehash
			hash := ldr.Lookup("go.link.pkghash."+l.Pkg, 0)
			pkghashes.AddAddr(ctxt.Arch, hash)
		}
		moduledata.AddAddr(ctxt.Arch, pkghashes.Sym())
		moduledata.AddUint(ctxt.Arch, uint64(len(ctxt.Library)))
		moduledata.AddUint(ctxt.Arch, uint64(len(ctxt.Library)))
	} else {
		moduledata.AddUint(ctxt.Arch, 0) // pluginpath
		moduledata.AddUint(ctxt.Arch, 0)
		moduledata.AddUint(ctxt.Arch, 0) // pkghashes slice
		moduledata.AddUint(ctxt.Arch, 0)
		moduledata.AddUint(ctxt.Arch, 0)
	}
	if len(ctxt.Shlibs) > 0 {
		thismodulename := filepath.Base(*flagOutfile)
		switch ctxt.BuildMode {
		case BuildModeExe, BuildModePIE:
			// When linking an executable, outfile is just "a.out". Make
			// it something slightly more comprehensible.
			thismodulename = "the executable"
		}
		addgostring(ctxt, ldr, moduledata, "go.link.thismodulename", thismodulename)

		modulehashes := ldr.CreateSymForUpdate("go.link.abihashes", 0)
		modulehashes.SetLocal(true)
		modulehashes.SetType(sym.SRODATA)

		for i, shlib := range ctxt.Shlibs {
			// modulehashes[i].modulename
			modulename := filepath.Base(shlib.Path)
			addgostring(ctxt, ldr, modulehashes, fmt.Sprintf("go.link.libname.%d", i), modulename)

			// modulehashes[i].linktimehash
			addgostring(ctxt, ldr, modulehashes, fmt.Sprintf("go.link.linkhash.%d", i), string(shlib.Hash))

			// modulehashes[i].runtimehash
			abihash := ldr.LookupOrCreateSym("go.link.abihash."+modulename, 0)
			ldr.SetAttrReachable(abihash, true)
			modulehashes.AddAddr(ctxt.Arch, abihash)
		}

		moduledata.AddAddr(ctxt.Arch, modulehashes.Sym())
		moduledata.AddUint(ctxt.Arch, uint64(len(ctxt.Shlibs)))
		moduledata.AddUint(ctxt.Arch, uint64(len(ctxt.Shlibs)))
	} else {
		moduledata.AddUint(ctxt.Arch, 0) // modulename
		moduledata.AddUint(ctxt.Arch, 0)
		moduledata.AddUint(ctxt.Arch, 0) // moduleshashes slice
		moduledata.AddUint(ctxt.Arch, 0)
		moduledata.AddUint(ctxt.Arch, 0)
	}

	hasmain := ctxt.BuildMode == BuildModeExe || ctxt.BuildMode == BuildModePIE
	if hasmain {
		moduledata.AddUint8(1)
	} else {
		moduledata.AddUint8(0)
	}

	// The rest of moduledata is zero initialized.
	// When linking an object that does not contain the runtime we are
	// creating the moduledata from scratch and it does not have a
	// compiler-provided size, so read it from the type data.
	moduledatatype := ldr.Lookup("type.runtime.moduledata", 0)
	moduledata.SetSize(decodetypeSize(ctxt.Arch, ldr.Data(moduledatatype)))
	moduledata.Grow(moduledata.Size())

	lastmoduledatap := ldr.CreateSymForUpdate("runtime.lastmoduledatap", 0)
	if lastmoduledatap.Type() != sym.SDYNIMPORT {
		lastmoduledatap.SetType(sym.SNOPTRDATA)
		lastmoduledatap.SetSize(0) // overwrite existing value
		lastmoduledatap.SetData(nil)
		lastmoduledatap.AddAddr(ctxt.Arch, moduledata.Sym())
	}
	return symGroupType
}

// CarrierSymByType tracks carrier symbols and their sizes.
var CarrierSymByType [sym.SXREF]struct {
	Sym  loader.Sym
	Size int64
}

func setCarrierSym(typ sym.SymKind, s loader.Sym) {
	if CarrierSymByType[typ].Sym != 0 {
		panic(fmt.Sprintf("carrier symbol for type %v already set", typ))
	}
	CarrierSymByType[typ].Sym = s
}

func setCarrierSize(typ sym.SymKind, sz int64) {
	if CarrierSymByType[typ].Size != 0 {
		panic(fmt.Sprintf("carrier symbol size for type %v already set", typ))
	}
	CarrierSymByType[typ].Size = sz
}

func isStaticTmp(name string) bool {
	// xx..stmp_
	return strings.Contains(name, "."+obj.StaticNamePref)
}

// Mangle function name with ABI information.
func mangleABIName(ctxt *Link, ldr *loader.Loader, x loader.Sym, name string) string {
	// For functions with ABI wrappers, we have to make sure that we
	// don't wind up with two symbol table entries with the same
	// name (since this will generated an error from the external
	// linker). If we have wrappers, keep the ABIInternal name
	// unmangled since we want cross-load-module calls to target
	// ABIInternal, and rename other symbols.
	//
	// TODO: avoid the ldr.Lookup calls below by instead using an aux
	// sym or marker relocation to associate the wrapper with the
	// wrapped function.
	if !buildcfg.Experiment.RegabiWrappers {
		return name
	}

	if !ldr.IsExternal(x) && ldr.SymType(x) == sym.STEXT && ldr.SymVersion(x) != sym.SymVerABIInternal {
		if s2 := ldr.Lookup(name, sym.SymVerABIInternal); s2 != 0 && ldr.SymType(s2) == sym.STEXT {
			name = fmt.Sprintf("%s.abi%d", name, ldr.SymVersion(x))
		}
	}

	// When loading a shared library, if a symbol has only one ABI,
	// and the name is not mangled, we don't know what ABI it is.
	// So we always mangle ABIInternal function name in shared linkage,
	// except symbols that are exported to C. Type symbols are always
	// ABIInternal so they are not mangled.
	if ctxt.IsShared() {
		if ldr.SymType(x) == sym.STEXT && ldr.SymVersion(x) == sym.SymVerABIInternal && !ldr.AttrCgoExport(x) && !strings.HasPrefix(name, "type.") {
			name = fmt.Sprintf("%s.abiinternal", name)
		}
	}

	return name
}
