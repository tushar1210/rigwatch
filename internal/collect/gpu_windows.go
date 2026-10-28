//go:build windows

package collect

import (
	"context"
	"strings"

	"github.com/yusufpapurcu/wmi"

	"github.com/tushar1210/rigwatch/internal/metrics"
)

type win32Video struct {
	Name        string
	PNPDeviceID string
}

func readPlatformGPUs(_ context.Context, skipNVIDIA bool, _ map[string]bool) []metrics.GPU {
	var rows []win32Video
	if err := wmi.Query("SELECT Name, PNPDeviceID FROM Win32_VideoController", &rows); err != nil {
		return nil
	}
	var out []metrics.GPU
	for _, row := range rows {
		name := strings.TrimSpace(row.Name)
		if name == "" {
			continue
		}
		vendor := vendorFromAdapter(name, row.PNPDeviceID)
		if skipNVIDIA && vendor == "nvidia" {
			continue
		}
		out = append(out, metrics.GPU{
			Name:       name,
			Vendor:     vendor,
			Source:     "wmi",
			Util:       metrics.Missing("start LibreHardwareMonitor"),
			Temp:       metrics.Missing("start LibreHardwareMonitor"),
			FanRPM:     metrics.Missing("start LibreHardwareMonitor"),
			FanPercent: metrics.Missing("start LibreHardwareMonitor"),
			Power:      metrics.Missing("start LibreHardwareMonitor"),
			ClockCore:  metrics.Missing("start LibreHardwareMonitor"),
			ClockMem:   metrics.Missing("start LibreHardwareMonitor"),
			VRAMHint:   "start LibreHardwareMonitor",
		})
	}
	return out
}

func vendorFromAdapter(name, pnp string) string {
	l := strings.ToLower(name + " " + pnp)
	switch {
	case strings.Contains(l, "ven_10de") || strings.Contains(l, "nvidia") || strings.Contains(l, "geforce"):
		return "nvidia"
	case strings.Contains(l, "ven_1002") || strings.Contains(l, "radeon") || strings.Contains(l, "amd"):
		return "amd"
	case strings.Contains(l, "ven_8086") || strings.Contains(l, "intel") || strings.Contains(l, "arc"):
		return "intel"
	default:
		return "unknown"
	}
}
