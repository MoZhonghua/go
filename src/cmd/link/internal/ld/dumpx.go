package ld

import (
	"cmd/link/internal/loader"
	"cmd/link/internal/sym"
	"fmt"
)

func indent2(prefix string) string {
	prefixSpaceLen := 0
	for i := 0; i < len(prefix); i++ {
		if prefix[i] != ' ' {
			break
		}
		prefixSpaceLen += 1
	}
	s := ""
	for i := 0; i < prefixSpaceLen+4; i++ {
		s += " "
	}
	return s
}

func (ctxt *Link) Dumpsymname(prefix string, name string) {
	if ctxt.loader == nil {
		fmt.Printf("%vsym = %v\n", prefix, name)
		return
	}
	sym := ctxt.loader.Lookup(name, 0)
	ctxt.Dumpsym(prefix, sym)
}

func (ctxt *Link) Dumpsym(prefix string, sym loader.Sym) {
	ldr := ctxt.loader
	if ldr == nil || sym == 0 || int(sym) >= ldr.NSym() {
		fmt.Printf("%vsym = %d\n", prefix, sym)
		return
	}
	name := ldr.SymName(sym)
	value := ldr.SymValue(sym)
	ty := ldr.SymType(sym)
	siz := ldr.SymSize(sym)
	ver := ldr.SymVersion(sym)
	relocs := ldr.Relocs(sym)

	flags := ""
	if !ldr.AttrReachable(sym) {
		flags += "dead "
	} else {
		flags += "reachable "
	}

	if ldr.AttrLocal(sym) {
		flags += "local "
	}

	if ldr.AttrSubSymbol(sym) {
		flags += "sub "
	}

	if ldr.AttrUsedInIface(sym) {
		flags += "use_iface "
	}

	if ldr.AttrSpecial(sym) {
		flags += "special "
	}

	if ldr.AttrCgoExportDynamic(sym) {
		flags += "cgo_export_dynamic "
	}

	if ldr.AttrCgoExportStatic(sym) {
		flags += "cgo_export_static "
	}

	if ldr.AttrDuplicateOK(sym) {
		flags += "dupok "
	}

	if ldr.AttrExternal(sym) {
		flags += "external "
	}

	if ldr.AttrNotInSymbolTable(sym) {
		flags += "nosymtab "
	}

	if ename := ldr.SymExtname(sym); ename != "" && ename != name {
		flags += "extname=" + name + " "
	}

	if sub := ldr.SubSym(sym); sub != 0 {
		flags += fmt.Sprintf("sub=%d ", sub)
	}

	if out := ldr.OuterSym(sym); out != 0 {
		flags += fmt.Sprintf("out=%d ", out)
	}

	fmt.Printf("%v%v(%d): val=0x%x, siz=%v, ty=%v, ver=%d, relocs=%d",
		prefix, name, sym, value, siz, ty, ver, relocs.Count())
	sec := ldr.SymSect(sym)
	if sec != nil {
		fmt.Printf(", sec=%v, vaddr=0x%x\n", sec.Name, sec.Vaddr)
	} else {
		fmt.Printf(", sec=nil\n")
	}
	fmt.Printf("%vflags: %v\n", indent2(prefix), flags)

	for i := 0; i < relocs.Count(); i++ {
		r := relocs.At(i)
		fmt.Printf("%vreloc[%d]: t=%v, off=%v, add=%v, tgt=%d\n",
			indent2(prefix), i, r.Type().String(), r.Off(), r.Add(), r.Sym())
	}
}

func dumpreloc(prefix string, ldr *loader.Loader, r loader.Reloc, from loader.Sym) {
	addr := ldr.SymValue(from) + int64(r.Off())

	fmt.Printf("%vreloc: off=%v (%x), size=%v, ref=%v, add=%v\n", prefix, r.Off(), addr, r.Siz(), r.Sym(), r.Add())
	// dumpsym(prefix + "  ", ldr, r.Sym())
}

func dumpelfshdr(prefix string, s *ElfShdr) {
	fmt.Printf("%v%+v\n", prefix, s)
}

func (ctxt *Link) Dumpsegs(header string) {
	fmt.Printf("==================%v==============\n", header)
	var segnames = []string{
		"Segtext",
		"Segrodata",
		"Segrelrodata",
		"Segdata",
		"Segdwarf",
	}

	for i, seg := range []*sym.Segment{
		&Segtext,
		&Segrodata,
		&Segrelrodata,
		&Segdata,
		&Segdwarf,
	} {
		fmt.Printf("Segment %v: nsections = %d; fileoff = 0x%x; filelen = 0x%x\n",
			segnames[i], len(seg.Sections), seg.Fileoff, seg.Filelen)

		for _, sec := range seg.Sections {
			off := sec.Vaddr - seg.Vaddr + seg.Fileoff
			fmt.Printf("  section %v: len = 0x%x vaddr = 0x%x; fileoff = 0x%x\n", sec.Name, sec.Length, sec.Vaddr, off)
		}
	}
}

func dumpliblist(ctxt *Link) {
	fmt.Println("lib list:")
	for _, lib := range ctxt.Library {
		fmt.Printf("  lib: %v (ref=%v, units=%d) => %v\n", lib.Pkg, lib.Srcref, len(lib.Units), lib.File)
	}

	for _, h := range hostobj {
		fmt.Printf("  hostobj: %v\n", h.file)
	}
}
