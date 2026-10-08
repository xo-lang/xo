#include "textflag.h"

// func curG() uintptr
TEXT ·curG(SB),NOSPLIT,$0-8
	MOVQ	(TLS), AX
	MOVQ	AX, ret+0(FP)
	RET
