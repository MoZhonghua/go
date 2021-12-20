// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"runtime/internal/atomic"
	"unsafe"
)

// GOMAXPROCS sets the maximum number of CPUs that can be executing
// simultaneously and returns the previous setting. It defaults to
// the value of runtime.NumCPU. If n < 1, it does not change the current setting.
// This call will go away when the scheduler improves.
func GOMAXPROCS(n int) int {
	if GOARCH == "wasm" && n > 1 {
		n = 1 // WebAssembly has no threads yet, so only one CPU is possible.
	}

	lock(&sched.lock)
	ret := int(gomaxprocs)
	unlock(&sched.lock)
	if n <= 0 || n == ret {
		return ret
	}

	stopTheWorldGC("GOMAXPROCS")

	// newprocs will be processed by startTheWorld
	newprocs = int32(n)

	startTheWorldGC()
	return ret
}

// NumCPU returns the number of logical CPUs usable by the current process.
//
// The set of available CPUs is checked by querying the operating system
// at process startup. Changes to operating system CPU allocation after
// process startup are not reflected.
func NumCPU() int {
	return int(ncpu)
}

// NumCgoCall returns the number of cgo calls made by the current process.
func NumCgoCall() int64 {
	var n = int64(atomic.Load64(&ncgocall))
	for mp := (*m)(atomic.Loadp(unsafe.Pointer(&allm))); mp != nil; mp = mp.alllink {
		n += int64(mp.ncgocall)
	}
	return n
}

// NumGoroutine returns the number of goroutines that currently exist.
func NumGoroutine() int {
	return int(gcount())
}

//go:linkname debug_modinfo runtime/debug.modinfo
func debug_modinfo() string {
	return modinfo
}

func GetFS() uintptr

func PrintCurrentG() {
	g := getg()
	println("g =", g)
}

func DebugTLS() {
	m1 := m{}

	m1Addr := uintptr(unsafe.Pointer(&m1))
	tls0Addr := uintptr(unsafe.Pointer(&m1.tls[0]))

	println("&m1 =", &m1)
	println("&tls[0] =", &m1.tls[0])

	println("tls[0] pos =", tls0Addr-m1Addr)
	println("tls[0] + 0xfffffffffffffff8 =", hex(tls0Addr+0xfffffffffffffff8))
}

func PrintBuildInfo() {
	println("runtime.buildVersion =", buildVersion)
	println("runtime.modinfo =", modinfo)
}

type markdebugdata struct {
	g       unsafe.Pointer
	g0      unsafe.Pointer
	gsignal unsafe.Pointer
	obj     uintptr
}

func (d *markdebugdata) needlog(gp *g) bool {
	if d.g != nil && d.g == unsafe.Pointer(gp) {
		return true
	}

	if d.g0 != nil && d.g0 == unsafe.Pointer(gp) {
		return true
	}

	if d.gsignal != nil && d.gsignal == unsafe.Pointer(gp) {
		return true
	}

	return false
}

var markdebug markdebugdata

func SetMarkDebug(obj uintptr) {
	g := getg()
	markdebug.g = unsafe.Pointer(g)
	if g.m != nil {
		markdebug.g0 = unsafe.Pointer(g.m.g0)
		markdebug.gsignal = unsafe.Pointer(g.m.gsignal)
	}
	markdebug.obj = obj
}

func Getg() unsafe.Pointer {
	return unsafe.Pointer(getg())
}
