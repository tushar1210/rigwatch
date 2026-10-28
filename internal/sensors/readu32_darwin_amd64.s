//go:build darwin && amd64

#include "textflag.h"

TEXT ·readU32(SB), NOSPLIT, $0-16
	MOVQ addr+0(FP), AX
	MOVL (AX), AX
	MOVQ AX, ret+8(FP)
	RET
