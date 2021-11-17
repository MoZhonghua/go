// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Declarations for operating systems implementing time.now directly in assembly.

//go:build !faketime && (windows || (linux && amd64))
// +build !faketime
// +build windows linux,amd64

package runtime

import _ "unsafe"

//go:linkname time_now time.now

// syscall clock_gettime(CLOCK_REALTIME, &timespec{})
// sec = timespec.tv_sec
// nsec = timespec.tv_nsec
// mono = nanotime1() = CLOCK_MONOTONIC tv_sec * 10e6 + tv_nsec
func time_now() (sec int64, nsec int32, mono int64)
