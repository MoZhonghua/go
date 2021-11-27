// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// MakeFunc implementation.

package reflect

import (
	"internal/abi"
	"unsafe"
)

// 基本使用模式:
/*
func sum(args []reflect.Value) []reflect.Value {
	a, b := args[0], args[1]
	return []reflect.Value{reflect.ValueOf(a.Int() + b.Int())}
}
func makeSum(fptr interface{}) {
	fn := reflect.ValueOf(fptr).Elem()
	v := reflect.MakeFunc(fn.Type(), sum)
	fn.Set(v)
}
var intSum func(int, int) int
makeSum(&intSum)
intSum(100, 200)
*/

/*
栈扫描中的问题: 首先代码都是提前编译好的，也就是intSum本质还是一个funcval
funcval.fn指向静态的代码。正常的函数参数（包括入参和出参）都是固定的，而就是给定funcval.fn
值，args的布局和ptrmask都是固定的，编译器提前写好。

但是在这里，参数时不确定的，而且组合无穷多，而funcval.fn是一个固定值，因此
在栈扫描中不能直接编译器静态生成的argmap。

应该用MakeFunc()传入的fn.Type()的argmap，这个值应该写在某个地方，让栈扫描过程能
找到正确的argmap, 比如写到funcval.fn这个函数栈的固定位置(栈底)
*/

// makeFuncImpl is the closure value implementing the function
// returned by MakeFunc.
// The first three words of this type must be kept in sync with
// methodValue and runtime.reflectMethodValue.
// Any changes should be reflected in all three.
// 可以接收任意参数列表的closure，参数类型信息是在创建closure时记录下来
// 栈扫描要特别处理
// funcval只要求第一个字段指向代码，因此这个结构体本身可以作为一个funcval
type makeFuncImpl struct {
	makeFuncCtxt
	ftyp *funcType
	fn   func([]Value) []Value
}

// MakeFunc returns a new function of the given Type
// that wraps the function fn. When called, that new function
// does the following:
//
//	- converts its arguments to a slice of Values.
//	- runs results := fn(args).
//	- returns the results as a slice of Values, one per formal result.
//
// The implementation fn can assume that the argument Value slice
// has the number and type of arguments given by typ.
// If typ describes a variadic function, the final Value is itself
// a slice representing the variadic arguments, as in the
// body of a variadic function. The result Value slice returned by fn
// must have the number and type of results given by typ.
//
// The Value.Call method allows the caller to invoke a typed function
// in terms of Values; in contrast, MakeFunc allows the caller to implement
// a typed function in terms of Values.
//
// The Examples section of the documentation includes an illustration
// of how to use MakeFunc to build a swap function for different types.
//
func MakeFunc(typ Type, fn func(args []Value) (results []Value)) Value {
	if typ.Kind() != Func {
		panic("reflect: call of MakeFunc with non-Func type")
	}

	t := typ.common()
	ftyp := (*funcType)(unsafe.Pointer(t))

	// Indirect Go func value (dummy) to obtain
	// actual code address. (A Go func value is a pointer
	// to a C function pointer. https://golang.org/s/go11func.)
	// 这里是要取出code这个值
	// dummy = &funcval { fn = code, ... captured data ... }
	dummy := makeFuncStub
	code := **(**uintptr)(unsafe.Pointer(&dummy))

	// makeFuncImpl contains a stack map for use by the runtime
	_, _, abi := funcLayout(ftyp, nil)

	impl := &makeFuncImpl{
		makeFuncCtxt: makeFuncCtxt{
			fn:      code,
			stack:   abi.stackPtrs,
			argLen:  abi.stackCallArgsSize,
			regPtrs: abi.inRegPtrs,
		},
		ftyp: ftyp,
		fn:   fn,
	}

	return Value{t, unsafe.Pointer(impl), flag(Func)}
}

// TODO(mzh): fix doc: It expects a *callReflectFunc is wrong
// makeFuncStub is an assembly function that is the code half of
// the function returned from MakeFunc. It expects a *makeFuncImpl
// as its context register, and its job is to invoke callReflect(ctxt, frame)
// where ctxt is the context register and frame is a pointer to the first
// word in the passed-in argument frame.
func makeFuncStub()

// The first 3 words of this type must be kept in sync with
// makeFuncImpl and runtime.reflectMethodValue.
// Any changes should be reflected in all three.
type methodValue struct {
	makeFuncCtxt
	method int
	rcvr   Value
}

// makeMethodValue converts v from the rcvr+method index representation
// of a method value to an actual method func value, which is
// basically the receiver value with a special bit set, into a true
// func value - a value holding an actual func. The output is
// semantically equivalent to the input as far as the user of package
// reflect can tell, but the true func representation can be handled
// by code like Convert and Interface and Assign.
func makeMethodValue(op string, v Value) Value {
	if v.flag&flagMethod == 0 {
		panic("reflect: internal error: invalid use of makeMethodValue")
	}

	// Ignoring the flagMethod bit, v describes the receiver, not the method type.
	fl := v.flag & (flagRO | flagAddr | flagIndir)
	fl |= flag(v.typ.Kind())
	rcvr := Value{v.typ, v.ptr, fl}

	// v.Type returns the actual type of the method value.
	ftyp := (*funcType)(unsafe.Pointer(v.Type().(*rtype)))

	// Indirect Go func value (dummy) to obtain
	// actual code address. (A Go func value is a pointer
	// to a C function pointer. https://golang.org/s/go11func.)
	dummy := methodValueCall
	code := **(**uintptr)(unsafe.Pointer(&dummy))

	// methodValue contains a stack map for use by the runtime
	_, _, abi := funcLayout(ftyp, nil)
	fv := &methodValue{
		makeFuncCtxt: makeFuncCtxt{
			fn:      code,
			stack:   abi.stackPtrs,
			argLen:  abi.stackCallArgsSize,
			regPtrs: abi.inRegPtrs,
		},
		method: int(v.flag) >> flagMethodShift,
		rcvr:   rcvr,
	}

	// Cause panic if method is not appropriate.
	// The panic would still happen during the call if we omit this,
	// but we want Interface() and other operations to fail early.
	methodReceiver(op, fv.rcvr, fv.method)

	return Value{&ftyp.rtype, unsafe.Pointer(fv), v.flag&flagRO | flag(Func)}
}

// methodValueCall is an assembly function that is the code half of
// the function returned from makeMethodValue. It expects a *methodValue
// as its context register, and its job is to invoke callMethod(ctxt, frame)
// where ctxt is the context register and frame is a pointer to the first
// word in the passed-in argument frame.
func methodValueCall()

// This structure must be kept in sync with runtime.reflectMethodValue.
// Any changes should be reflected in all both.
type makeFuncCtxt struct {
	fn      uintptr
	stack   *bitVector // ptrmap for both stack args and results
	argLen  uintptr    // just args
	regPtrs abi.IntArgRegBitmap
}

// moveMakeFuncArgPtrs uses ctxt.regPtrs to copy integer pointer arguments
// in args.Ints to args.Ptrs where the GC can see them.
//
// This is similar to what reflectcallmove does in the runtime, except
// that happens on the return path, whereas this happens on the call path.
//
// nosplit because pointers are being held in uintptr slots in args, so
// having our stack scanned now could lead to accidentally freeing
// memory.
//go:nosplit
func moveMakeFuncArgPtrs(ctxt *makeFuncCtxt, args *abi.RegArgs) {
	for i, arg := range args.Ints {
		// Avoid write barriers! Because our write barrier enqueues what
		// was there before, we might enqueue garbage.
		if ctxt.regPtrs.Get(i) {
			*(*uintptr)(unsafe.Pointer(&args.Ptrs[i])) = arg
		} else {
			// We *must* zero this space ourselves because it's defined in
			// assembly code and the GC will scan these pointers. Otherwise,
			// there will be garbage here.
			*(*uintptr)(unsafe.Pointer(&args.Ptrs[i])) = 0
		}
	}
}
