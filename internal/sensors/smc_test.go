package sensors

import (
	"math"
	"testing"
	"unsafe"
)

func TestSMCStructLayout(t *testing.T) {
	var s smcKeyData
	if unsafe.Sizeof(s) != 80 {
		t.Fatalf("sizeof = %d", unsafe.Sizeof(s))
	}
	if unsafe.Offsetof(s.Key) != 0 || unsafe.Offsetof(s.Vers) != 4 {
		t.Fatalf("head offsets key=%d vers=%d", unsafe.Offsetof(s.Key), unsafe.Offsetof(s.Vers))
	}
	if unsafe.Offsetof(s.PLimit) != 12 || unsafe.Offsetof(s.KeyInfo) != 28 {
		t.Fatalf("mid offsets plimit=%d info=%d", unsafe.Offsetof(s.PLimit), unsafe.Offsetof(s.KeyInfo))
	}
	if unsafe.Offsetof(s.Result) != 40 || unsafe.Offsetof(s.Data32) != 44 || unsafe.Offsetof(s.Bytes) != 48 {
		t.Fatalf("tail result=%d data32=%d bytes=%d", unsafe.Offsetof(s.Result), unsafe.Offsetof(s.Data32), unsafe.Offsetof(s.Bytes))
	}
}

func TestDecodeSMC(t *testing.T) {
	// 62.5°C in sp78 is 62.5 * 256 = 16000 = 0x3E80
	v, typ, ok := decodeSMC(fourCC("sp78"), []byte{0x3E, 0x80})
	if !ok || typ != "sp78" || math.Abs(v-62.5) > 0.01 {
		t.Fatalf("sp78 = %v %s %v", v, typ, ok)
	}
	rpm, _, ok := decodeSMC(fourCC("fpe2"), []byte{0x12, 0xC0})
	if !ok || math.Abs(rpm-1200) > 0.01 {
		t.Fatalf("fpe2 = %v", rpm)
	}
	if !isSMCFan("F0Ac", "fpe2") || isSMCFan("FNum", "ui8 ") {
		t.Fatal("fan classification")
	}
	if !isSMCTemp("TC0P", "sp78") || isSMCTemp("F0Ac", "fpe2") {
		t.Fatal("temp classification")
	}
}

func TestFourCC(t *testing.T) {
	if fourCCString(fourCC("TC0P")) != "TC0P" {
		t.Fatalf("round trip %q", fourCCString(fourCC("TC0P")))
	}
}
