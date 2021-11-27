// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include "textflag.h"
#include "funcdata.h"
#include "go_asm.h"

// The frames of each of the two functions below contain two locals, at offsets
// that are known to the runtime.
//
// The first local is a bool called retValid with a whole pointer-word reserved
// for it on the stack. The purpose of this word is so that the runtime knows
// whether the stack-allocated return space contains valid values for stack
// scanning.
//
// The second local is an abi.RegArgs value whose offset is also known to the
// runtime, so that a stack map for it can be constructed, since it contains
// pointers visible to the GC.
#define LOCAL_RETVALID 32
#define LOCAL_REGARGS 40

// makeFuncStub is the code half of the function returned by MakeFunc.
// See the comment on the declaration of makeFuncStub in makefunc.go
// for more details.
// No arg size here; runtime pulls arg map out of the func value.
// makeFuncStub must be ABIInternal because it is placed directly
// in function values.
// This frame contains two locals. See the comment above LOCAL_RETVALID.
//
// 栈布局为:
//
// [ args     ] <- fp: makeFuncStub本身声明是没有参数，但是调用MakeFunc()生成的closure有参数a
// [ ret      ]
// [ bp       ]
// [ RegArgs  ] sp+40  272字节
// [ RetValid ] sp+32
// [ &RegArgs ] sp+24  // 最后4个word是用来调用callReflect的参数
// [ &Retvalid] sp+16
// [ fp       ] sp+8
// [ CxtReg   ] sp+0  // &funcval, *makeFuncImpl
// func callReflect(ctxt *makeFuncImpl, frame unsafe.Pointer, retValid *bool, regs *abi.RegArgs)
TEXT ·makeFuncStub<ABIInternal>(SB),(NOSPLIT|WRAPPER),$312
	NO_LOCAL_POINTERS
	// NO_LOCAL_POINTERS is a lie. The stack map for the two locals in this
	// frame is specially handled in the runtime. See the comment above LOCAL_RETVALID.
	LEAQ	LOCAL_REGARGS(SP), R12
	CALL	runtime·spillArgs<ABIInternal>(SB)
	MOVQ	DX, 24(SP) // outside of moveMakeFuncArgPtrs's arg area
	MOVQ	DX, 0(SP)
	MOVQ	R12, 8(SP)
	CALL	·moveMakeFuncArgPtrs(SB)
	MOVQ	24(SP), DX
	MOVQ	DX, 0(SP)
	LEAQ	argframe+0(FP), CX
	MOVQ	CX, 8(SP)
	MOVB	$0, LOCAL_RETVALID(SP)
	LEAQ	LOCAL_RETVALID(SP), AX
	MOVQ	AX, 16(SP)
	LEAQ	LOCAL_REGARGS(SP), AX
	MOVQ	AX, 24(SP)
	CALL	·callReflect(SB)
	LEAQ	LOCAL_REGARGS(SP), R12
	CALL	runtime·unspillArgs<ABIInternal>(SB)
	RET

// methodValueCall is the code half of the function returned by makeMethodValue.
// See the comment on the declaration of methodValueCall in makefunc.go
// for more details.
// No arg size here; runtime pulls arg map out of the func value.
// methodValueCall must be ABIInternal because it is placed directly
// in function values.
// This frame contains two locals. See the comment above LOCAL_RETVALID.
// func callMethod(ctxt *methodValue, frame unsafe.Pointer, retValid *bool, regs *abi.RegArgs)
TEXT ·methodValueCall<ABIInternal>(SB),(NOSPLIT|WRAPPER),$312
	NO_LOCAL_POINTERS
	// NO_LOCAL_POINTERS is a lie. The stack map for the two locals in this
	// frame is specially handled in the runtime. See the comment above LOCAL_RETVALID.
	LEAQ	LOCAL_REGARGS(SP), R12
	CALL	runtime·spillArgs<ABIInternal>(SB)
	MOVQ	DX, 24(SP) // outside of moveMakeFuncArgPtrs's arg area
	MOVQ	DX, 0(SP)
	MOVQ	R12, 8(SP)
	CALL	·moveMakeFuncArgPtrs(SB)
	MOVQ	24(SP), DX
	MOVQ	DX, 0(SP)
	LEAQ	argframe+0(FP), CX
	MOVQ	CX, 8(SP)
	MOVB	$0, LOCAL_RETVALID(SP)
	LEAQ	LOCAL_RETVALID(SP), AX
	MOVQ	AX, 16(SP)
	LEAQ	LOCAL_REGARGS(SP), AX
	MOVQ	AX, 24(SP)
	CALL	·callMethod(SB)
	LEAQ	LOCAL_REGARGS(SP), R12
	CALL	runtime·unspillArgs<ABIInternal>(SB)
	RET
