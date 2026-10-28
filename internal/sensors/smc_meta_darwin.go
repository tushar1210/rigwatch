//go:build darwin

package sensors

const (
	smcKernelIndex  = 2
	smcCmdReadBytes = 5
	smcCmdReadIndex = 8
	smcCmdReadInfo  = 9
)

func smcKind(key string) string {
	if len(key) < 2 {
		return "other"
	}
	switch key[:2] {
	case "TC", "Tp", "Te", "Ta":
		if key == "TC0D" || key == "TC0E" || len(key) == 4 && key[3] == 'P' {
			return "package"
		}
		return "core"
	case "TG", "Tg":
		return "gpu"
	case "TH", "Th":
		return "disk"
	default:
		return "other"
	}
}

func smcTempName(key string) string {
	if name, ok := smcNames[key]; ok {
		return name
	}
	return key
}

var smcNames = map[string]string{
	"TC0P": "CPU proximity",
	"TC0D": "CPU diode",
	"TC0E": "CPU die",
	"TC0F": "CPU extra",
	"TG0P": "GPU proximity",
	"TG0D": "GPU die",
	"TPCD": "platform controller",
	"Tp0P": "CPU performance",
	"Tp01": "CPU p-core 1",
	"Tp05": "CPU p-core 2",
	"Te05": "CPU e-core",
	"Tg05": "GPU",
	"Tg0D": "GPU die",
	"Th0H": "SSD",
	"TB0T": "battery",
}

func fallbackSMCKeys() []string {
	return []string{
		"TC0P", "TC0E", "TC0F", "TC0D", "TC1C", "TC2C", "TC3C", "TC4C",
		"TG0P", "TG0D", "TPCD", "Th0H", "TB0T",
		"Tp01", "Tp05", "Tp09", "Tp0D", "Tp0H", "Tp0L", "Tp0P", "Tp0X",
		"Te05", "Te0P", "Te0S",
		"Tg05", "Tg0D", "Tg0P", "Tg0f", "Tg0j",
		"F0Ac", "F1Ac", "F2Ac", "F3Ac",
	}
}
