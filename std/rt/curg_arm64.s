#include "textflag.h"

// func curG() uintptr
TEXT ·curG(SB),NOSPLIT,$0-8
	MOVD	g, R0
	MOVD	R0, ret+0(FP)
	RET
