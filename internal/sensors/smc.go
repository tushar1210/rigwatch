package sensors

import (
	"math"
	"strings"
)

// SMC key data matches the AppleSMC user-client struct (80 bytes on
// darwin/amd64 and darwin/arm64). Offsets are checked in tests.
type smcVers struct {
	Major    byte
	Minor    byte
	Build    byte
	Reserved byte
	Release  uint16
}

type smcPLimit struct {
	Version uint16
	Length  uint16
	CPU     uint32
	GPU     uint32
	Mem     uint32
}

type smcKeyInfo struct {
	DataSize uint32
	DataType uint32
	Attr     byte
}

type smcKeyData struct {
	Key     uint32
	Vers    smcVers
	PLimit  smcPLimit
	KeyInfo smcKeyInfo
	Result  byte
	Status  byte
	Data8   byte
	Data32  uint32
	Bytes   [32]byte
}

func fourCC(s string) uint32 {
	b := [4]byte{}
	copy(b[:], s)
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}

func fourCCString(v uint32) string {
	b := []byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}
	return strings.TrimRight(string(b), "\x00 ")
}

func decodeSMC(dataType uint32, raw []byte) (float64, string, bool) {
	typ := fourCCString(dataType)
	switch typ {
	case "sp78":
		if len(raw) < 2 {
			return 0, typ, false
		}
		v := int16(uint16(raw[0])<<8 | uint16(raw[1]))
		return float64(v) / 256, typ, true
	case "fpe2":
		if len(raw) < 2 {
			return 0, typ, false
		}
		v := uint16(raw[0])<<8 | uint16(raw[1])
		return float64(v) / 4, typ, true
	case "flt ":
		if len(raw) < 4 {
			return 0, typ, false
		}
		bits := uint32(raw[0]) | uint32(raw[1])<<8 | uint32(raw[2])<<16 | uint32(raw[3])<<24
		return float64(math.Float32frombits(bits)), typ, true
	case "ui8 ":
		if len(raw) < 1 {
			return 0, typ, false
		}
		return float64(raw[0]), typ, true
	case "ui16":
		if len(raw) < 2 {
			return 0, typ, false
		}
		return float64(uint16(raw[0])<<8 | uint16(raw[1])), typ, true
	case "ui32":
		if len(raw) < 4 {
			return 0, typ, false
		}
		v := uint32(raw[0])<<24 | uint32(raw[1])<<16 | uint32(raw[2])<<8 | uint32(raw[3])
		return float64(v), typ, true
	default:
		return 0, typ, false
	}
}

func isSMCFan(key, typ string) bool {
	return len(key) == 4 && key[0] == 'F' && key[2] == 'A' && key[3] == 'c' && typ == "fpe2"
}

func isSMCTemp(key, typ string) bool {
	if len(key) != 4 || key[0] != 'T' {
		return false
	}
	return typ == "sp78" || typ == "flt "
}
