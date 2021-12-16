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

func dumpsymname(prefix string, ldr *loader.Loader, name string) {
	sym := ldr.Lookup(name, 0)
	dumpsym(prefix, ldr, sym)
}

func dumpsym(prefix string, ldr *loader.Loader, sym loader.Sym) {
	if sym == 0 {
		fmt.Printf("%vsym = 0\n", prefix)
	}
	name := ldr.SymName(sym)
	value := ldr.SymValue(sym)
	ty := ldr.SymType(sym)
	siz := ldr.SymSize(sym)

	fmt.Printf("%v%v, val=%x, siz=%v, ty=%v", prefix, name, value, siz, ty)
	sec := ldr.SymSect(sym)
	if sec != nil {
		fmt.Printf(", sec=%v, vaddr=%x\n", sec.Name, sec.Vaddr)
	} else {
		fmt.Printf(", sec=nil\n")
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
		fmt.Printf("  %v (ref=%v, units=%d) => %v\n", lib.Pkg, lib.Srcref, len(lib.Units), lib.File)
	}
	for _, h := range hostobj {
		fmt.Printf("  hostobj: %v\n", h.file)
	}
}
