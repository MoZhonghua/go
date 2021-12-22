// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package version

import (
	"bytes"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"fmt"
	"internal/xcoff"
	"io"
	"os"
)

// An exe is a generic interface to an OS executable (ELF, Mach-O, PE, XCOFF).
type exe interface {
	// Close closes the underlying file.
	Close() error

	// ReadData reads and returns up to size byte starting at virtual address addr.
	ReadData(addr, size uint64) ([]byte, error)

	// DataStart returns the writable data segment start address.
	DataStart() uint64
}

// openExe opens file and returns it as an exe.
func openExe(file string) (exe, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	data := make([]byte, 16)
	if _, err := io.ReadFull(f, data); err != nil {
		return nil, err
	}
	f.Seek(0, 0)
	if bytes.HasPrefix(data, []byte("\x7FELF")) {
		e, err := elf.NewFile(f)
		if err != nil {
			f.Close()
			return nil, err
		}
		return &elfExe{f, e}, nil
	}
	if bytes.HasPrefix(data, []byte("MZ")) {
		e, err := pe.NewFile(f)
		if err != nil {
			f.Close()
			return nil, err
		}
		return &peExe{f, e}, nil
	}
	if bytes.HasPrefix(data, []byte("\xFE\xED\xFA")) || bytes.HasPrefix(data[1:], []byte("\xFA\xED\xFE")) {
		e, err := macho.NewFile(f)
		if err != nil {
			f.Close()
			return nil, err
		}
		return &machoExe{f, e}, nil
	}
	if bytes.HasPrefix(data, []byte{0x01, 0xDF}) || bytes.HasPrefix(data, []byte{0x01, 0xF7}) {
		e, err := xcoff.NewFile(f)
		if err != nil {
			f.Close()
			return nil, err
		}
		return &xcoffExe{f, e}, nil

	}
	return nil, fmt.Errorf("unrecognized executable format")
}

// elfExe is the ELF implementation of the exe interface.
type elfExe struct {
	os *os.File
	f  *elf.File
}

func (x *elfExe) Close() error {
	return x.os.Close()
}

func (x *elfExe) ReadData(addr, size uint64) ([]byte, error) {
	for _, prog := range x.f.Progs {
		if prog.Vaddr <= addr && addr <= prog.Vaddr+prog.Filesz-1 {
			n := prog.Vaddr + prog.Filesz - addr
			if n > size {
				n = size
			}
			data := make([]byte, n)
			_, err := prog.ReadAt(data, int64(addr-prog.Vaddr))
			if err != nil {
				return nil, err
			}
			return data, nil
		}
	}
	return nil, fmt.Errorf("address not mapped")
}

/*
Section Headers:
  [Nr] Name              Type            Address          Off    Size   ES Flg Lk Inf Al
  [ 0]                   NULL            0000000000000000 000000 000000 00      0   0  0
  [ 1] .text             PROGBITS        0000000000401000 001000 054c28 00  AX  0   0 32
  [ 2] .rodata           PROGBITS        0000000000456000 056000 022741 00   A  0   0 32
  [ 3] .shstrtab         STRTAB          0000000000000000 078760 00017a 00      0   0  1
  [ 4] .typelink         PROGBITS        00000000004788e0 0788e0 0002c0 00   A  0   0 32
  [ 5] .itablink         PROGBITS        0000000000478ba0 078ba0 000008 00   A  0   0  8
  [ 6] .gosymtab         PROGBITS        0000000000478ba8 078ba8 000000 00   A  0   0  1
  [ 7] .gopclntab        PROGBITS        0000000000478bc0 078bc0 040440 00   A  0   0 32
  [ 8] .go.buildinfo     PROGBITS        00000000004b9000 0b9000 000020 00  WA  0   0 16
  [ 9] .noptrdata        PROGBITS        00000000004b9020 0b9020 001180 00  WA  0   0 32
  [10] .data             PROGBITS        00000000004ba1a0 0ba1a0 0021f0 00  WA  0   0 32
  [11] .bss              NOBITS          00000000004bc3a0 0bc3a0 02eb48 00  WA  0   0 32
  [12] .noptrbss         NOBITS          00000000004eaf00 0eaf00 005320 00  WA  0   0 32
  [13] .zdebug_abbrev    PROGBITS        00000000004f1000 0bd000 000119 00      0   0  1
  [14] .zdebug_line      PROGBITS        00000000004f1119 0bd119 0136f2 00      0   0  1
  [15] .zdebug_frame     PROGBITS        000000000050480b 0d080b 003be2 00      0   0  1
  [16] .debug_gdb_scripts PROGBITS        00000000005083ed 0d43ed 000049 00      0   0  1
  [17] .zdebug_info      PROGBITS        0000000000508436 0d4436 02150f 00      0   0  1
  [18] .zdebug_loc       PROGBITS        0000000000529945 0f5945 0116eb 00      0   0  1
  [19] .zdebug_ranges    PROGBITS        000000000053b030 107030 006906 00      0   0  1
  [20] .note.go.buildid  NOTE            0000000000400f9c 000f9c 000064 00   A  0   0  4
  [21] .symtab           SYMTAB          0000000000000000 10d938 0079b0 18     22  89  8
  [22] .strtab           STRTAB          0000000000000000 1152e8 006f69 00      0   0  1
Key to Flags:
  W (write), A (alloc), X (execute)

Program Headers:
  Type           Offset   VirtAddr           PhysAddr           FileSiz  MemSiz   Flg Align
  PHDR           0x000040 0x0000000000400040 0x0000000000400040 0x000188 0x000188 R   0x1000
  NOTE           0x000f9c 0x0000000000400f9c 0x0000000000400f9c 0x000064 0x000064 R   0x4
  LOAD           0x000000 0x0000000000400000 0x0000000000400000 0x055c28 0x055c28 R E 0x1000
  LOAD           0x056000 0x0000000000456000 0x0000000000456000 0x063000 0x063000 R   0x1000
  LOAD           0x0b9000 0x00000000004b9000 0x00000000004b9000 0x0033a0 0x037220 RW  0x1000
  GNU_STACK      0x000000 0x0000000000000000 0x0000000000000000 0x000000 0x000000 RW  0x8
  LOOS+0x5041580 0x000000 0x0000000000000000 0x0000000000000000 0x000000 0x000000     0x8
*/
func (x *elfExe) DataStart() uint64 {
	// ../../../link/internal/ld/data.go:2270
	// 固定为32字节，包含两个指向runtime.buildVersion和runtime.modinfo的指针
	// 注意指针值为link-relocation后的值，也就是vaddr，不是文件offset
	for _, s := range x.f.Sections {
		if s.Name == ".go.buildinfo" { // 总是在Segdata第一个section
			return s.Addr
		}
	}
	for _, p := range x.f.Progs {
		if p.Type == elf.PT_LOAD && p.Flags&(elf.PF_X|elf.PF_W) == elf.PF_W {
			return p.Vaddr
		}
	}
	return 0
}

// peExe is the PE (Windows Portable Executable) implementation of the exe interface.
type peExe struct {
	os *os.File
	f  *pe.File
}

func (x *peExe) Close() error {
	return x.os.Close()
}

func (x *peExe) imageBase() uint64 {
	switch oh := x.f.OptionalHeader.(type) {
	case *pe.OptionalHeader32:
		return uint64(oh.ImageBase)
	case *pe.OptionalHeader64:
		return oh.ImageBase
	}
	return 0
}

func (x *peExe) ReadData(addr, size uint64) ([]byte, error) {
	addr -= x.imageBase()
	for _, sect := range x.f.Sections {
		if uint64(sect.VirtualAddress) <= addr && addr <= uint64(sect.VirtualAddress+sect.Size-1) {
			n := uint64(sect.VirtualAddress+sect.Size) - addr
			if n > size {
				n = size
			}
			data := make([]byte, n)
			_, err := sect.ReadAt(data, int64(addr-uint64(sect.VirtualAddress)))
			if err != nil {
				return nil, err
			}
			return data, nil
		}
	}
	return nil, fmt.Errorf("address not mapped")
}

func (x *peExe) DataStart() uint64 {
	// Assume data is first writable section.
	const (
		IMAGE_SCN_CNT_CODE               = 0x00000020
		IMAGE_SCN_CNT_INITIALIZED_DATA   = 0x00000040
		IMAGE_SCN_CNT_UNINITIALIZED_DATA = 0x00000080
		IMAGE_SCN_MEM_EXECUTE            = 0x20000000
		IMAGE_SCN_MEM_READ               = 0x40000000
		IMAGE_SCN_MEM_WRITE              = 0x80000000
		IMAGE_SCN_MEM_DISCARDABLE        = 0x2000000
		IMAGE_SCN_LNK_NRELOC_OVFL        = 0x1000000
		IMAGE_SCN_ALIGN_32BYTES          = 0x600000
	)
	for _, sect := range x.f.Sections {
		if sect.VirtualAddress != 0 && sect.Size != 0 &&
			sect.Characteristics&^IMAGE_SCN_ALIGN_32BYTES == IMAGE_SCN_CNT_INITIALIZED_DATA|IMAGE_SCN_MEM_READ|IMAGE_SCN_MEM_WRITE {
			return uint64(sect.VirtualAddress) + x.imageBase()
		}
	}
	return 0
}

// machoExe is the Mach-O (Apple macOS/iOS) implementation of the exe interface.
type machoExe struct {
	os *os.File
	f  *macho.File
}

func (x *machoExe) Close() error {
	return x.os.Close()
}

func (x *machoExe) ReadData(addr, size uint64) ([]byte, error) {
	for _, load := range x.f.Loads {
		seg, ok := load.(*macho.Segment)
		if !ok {
			continue
		}
		if seg.Addr <= addr && addr <= seg.Addr+seg.Filesz-1 {
			if seg.Name == "__PAGEZERO" {
				continue
			}
			n := seg.Addr + seg.Filesz - addr
			if n > size {
				n = size
			}
			data := make([]byte, n)
			_, err := seg.ReadAt(data, int64(addr-seg.Addr))
			if err != nil {
				return nil, err
			}
			return data, nil
		}
	}
	return nil, fmt.Errorf("address not mapped")
}

func (x *machoExe) DataStart() uint64 {
	// Look for section named "__go_buildinfo".
	for _, sec := range x.f.Sections {
		if sec.Name == "__go_buildinfo" {
			return sec.Addr
		}
	}
	// Try the first non-empty writable segment.
	const RW = 3
	for _, load := range x.f.Loads {
		seg, ok := load.(*macho.Segment)
		if ok && seg.Addr != 0 && seg.Filesz != 0 && seg.Prot == RW && seg.Maxprot == RW {
			return seg.Addr
		}
	}
	return 0
}

// xcoffExe is the XCOFF (AIX eXtended COFF) implementation of the exe interface.
type xcoffExe struct {
	os *os.File
	f  *xcoff.File
}

func (x *xcoffExe) Close() error {
	return x.os.Close()
}

func (x *xcoffExe) ReadData(addr, size uint64) ([]byte, error) {
	for _, sect := range x.f.Sections {
		if uint64(sect.VirtualAddress) <= addr && addr <= uint64(sect.VirtualAddress+sect.Size-1) {
			n := uint64(sect.VirtualAddress+sect.Size) - addr
			if n > size {
				n = size
			}
			data := make([]byte, n)
			_, err := sect.ReadAt(data, int64(addr-uint64(sect.VirtualAddress)))
			if err != nil {
				return nil, err
			}
			return data, nil
		}
	}
	return nil, fmt.Errorf("address not mapped")
}

func (x *xcoffExe) DataStart() uint64 {
	return x.f.SectionByType(xcoff.STYP_DATA).VirtualAddress
}
