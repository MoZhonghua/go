// Inferno utils/6l/asm.c
// https://bitbucket.org/inferno-os/inferno-os/src/master/utils/6l/asm.c
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

package amd64

import (
	"cmd/internal/objabi"
	"cmd/internal/sys"
	"cmd/link/internal/ld"
	"cmd/link/internal/loader"
	"cmd/link/internal/sym"
	"debug/elf"
	"log"
)

// amd64要求48位地址sign-extended
func PADDR(x uint32) uint32 {
	return x &^ 0x80000000
}

func gentext(ctxt *ld.Link, ldr *loader.Loader) {
	// addmoduledata是指向函数runtime.addmoduledata()
	// 在.init会有一项指向initfunc，我们要做的是构造initfunc函数体
	initfunc, addmoduledata := ld.PrepareAddmoduledata(ctxt)
	if initfunc == nil {
		return
	}

	o := func(op ...uint8) {
		for _, op1 := range op {
			initfunc.AddUint8(op1)
		}
	}

	// 0000000000000000 <local.dso_init>:
	//    0:	48 8d 3d 00 00 00 00 	lea    0x0(%rip),%rdi        # 7 <local.dso_init+0x7>
	// 			3: R_X86_64_PC32	runtime.firstmoduledata-0x4
	o(0x48, 0x8d, 0x3d)
	initfunc.AddPCRelPlus(ctxt.Arch, ctxt.Moduledata, 0)
	//    7:	e8 00 00 00 00       	callq  c <local.dso_init+0xc>
	// 			8: R_X86_64_PLT32	runtime.addmoduledata-0x4
	o(0xe8)
	initfunc.AddSymRef(ctxt.Arch, addmoduledata, 0, objabi.R_CALL, 4)
	//    c:	c3                   	retq
	o(0xc3)
}

// 调用方式:
//  - buildmode=pie && linkinternal 时对所有的go, elf reloc都调用这个转换
//    * 大部分都是转换为go reloc, 在link时处理
//    * 其他变成R_X86_64_RELATIVE类型的elf reloc，添加对应的.got, .rela, .symtab, .dynsym项
//  - 特定reloc: SymType(target) == sym.SDYNIMPORT || reloc.Type() >= objabi.ElfRelocOffsetr.Sym
//    * 引用的dso中sym，需要生成elf reloc
//    * reloc本身是elf reloc，也就是从host object中加载进来的reloc
//    * 只有GOT和PLT类型的reloc才能引用SDYNIMPORT sym
func adddynrel(target *ld.Target, ldr *loader.Loader, syms *ld.ArchSyms, s loader.Sym, r loader.Reloc, rIdx int) bool {
	targ := r.Sym()
	var targType sym.SymKind
	if targ != 0 {
		targType = ldr.SymType(targ)
	}

	switch rt := r.Type(); rt {
	default:
		if rt >= objabi.ElfRelocOffset {
			ldr.Errorf(s, "unexpected relocation type %d (%s)", r.Type(), sym.RelocName(target.Arch, r.Type()))
			return false
		}

		// Handle relocations found in ELF object files.
	case objabi.ElfRelocOffset + objabi.RelocType(elf.R_X86_64_PC32):
		// 能用ip-relative addressing的一定是定义在同一个.o中的，因此不可能是SDYNIMPORT
		if targType == sym.SDYNIMPORT {
			ldr.Errorf(s, "unexpected R_X86_64_PC32 relocation for dynamic symbol %s", ldr.SymName(targ))
		}
		// TODO(mwhudson): the test of VisibilityHidden here probably doesn't make
		// sense and should be removed when someone has thought about it properly.
		if (targType == 0 || targType == sym.SXREF) && !ldr.AttrVisibilityHidden(targ) {
			ldr.Errorf(s, "unknown symbol %s in pcrel", ldr.SymName(targ))
		}
		su := ldr.MakeSymbolUpdater(s)
		// 指令中相对pc的偏移量，ip-relative addressing是指令结束地址+offset, 因此+4
		su.SetRelocType(rIdx, objabi.R_PCREL)
		su.SetRelocAdd(rIdx, r.Add()+4)
		return true

	case objabi.ElfRelocOffset + objabi.RelocType(elf.R_X86_64_PC64):
		if targType == sym.SDYNIMPORT {
			ldr.Errorf(s, "unexpected R_X86_64_PC64 relocation for dynamic symbol %s", ldr.SymName(targ))
		}
		if targType == 0 || targType == sym.SXREF {
			ldr.Errorf(s, "unknown symbol %s in pcrel", ldr.SymName(targ))
		}
		su := ldr.MakeSymbolUpdater(s)
		su.SetRelocType(rIdx, objabi.R_PCREL)
		su.SetRelocAdd(rIdx, r.Add()+8)
		return true

	case objabi.ElfRelocOffset + objabi.RelocType(elf.R_X86_64_PLT32):
		// 通过PTL调用，一般都是SDYNIMPORT, 即引用的dso中的函数
		su := ldr.MakeSymbolUpdater(s)
		su.SetRelocType(rIdx, objabi.R_PCREL)
		su.SetRelocAdd(rIdx, r.Add()+4)
		if targType == sym.SDYNIMPORT {
			// call name1 => call name1@plt(%rip);
			addpltsym(target, ldr, syms, targ)
			su.SetRelocSym(rIdx, syms.PLT)
			su.SetRelocAdd(rIdx, r.Add()+int64(ldr.SymPlt(targ)))
		}

		return true

	case objabi.ElfRelocOffset + objabi.RelocType(elf.R_X86_64_GOTPCREL),
		objabi.ElfRelocOffset + objabi.RelocType(elf.R_X86_64_GOTPCRELX),
		objabi.ElfRelocOffset + objabi.RelocType(elf.R_X86_64_REX_GOTPCRELX):
		// REX => 指x86指令的REX prefix, 用在add name1@GOT(%rip), %rax这种指令中
		// opcode不是call，jump之类的
		su := ldr.MakeSymbolUpdater(s)
		if targType != sym.SDYNIMPORT {
			// x86-64-psABI v1.0: B.2 Optimize GOTPCRELX Relocations
			// 如果target不是外部sym，我们可以直接在link时计算出ip-relative offset，不用走GOT
			// have symbol
			sData := ldr.Data(s)
			// 48 8b 1d 8b 38 5f 00
			// mov 0x5f388b(%rip),%rbx
			// 转换为
			// 48 8d 1d 00 00 00 00
			// lea <ip_relative_offset_of_target>(%rip),%rbx
			if r.Off() >= 2 && sData[r.Off()-2] == 0x8b {
				su.MakeWritable()
				// turn MOVQ of GOT entry into LEAQ of symbol itself
				writeableData := su.Data()
				writeableData[r.Off()-2] = 0x8d
				su.SetRelocType(rIdx, objabi.R_PCREL)
				su.SetRelocAdd(rIdx, r.Add()+4)
				return true
			}
		}

		// fall back to using GOT and hope for the best (CMOV*)
		// TODO: just needs relocation, no need to put in .dynsym
		// 三个操作:
		//  - .dynsym 增加 target
		//  - .got写入一个8字节
		//  - .rela写入一个elf reloc
		//    * elf reloc的offset字段需要通过link-reloc来设置
		ld.AddGotSym(target, ldr, syms, targ, uint32(elf.R_X86_64_GLOB_DAT))

		su.SetRelocType(rIdx, objabi.R_PCREL)
		su.SetRelocSym(rIdx, syms.GOT)
		su.SetRelocAdd(rIdx, r.Add()+4+int64(ldr.SymGot(targ)))
		return true

	case objabi.ElfRelocOffset + objabi.RelocType(elf.R_X86_64_64):
		if targType == sym.SDYNIMPORT {
			ldr.Errorf(s, "unexpected R_X86_64_64 relocation for dynamic symbol %s", ldr.SymName(targ))
		}
		su := ldr.MakeSymbolUpdater(s)
		su.SetRelocType(rIdx, objabi.R_ADDR)
		if target.IsPIE() && target.IsInternal() {
			// For internal linking PIE, this R_ADDR relocation cannot
			// be resolved statically. We need to generate a dynamic
			// relocation. Let the code below handle it.
			// 注意上面已经把reloc type改成了R_ADDR
			break
		}
		return true

	// Handle relocations found in Mach-O object files.
	case objabi.MachoRelocOffset + ld.MACHO_X86_64_RELOC_UNSIGNED*2 + 0,
		objabi.MachoRelocOffset + ld.MACHO_X86_64_RELOC_SIGNED*2 + 0,
		objabi.MachoRelocOffset + ld.MACHO_X86_64_RELOC_BRANCH*2 + 0:
		su := ldr.MakeSymbolUpdater(s)
		su.SetRelocType(rIdx, objabi.R_ADDR)

		if targType == sym.SDYNIMPORT {
			ldr.Errorf(s, "unexpected reloc for dynamic symbol %s", ldr.SymName(targ))
		}
		if target.IsPIE() && target.IsInternal() {
			// For internal linking PIE, this R_ADDR relocation cannot
			// be resolved statically. We need to generate a dynamic
			// relocation. Let the code below handle it.
			if rt == objabi.MachoRelocOffset+ld.MACHO_X86_64_RELOC_UNSIGNED*2 {
				break
			} else {
				// MACHO_X86_64_RELOC_SIGNED or MACHO_X86_64_RELOC_BRANCH
				// Can this happen? The object is expected to be PIC.
				ldr.Errorf(s, "unsupported relocation for PIE: %v", rt)
			}
		}
		return true

	case objabi.MachoRelocOffset + ld.MACHO_X86_64_RELOC_BRANCH*2 + 1:
		if targType == sym.SDYNIMPORT {
			addpltsym(target, ldr, syms, targ)
			su := ldr.MakeSymbolUpdater(s)
			su.SetRelocSym(rIdx, syms.PLT)
			su.SetRelocType(rIdx, objabi.R_PCREL)
			su.SetRelocAdd(rIdx, int64(ldr.SymPlt(targ)))
			return true
		}
		fallthrough

	case objabi.MachoRelocOffset + ld.MACHO_X86_64_RELOC_UNSIGNED*2 + 1,
		objabi.MachoRelocOffset + ld.MACHO_X86_64_RELOC_SIGNED*2 + 1,
		objabi.MachoRelocOffset + ld.MACHO_X86_64_RELOC_SIGNED_1*2 + 1,
		objabi.MachoRelocOffset + ld.MACHO_X86_64_RELOC_SIGNED_2*2 + 1,
		objabi.MachoRelocOffset + ld.MACHO_X86_64_RELOC_SIGNED_4*2 + 1:
		su := ldr.MakeSymbolUpdater(s)
		su.SetRelocType(rIdx, objabi.R_PCREL)

		if targType == sym.SDYNIMPORT {
			ldr.Errorf(s, "unexpected pc-relative reloc for dynamic symbol %s", ldr.SymName(targ))
		}
		return true

	case objabi.MachoRelocOffset + ld.MACHO_X86_64_RELOC_GOT_LOAD*2 + 1:
		if targType != sym.SDYNIMPORT {
			// have symbol
			// turn MOVQ of GOT entry into LEAQ of symbol itself
			sdata := ldr.Data(s)
			if r.Off() < 2 || sdata[r.Off()-2] != 0x8b {
				ldr.Errorf(s, "unexpected GOT_LOAD reloc for non-dynamic symbol %s", ldr.SymName(targ))
				return false
			}

			su := ldr.MakeSymbolUpdater(s)
			su.MakeWritable()
			sdata = su.Data()
			sdata[r.Off()-2] = 0x8d
			su.SetRelocType(rIdx, objabi.R_PCREL)
			return true
		}
		fallthrough

	case objabi.MachoRelocOffset + ld.MACHO_X86_64_RELOC_GOT*2 + 1:
		if targType != sym.SDYNIMPORT {
			ldr.Errorf(s, "unexpected GOT reloc for non-dynamic symbol %s", ldr.SymName(targ))
		}
		ld.AddGotSym(target, ldr, syms, targ, 0)
		su := ldr.MakeSymbolUpdater(s)
		su.SetRelocType(rIdx, objabi.R_PCREL)
		su.SetRelocSym(rIdx, syms.GOT)
		su.SetRelocAdd(rIdx, r.Add()+int64(ldr.SymGot(targ)))
		return true
	}

	// Reread the reloc to incorporate any changes in type above.
	relocs := ldr.Relocs(s)
	r = relocs.At(rIdx)

	switch r.Type() {
	case objabi.R_CALL,
		objabi.R_PCREL:
		if targType != sym.SDYNIMPORT {
			// nothing to do, the relocation will be laid out in reloc
			return true
		}
		if target.IsExternal() {
			// External linker will do this relocation.
			return true
		}
		// Internal linking, for both ELF and Mach-O.
		// Build a PLT entry and change the relocation target to that entry.
		addpltsym(target, ldr, syms, targ)
		su := ldr.MakeSymbolUpdater(s)
		su.SetRelocSym(rIdx, syms.PLT)
		su.SetRelocAdd(rIdx, int64(ldr.SymPlt(targ)))
		return true

	case objabi.R_ADDR:
		// 一定是call *name1@GOT(%rip)?
		if ldr.SymType(s) == sym.STEXT && target.IsElf() {
			su := ldr.MakeSymbolUpdater(s)
			if target.IsSolaris() {
				addpltsym(target, ldr, syms, targ)
				su.SetRelocSym(rIdx, syms.PLT)
				su.SetRelocAdd(rIdx, r.Add()+int64(ldr.SymPlt(targ)))
				return true
			}
			// The code is asking for the address of an external
			// function. We provide it with the address of the
			// correspondent GOT symbol.
			ld.AddGotSym(target, ldr, syms, targ, uint32(elf.R_X86_64_GLOB_DAT))

			su.SetRelocSym(rIdx, syms.GOT)
			su.SetRelocAdd(rIdx, r.Add()+int64(ldr.SymGot(targ)))
			return true
		}

		// Process dynamic relocations for the data sections.
		if target.IsPIE() && target.IsInternal() {
			// When internally linking, generate dynamic relocations
			// for all typical R_ADDR relocations. The exception
			// are those R_ADDR that are created as part of generating
			// the dynamic relocations and must be resolved statically.
			//
			// There are three phases relevant to understanding this:
			//
			//	dodata()  // we are here
			//	address() // symbol address assignment
			//	reloc()   // resolution of static R_ADDR relocs
			//
			// At this point symbol addresses have not been
			// assigned yet (as the final size of the .rela section
			// will affect the addresses), and so we cannot write
			// the Elf64_Rela.r_offset now. Instead we delay it
			// until after the 'address' phase of the linker is
			// complete. We do this via Addaddrplus, which creates
			// a new R_ADDR relocation which will be resolved in
			// the 'reloc' phase.
			//
			// These synthetic static R_ADDR relocs must be skipped
			// now, or else we will be caught in an infinite loop
			// of generating synthetic relocs for our synthetic
			// relocs.
			//
			// Furthermore, the rela sections contain dynamic
			// relocations with R_ADDR relocations on
			// Elf64_Rela.r_offset. This field should contain the
			// symbol offset as determined by reloc(), not the
			// final dynamically linked address as a dynamic
			// relocation would provide.

			// 这些section中的数据在load时都不会修改，也就是不会有load-relocation来修改这些
			// 全部都是link-relocation来修改，比如.rela的offset字段. 不需要特别处理，后面的
			// reloc()阶段会填入正确值
			switch ldr.SymName(s) {
			case ".dynsym", ".rela", ".rela.plt", ".got.plt", ".dynamic":
				return false
			}
		} else { // !target.IsPIE() || !target.IsInternal()
			// Either internally linking a static executable,
			// in which case we can resolve these relocations
			// statically in the 'reloc' phase, or externally
			// linking, in which case the relocation will be
			// prepared in the 'reloc' phase and passed to the
			// external linker in the 'asmb' phase.
			if ldr.SymType(s) != sym.SDATA && ldr.SymType(s) != sym.SRODATA {
				break
			}
		}

		if target.IsElf() {
			// Generate R_X86_64_RELATIVE relocations for best
			// efficiency in the dynamic linker.
			//
			// As noted above, symbol addresses have not been
			// assigned yet, so we can't generate the final reloc
			// entry yet. We ultimately want:
			//
			// r_offset = s + r.Off
			// r_info = R_X86_64_RELATIVE
			// r_addend = targ + r.Add
			//
			// The dynamic linker will set *offset = base address +
			// addend.
			//
			// AddAddrPlus is used for r_offset and r_addend to
			// generate new R_ADDR relocations that will update
			// these fields in the 'reloc' phase.

			// 注意只是在.rela增加了一项，reloc本身没有修改
			rela := ldr.MakeSymbolUpdater(syms.Rela)
			rela.AddAddrPlus(target.Arch, s, int64(r.Off()))
			if r.Siz() == 8 {
				rela.AddUint64(target.Arch, elf.R_INFO(0, uint32(elf.R_X86_64_RELATIVE)))
			} else {
				ldr.Errorf(s, "unexpected relocation for dynamic symbol %s", ldr.SymName(targ))
			}
			rela.AddAddrPlus(target.Arch, targ, int64(r.Add()))
			// Not mark r done here. So we still apply it statically,
			// so in the file content we'll also have the right offset
			// to the relocation target. So it can be examined statically
			// (e.g. go version).
			return true
		}

		if target.IsDarwin() {
			// Mach-O relocations are a royal pain to lay out.
			// They use a compact stateful bytecode representation.
			// Here we record what are needed and encode them later.
			ld.MachoAddRebase(s, int64(r.Off()))
			// Not mark r done here. So we still apply it statically,
			// so in the file content we'll also have the right offset
			// to the relocation target. So it can be examined statically
			// (e.g. go version).
			return true
		}
	}

	return false
}


// ctxt.IsExternal()时调用

// ../ld/data.go:635 extreloc() 把loader.Reloc -> loader.ExtReloc
func elfreloc1(ctxt *ld.Link, out *ld.OutBuf, ldr *loader.Loader, s loader.Sym, r loader.ExtReloc, ri int, sectoff int64) bool {
	out.Write64(uint64(sectoff))

	elfsym := ld.ElfSymForReloc(ctxt, r.Xsym) //
	siz := r.Size
	switch r.Type {
	default:
		return false
	case objabi.R_ADDR, objabi.R_DWARFSECREF:
		if siz == 4 {
			out.Write64(uint64(elf.R_X86_64_32) | uint64(elfsym)<<32)
		} else if siz == 8 {
			out.Write64(uint64(elf.R_X86_64_64) | uint64(elfsym)<<32)
		} else {
			return false
		}
	case objabi.R_TLS_LE:
		if siz == 4 {
			out.Write64(uint64(elf.R_X86_64_TPOFF32) | uint64(elfsym)<<32)
		} else {
			return false
		}
	case objabi.R_TLS_IE:
		if siz == 4 {
			out.Write64(uint64(elf.R_X86_64_GOTTPOFF) | uint64(elfsym)<<32)
		} else {
			return false
		}
	case objabi.R_CALL:
		if siz == 4 {
			if ldr.SymType(r.Xsym) == sym.SDYNIMPORT {
				out.Write64(uint64(elf.R_X86_64_PLT32) | uint64(elfsym)<<32)
			} else {
				out.Write64(uint64(elf.R_X86_64_PC32) | uint64(elfsym)<<32)
			}
		} else {
			return false
		}
	case objabi.R_PCREL:
		if siz == 4 {
			if ldr.SymType(r.Xsym) == sym.SDYNIMPORT && ldr.SymElfType(r.Xsym) == elf.STT_FUNC {
				out.Write64(uint64(elf.R_X86_64_PLT32) | uint64(elfsym)<<32)
			} else {
				out.Write64(uint64(elf.R_X86_64_PC32) | uint64(elfsym)<<32)
			}
		} else {
			return false
		}
	case objabi.R_GOTPCREL:
		if siz == 4 {
			out.Write64(uint64(elf.R_X86_64_GOTPCREL) | uint64(elfsym)<<32)
		} else {
			return false
		}
	}

	out.Write64(uint64(r.Xadd))
	return true
}

func machoreloc1(arch *sys.Arch, out *ld.OutBuf, ldr *loader.Loader, s loader.Sym, r loader.ExtReloc, sectoff int64) bool {
	var v uint32

	rs := r.Xsym
	rt := r.Type

	if ldr.SymType(rs) == sym.SHOSTOBJ || rt == objabi.R_PCREL || rt == objabi.R_GOTPCREL || rt == objabi.R_CALL {
		if ldr.SymDynid(rs) < 0 {
			ldr.Errorf(s, "reloc %d (%s) to non-macho symbol %s type=%d (%s)", rt, sym.RelocName(arch, rt), ldr.SymName(rs), ldr.SymType(rs), ldr.SymType(rs))
			return false
		}

		v = uint32(ldr.SymDynid(rs))
		v |= 1 << 27 // external relocation
	} else {
		v = uint32(ldr.SymSect(rs).Extnum)
		if v == 0 {
			ldr.Errorf(s, "reloc %d (%s) to symbol %s in non-macho section %s type=%d (%s)", rt, sym.RelocName(arch, rt), ldr.SymName(rs), ldr.SymSect(rs).Name, ldr.SymType(rs), ldr.SymType(rs))
			return false
		}
	}

	switch rt {
	default:
		return false

	case objabi.R_ADDR:
		v |= ld.MACHO_X86_64_RELOC_UNSIGNED << 28

	case objabi.R_CALL:
		v |= 1 << 24 // pc-relative bit
		v |= ld.MACHO_X86_64_RELOC_BRANCH << 28

		// NOTE: Only works with 'external' relocation. Forced above.
	case objabi.R_PCREL:
		v |= 1 << 24 // pc-relative bit
		v |= ld.MACHO_X86_64_RELOC_SIGNED << 28
	case objabi.R_GOTPCREL:
		v |= 1 << 24 // pc-relative bit
		v |= ld.MACHO_X86_64_RELOC_GOT_LOAD << 28
	}

	switch r.Size {
	default:
		return false

	case 1:
		v |= 0 << 25

	case 2:
		v |= 1 << 25

	case 4:
		v |= 2 << 25

	case 8:
		v |= 3 << 25
	}

	out.Write32(uint32(sectoff))
	out.Write32(v)
	return true
}

func pereloc1(arch *sys.Arch, out *ld.OutBuf, ldr *loader.Loader, s loader.Sym, r loader.ExtReloc, sectoff int64) bool {
	var v uint32

	rs := r.Xsym
	rt := r.Type

	if ldr.SymDynid(rs) < 0 {
		ldr.Errorf(s, "reloc %d (%s) to non-coff symbol %s type=%d (%s)", rt, sym.RelocName(arch, rt), ldr.SymName(rs), ldr.SymType(rs), ldr.SymType(rs))
		return false
	}

	out.Write32(uint32(sectoff))
	out.Write32(uint32(ldr.SymDynid(rs)))

	switch rt {
	default:
		return false

	case objabi.R_DWARFSECREF:
		v = ld.IMAGE_REL_AMD64_SECREL

	case objabi.R_ADDR:
		if r.Size == 8 {
			v = ld.IMAGE_REL_AMD64_ADDR64
		} else {
			v = ld.IMAGE_REL_AMD64_ADDR32
		}

	case objabi.R_CALL,
		objabi.R_PCREL:
		v = ld.IMAGE_REL_AMD64_REL32
	}

	out.Write16(uint16(v))

	return true
}

func archreloc(*ld.Target, *loader.Loader, *ld.ArchSyms, loader.Reloc, loader.Sym, int64) (int64, int, bool) {
	return -1, 0, false
}

func archrelocvariant(*ld.Target, *loader.Loader, loader.Reloc, sym.RelocVariant, loader.Sym, int64, []byte) int64 {
	log.Fatalf("unexpected relocation variant")
	return -1
}


// 只设置了PLT[0]的代码。其他对应函数的PLT项在后面按照需要创建
func elfsetupplt(ctxt *ld.Link, plt, got *loader.SymbolBuilder, dynamic loader.Sym) {
	if plt.Size() == 0 {
/* objdump -d -j .plt /tmp/main  // -buildmode=pie
00000000004971a0 <.plt>:
  // .PLT0
  4971a0:       ff 35 82 9e 0d 00       push   0xd9e82(%rip)        # 571028 <runtime.epclntab+0x320>
  4971a6:       ff 25 84 9e 0d 00       jmp    *0xd9e84(%rip)        # 571030 <runtime.epclntab+0x328>
  4971ac:       0f 1f 40 00             nopl   0x0(%rax)

.PLT0: pushq GOT+8(%rip)  // GOT[1] loader填写的，标识符，具体含义?
       jmp *GOT+16(%rip)  // 跳转到GOT[2]，也就是到loader代码。也是由loader填写, 会设置GOT[$index1]指向name1实际地址
       nopl

.PLT1: jmp *name1@GOTPCREL(%rip) // lazy-reloc; 初始值指向pushq $index1这条指令。reloc之后指向name1实际地址
       pushq $index1  //这个是name1这个exported函数对应的GOT项index
	   jmp .PLT0
*/
		// pushq got+8(IP)
		plt.AddUint8(0xff)

		plt.AddUint8(0x35)
		plt.AddPCRelPlus(ctxt.Arch, got.Sym(), 8) // .got.plt + 8; .got.plt[1]

		// jmpq got+16(IP)
		plt.AddUint8(0xff)

		plt.AddUint8(0x25)
		plt.AddPCRelPlus(ctxt.Arch, got.Sym(), 16) // .got.plt + 16; .got.plt[2]

		// nopl 0(AX)
		plt.AddUint32(ctxt.Arch, 0x00401f0f)

		// .got.plt中增加三项。第一项应该指向.dynamic对象，对应PT_DYNAMIC program header
		// assume got->size == 0 too
		got.AddAddrPlus(ctxt.Arch, dynamic, 0)

		got.AddUint64(ctxt.Arch, 0)
		got.AddUint64(ctxt.Arch, 0)
	}
}

// .plt中增加一项, 涉及到4个section
//  - .dynsym中创建s的elf sym entry，且设置ldr.SetSymDynid(s, dynid)
//  - .plt中创建一项，且设置ldr.SetPlt(target, plt_offset)
//  - .got.plt中写入8字节，通过link-relocation指向.plt项的起始位置
//  - .rela.plt中写入一项elf reloc:
//      - off -> vaddr of .got.plt entry when load, 通过link-relocation填写
//      - sym_idx -> dynid
//      - type = R_X86_64_JMP_SLOT
//      - addend =  0
func addpltsym(target *ld.Target, ldr *loader.Loader, syms *ld.ArchSyms, s loader.Sym) {
	if ldr.SymPlt(s) >= 0 {
		return
	}

	ld.Adddynsym(ldr, target, syms, s)

	if target.IsElf() {
		// <<System V Application Binary Interface AMD64 Architecture Processor Supplement>> p79
		// procedure linkage table

		// 只需要访问c runtime函数时需要通过PLT，因此-buildmode=pie会有些项，但是不会太多(41个)
		// 比如: abort@GLIBC_2.2.5, pthread_attr_destroy@GLIBC_2.2.5
		// objdump -d -j .plt /tmp/main

		plt := ldr.MakeSymbolUpdater(syms.PLT)
		got := ldr.MakeSymbolUpdater(syms.GOTPLT)
		rela := ldr.MakeSymbolUpdater(syms.RelaPLT)
		if plt.Size() == 0 {
			panic("plt is not set up")
		}

		// jmpq *got+size(IP)
		plt.AddUint8(0xff)

		plt.AddUint8(0x25)
		plt.AddPCRelPlus(target.Arch, got.Sym(), got.Size())

		// add to got: pointer to current pos in plt
		got.AddAddrPlus(target.Arch, plt.Sym(), plt.Size())

		// pushq $x; $x为s在GOT中对应项的index
		plt.AddUint8(0x68)

		// -24: GOT[0,1,2]保留, 含义见上面
		// -8: 现在指向的当前项的结束地址，需要的是起始地址，-8字节
		plt.AddUint32(target.Arch, uint32((got.Size()-24-8)/8))

		// jmpq .plt
		plt.AddUint8(0xe9)

		plt.AddUint32(target.Arch, uint32(-(plt.Size() + 4))) // IP-relative地址，指向PLT[0]

		// 增加一个reloc项，lazy-reloc时填写, GOT[$x]指向程序加载后s的绝对地址
		// In relocatable files, r_offset holds a section offset
		// In executable and shared object files, r_offset holds a virtual address
		rela.AddAddrPlus(target.Arch, got.Sym(), got.Size()-8)

		sDynid := ldr.SymDynid(s)
		rela.AddUint64(target.Arch, elf.R_INFO(uint32(sDynid), uint32(elf.R_X86_64_JMP_SLOT)))
		rela.AddUint64(target.Arch, 0)

		ldr.SetPlt(s, int32(plt.Size()-16))
	} else if target.IsDarwin() {
		ld.AddGotSym(target, ldr, syms, s, 0)

		sDynid := ldr.SymDynid(s)
		lep := ldr.MakeSymbolUpdater(syms.LinkEditPLT)
		lep.AddUint32(target.Arch, uint32(sDynid))

		plt := ldr.MakeSymbolUpdater(syms.PLT)
		ldr.SetPlt(s, int32(plt.Size()))

		// jmpq *got+size(IP)
		plt.AddUint8(0xff)
		plt.AddUint8(0x25)
		plt.AddPCRelPlus(target.Arch, syms.GOT, int64(ldr.SymGot(s)))
	} else {
		ldr.Errorf(s, "addpltsym: unsupported binary format")
	}
}

func tlsIEtoLE(P []byte, off, size int) {
	// Transform the PC-relative instruction into a constant load.
	// That is,
	//
	//	MOVQ X(IP), REG  ->  MOVQ $Y, REG
	//
	// To determine the instruction and register, we study the op codes.
	// Consult an AMD64 instruction encoding guide to decipher this.

	// 修改的是操作符，真正的relocation地址没有改
	// 4c8b3500000000          MOVQ $0(IP), R14  // 原始值，IE模式
	// 49c7c600000000          MOVQ $0, R14      // 转换为LE模式
	// 49c7c6f8ffffff          MOVQ $-0x8, R14   // 经过relocation
	if off < 3 {
		log.Fatal("R_X86_64_GOTTPOFF reloc not preceded by MOVQ or ADDQ instruction")
	}
	op := P[off-3 : off] // op=[49, c7, c6]
	reg := op[2] >> 3

	if op[1] == 0x8b || reg == 4 {
		// MOVQ
		if op[0] == 0x4c {
			op[0] = 0x49
		} else if size == 4 && op[0] == 0x44 {
			op[0] = 0x41
		}
		if op[1] == 0x8b {
			op[1] = 0xc7
		} else {
			op[1] = 0x81 // special case for SP
		}
		op[2] = 0xc0 | reg
	} else {
		// An alternate op is ADDQ. This is handled by GNU gold,
		// but right now is not generated by the Go compiler:
		//	ADDQ X(IP), REG  ->  ADDQ $Y, REG
		// Consider adding support for it here.
		log.Fatalf("expected TLS IE op to be MOVQ, got %v", op)
	}
}
