// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pprof

// A protobuf is a simple protocol buffer encoder.
type protobuf struct {
	data []byte
	tmp  [16]byte
	nest int
}

func (b *protobuf) varint(x uint64) {
	for x >= 128 {
		b.data = append(b.data, byte(x)|0x80)
		x >>= 7
	}
	b.data = append(b.data, byte(x))
}

func (b *protobuf) length(tag int, len int) {
	b.varint(uint64(tag)<<3 | 2)
	b.varint(uint64(len))
}

func (b *protobuf) uint64(tag int, x uint64) {
	// append varint to b.data
	b.varint(uint64(tag)<<3 | 0)
	b.varint(x)
}

func (b *protobuf) uint64s(tag int, x []uint64) {
	// varint编码, 数据长度不定。而长度不定导致写入长度本身要占用的字节数也不定
	// 先编码数据，再编码长度。然后把数据后移，再长度移到开头。
	// TODO(mzh): 可以预估长度字段本身占用的字节数，如果正确则不用复制

	// Use packed encoding
	if len(x) > 2 {
		// Use packed encoding
		n1 := len(b.data)
		for _, u := range x {
			b.varint(u)
		}
		n2 := len(b.data)
		b.length(tag, n2-n1)
		n3 := len(b.data)
		copy(b.tmp[:], b.data[n2:n3])
		copy(b.data[n1+(n3-n2):], b.data[n1:n2])
		copy(b.data[n1:], b.tmp[:n3-n2])
		return
	}
	for _, u := range x {
		b.uint64(tag, u)
	}
}

func (b *protobuf) uint64Opt(tag int, x uint64) {
	if x == 0 {
		return
	}
	b.uint64(tag, x)
}

func (b *protobuf) int64(tag int, x int64) {
	u := uint64(x)
	b.uint64(tag, u)
}

func (b *protobuf) int64Opt(tag int, x int64) {
	if x == 0 {
		return
	}
	b.int64(tag, x)
}

func (b *protobuf) int64s(tag int, x []int64) {
	if len(x) > 2 {
		// Use packed encoding
		n1 := len(b.data)
		for _, u := range x {
			b.varint(uint64(u))
		}
		n2 := len(b.data)
		b.length(tag, n2-n1)
		n3 := len(b.data)
		copy(b.tmp[:], b.data[n2:n3])
		copy(b.data[n1+(n3-n2):], b.data[n1:n2])
		copy(b.data[n1:], b.tmp[:n3-n2])
		return
	}
	for _, u := range x {
		b.int64(tag, u)
	}
}

func (b *protobuf) string(tag int, x string) {
	b.length(tag, len(x))
	b.data = append(b.data, x...)
}

func (b *protobuf) strings(tag int, x []string) {
	for _, s := range x {
		b.string(tag, s)
	}
}

func (b *protobuf) stringOpt(tag int, x string) {
	if x == "" {
		return
	}
	b.string(tag, x)
}

func (b *protobuf) bool(tag int, x bool) {
	if x {
		b.uint64(tag, 1)
	} else {
		b.uint64(tag, 0)
	}
}

func (b *protobuf) boolOpt(tag int, x bool) {
	if x == false {
		return
	}
	b.bool(tag, x)
}

type msgOffset int

func (b *protobuf) startMessage() msgOffset {
	b.nest++
	return msgOffset(len(b.data))
}

func (b *protobuf) endMessage(tag int, start msgOffset) {
	n1 := int(start)
	n2 := len(b.data)
	b.length(tag, n2-n1)
	n3 := len(b.data)
	copy(b.tmp[:], b.data[n2:n3])
	copy(b.data[n1+(n3-n2):], b.data[n1:n2])
	copy(b.data[n1:], b.tmp[:n3-n2])
	b.nest--
}
