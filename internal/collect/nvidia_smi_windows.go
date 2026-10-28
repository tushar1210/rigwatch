//go:build windows

package collect

import "os"

func windowsNvidiaSMI() string {
	candidates := []string{
		`C:\Windows\System32\nvidia-smi.exe`,
		`C:\Program Files\NVIDIA Corporation\NVSMI\nvidia-smi.exe`,
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}
