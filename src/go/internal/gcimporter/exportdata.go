// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file implements FindExportData.

package gcimporter

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

func readGopackHeader(r *bufio.Reader) (name string, size int, err error) {
	// ../../../cmd/internal/archive/archive.go:36
	// %-16s%-12d%-6d%-6d%-8o%-10d`\n // 注意最后面有个`加换行
	// name mtime uid gid mode size
	hdr := make([]byte, 16+12+6+6+8+10+2)
	_, err = io.ReadFull(r, hdr)
	if err != nil {
		return
	}
	// leave for debugging
	if false {
		fmt.Printf("header: %s", hdr)
	}
	s := strings.TrimSpace(string(hdr[16+12+6+6+8:][:10]))
	size, err = strconv.Atoi(s)
	if err != nil || hdr[len(hdr)-2] != '`' || hdr[len(hdr)-1] != '\n' {
		err = fmt.Errorf("invalid archive header")
		return
	}
	name = strings.TrimSpace(string(hdr[:16]))
	return
}

// FindExportData positions the reader r at the beginning of the
// export data section of an underlying GC-created object/archive
// file by reading from it. The reader must be positioned at the
// start of the file before calling this function. The hdr result
// is the string before the export data, either "$$" or "$$B".
//
/*
!<arch>
__.PKGDEF       0           0     0     644     12973     `
go object linux amd64 go1.17.2 X:regabiwrappers,regabig,regabireflect,regabidefer,regabiargs
build id "JjffeX1LVeBlM4SUl9WF/GWzcADjG4FIf4_g-HuDz"


$$B
....
*/
func FindExportData(r *bufio.Reader) (hdr string, err error) {
	// Read first line to make sure this is an object file.
	line, err := r.ReadSlice('\n')
	if err != nil {
		err = fmt.Errorf("can't find export data (%v)", err)
		return
	}

	if string(line) == "!<arch>\n" {
		// Archive file. Scan to __.PKGDEF.
		var name string
		if name, _, err = readGopackHeader(r); err != nil {
			return
		}

		// First entry should be __.PKGDEF.
		if name != "__.PKGDEF" {
			err = fmt.Errorf("go archive is missing __.PKGDEF")
			return
		}

		// Read first line of __.PKGDEF data, so that line
		// is once again the first line of the input.
		if line, err = r.ReadSlice('\n'); err != nil {
			err = fmt.Errorf("can't find export data (%v)", err)
			return
		}
	}

	// 第一行是: go object linux amd64 go1.17.2 X:regabiwrappers,regabig,regabireflect,regabidefer,regabiargs
	// 注意这里列出的是所有enabled experiment，不是和默认值不一样的experiment
	// Now at __.PKGDEF in archive or still at beginning of file.
	// Either way, line should begin with "go object ".
	if !strings.HasPrefix(string(line), "go object ") {
		err = fmt.Errorf("not a Go object file")
		return
	}

	// Skip over object header to export data.
	// Begins after first line starting with $$.
	for line[0] != '$' {
		if line, err = r.ReadSlice('\n'); err != nil {
			err = fmt.Errorf("can't find export data (%v)", err)
			return
		}
	}
	// $$B\n
	hdr = string(line)

	return
}
