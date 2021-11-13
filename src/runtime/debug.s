#include "go_asm.h"
#include "go_tls.h"
#include "funcdata.h"
#include "textflag.h"

// func GetFS() uintptr
TEXT runtime·GetFS(SB),NOSPLIT,$0-0
	MOVQ    TLS, CX
	MOVQ	0(CX)(TLS*1), AX
	MOVQ    AX, ret+0(FP)

	RET
