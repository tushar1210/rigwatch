//go:build darwin && arm64

#include "textflag.h"

TEXT ·readU32(SB), NOSPLIT, $0-16
	MOVD addr+0(FP), R0
	MOVWU (R0), R0
	MOVD R0, ret+8(FP)
	RET
