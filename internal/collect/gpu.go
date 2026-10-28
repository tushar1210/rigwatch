package collect

import (
	"context"
	"strings"

	"github.com/tushar1210/rigwatch/internal/metrics"
)

func collectGPUs(ctx context.Context) []metrics.GPU {
	var nv []metrics.GPU
	if list, err := readNVML(); err == nil && len(list) > 0 {
		nv = list
	} else if list, err := readNvidiaSMI(ctx); err == nil && len(list) > 0 {
		nv = list
	}
	skip := map[string]bool{}
	hasNVIDIA := false
	for _, g := range nv {
		if g.Vendor == "nvidia" {
			hasNVIDIA = true
			if g.PCI != "" {
				skip[metrics.NormPCI(g.PCI)] = true
			}
		}
	}
	return append(nv, readPlatformGPUs(ctx, hasNVIDIA, skip)...)
}

func mergeGPUs(primary, extra []metrics.GPU) []metrics.GPU {
	if len(primary) == 0 {
		return extra
	}
	used := map[int]bool{}
	for i := range primary {
		j := matchGPU(primary[i], extra, used)
		if j < 0 {
			continue
		}
		used[j] = true
		fillGPU(&primary[i], extra[j])
	}
	for j := range extra {
		if !used[j] {
			primary = append(primary, extra[j])
		}
	}
	return primary
}

func matchGPU(g metrics.GPU, extra []metrics.GPU, used map[int]bool) int {
	if g.PCI != "" {
		want := metrics.NormPCI(g.PCI)
		for i := range extra {
			if used[i] || extra[i].PCI == "" {
				continue
			}
			if metrics.NormPCI(extra[i].PCI) == want {
				return i
			}
		}
	}
	for i := range extra {
		if used[i] {
			continue
		}
		if g.Vendor != "" && g.Vendor == extra[i].Vendor && strings.EqualFold(g.Name, extra[i].Name) {
			return i
		}
	}
	if g.Vendor != "" {
		only := -1
		for i := range extra {
			if used[i] || extra[i].Vendor != g.Vendor {
				continue
			}
			if only >= 0 {
				return -1
			}
			only = i
		}
		return only
	}
	return -1
}

func fillGPU(dst *metrics.GPU, src metrics.GPU) {
	if dst.Name == "" {
		dst.Name = src.Name
	}
	if dst.Vendor == "" || dst.Vendor == "unknown" {
		dst.Vendor = src.Vendor
	}
	if dst.PCI == "" {
		dst.PCI = src.PCI
	}
	if dst.Source == "" {
		dst.Source = src.Source
	}
	fillGauge(&dst.Util, src.Util)
	fillGauge(&dst.Temp, src.Temp)
	fillGauge(&dst.FanRPM, src.FanRPM)
	fillGauge(&dst.FanPercent, src.FanPercent)
	fillGauge(&dst.Power, src.Power)
	fillGauge(&dst.ClockCore, src.ClockCore)
	fillGauge(&dst.ClockMem, src.ClockMem)
	if !dst.VRAMOK && src.VRAMOK {
		dst.VRAMUsed = src.VRAMUsed
		dst.VRAMTotal = src.VRAMTotal
		dst.VRAMOK = true
		dst.VRAMHint = src.VRAMHint
		dst.Unified = src.Unified
	}
	if dst.VRAMHint == "" {
		dst.VRAMHint = src.VRAMHint
	}
}

func fillGauge(dst *metrics.Gauge, src metrics.Gauge) {
	if !dst.Available && src.Available {
		*dst = src
	}
}

func foldSensors(snap *metrics.Snapshot) {
	for i := range snap.GPUs {
		g := &snap.GPUs[i]
		if !g.Temp.Available {
			if t, ok := tempForGPU(snap.Temps, g.Name); ok {
				g.Temp = metrics.OK(t.Celsius)
			}
		}
		if !g.FanRPM.Available {
			for _, f := range snap.Fans {
				if f.HasRPM && strings.Contains(strings.ToLower(f.Name), strings.ToLower(firstWord(g.Name))) && g.Vendor != "" {
					g.FanRPM = metrics.OK(f.RPM)
					break
				}
			}
		}
	}
	for _, g := range snap.GPUs {
		if g.Temp.Available && !hasTempName(snap.Temps, g.Name) {
			snap.Temps = append(snap.Temps, metrics.Temp{
				Name:    g.Name,
				Celsius: g.Temp.Value,
				Source:  g.Source,
				Kind:    "gpu",
			})
		}
		if g.FanRPM.Available && !hasFanName(snap.Fans, g.Name) {
			snap.Fans = append(snap.Fans, metrics.Fan{
				Name:   g.Name,
				RPM:    g.FanRPM.Value,
				HasRPM: true,
				Source: g.Source,
			})
		}
	}
}

func tempForGPU(temps []metrics.Temp, name string) (metrics.Temp, bool) {
	for _, t := range temps {
		if t.Kind == "gpu" && (strings.Contains(strings.ToLower(t.Name), strings.ToLower(name)) || strings.Contains(strings.ToLower(name), strings.ToLower(t.Name))) {
			return t, true
		}
	}
	for _, t := range temps {
		if t.Kind == "gpu" {
			return t, true
		}
	}
	return metrics.Temp{}, false
}

func hasTempName(temps []metrics.Temp, name string) bool {
	for _, t := range temps {
		if strings.EqualFold(t.Name, name) {
			return true
		}
	}
	return false
}

func hasFanName(fans []metrics.Fan, name string) bool {
	for _, f := range fans {
		if strings.EqualFold(f.Name, name) {
			return true
		}
	}
	return false
}

func firstWord(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, ' '); i > 0 {
		return s[:i]
	}
	return s
}
