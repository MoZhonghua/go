package ld

import (
	"cmd/link/internal/loader"
	"cmd/link/internal/sym"
	"fmt"
)

func indent2(prefix string) string {
	s := ""
	for i := 0; i < len(prefix)+2; i++ {
		s += " "
	}
	return s
}

func (ctxt *Link) Dumpsymname(prefix string, name string) {
	sym := ctxt.loader.Lookup(name, 0)
	ctxt.Dumpsym(prefix, sym)
}

func (ctxt *Link) Dumpsym(prefix string, sym loader.Sym) {
	ldr := ctxt.loader
	if sym == 0 {
		fmt.Printf("%vsym = 0\n", prefix)
	}
	name := ldr.SymName(sym)
	value := ldr.SymValue(sym)
	ty := ldr.SymType(sym)
	siz := ldr.SymSize(sym)
	ver := ldr.SymVersion(sym)

	flags := ""
	if !ldr.AttrReachable(sym) {
		flags += "dead "
	} else {
		flags += "live "
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

	if ename := ldr.SymExtname(sym); ename != "" && ename != name {
		flags += "extname=" + name + " "
	}

	if sub := ldr.SubSym(sym); sub != 0 {
		flags += fmt.Sprintf("sub=%d ", sub)
	}

	if out := ldr.OuterSym(sym); out != 0 {
		flags += fmt.Sprintf("out=%d ", out)
	}

	fmt.Printf("%v%v: val=%x, siz=%v, ty=%v, ver=%d", prefix, name, value, siz, ty, ver)
	sec := ldr.SymSect(sym)
	if sec != nil {
		fmt.Printf(", sec=%v, vaddr=%x\n", sec.Name, sec.Vaddr)
	} else {
		fmt.Printf(", sec=nil\n")
	}
	fmt.Printf("%vflags: %v\n", prefix, flags)
}

func dumpreloc(prefix string, ldr *loader.Loader, r loader.Reloc, from loader.Sym) {
	addr := ldr.SymValue(from) + int64(r.Off())

	fmt.Printf("%vreloc: off=%v (%x), size=%v, ref=%v, add=%v\n", prefix, r.Off(), addr, r.Siz(), r.Sym(), r.Add())
	// dumpsym(prefix + "  ", ldr, r.Sym())
}

func dumpelfshdr(prefix string, s *ElfShdr) {
	fmt.Printf("%v%+v\n", prefix, s)
}

func dumpsegs() {
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
		fmt.Printf("Segment %v: nsections = %d; fileoff = %x; filelen = %x\n",
			segnames[i], len(seg.Sections), seg.Fileoff, seg.Filelen)

		for _, sec := range seg.Sections {
			off := sec.Vaddr - seg.Vaddr + seg.Fileoff
			fmt.Printf("  section %v: len = %x vaddr = %x; fileoff = %x\n", sec.Name, sec.Length, sec.Vaddr, off)
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
