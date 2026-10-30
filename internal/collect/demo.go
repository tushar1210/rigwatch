package collect

import (
	"context"
	"math"
	"time"

	"github.com/tushar1210/rigwatch/internal/metrics"
)

// Demo returns a realistic synthetic machine so the UI can be exercised
// without hardware sensors.
type Demo struct {
	n int
}

// NewDemo returns a demo collector.
func NewDemo() *Demo { return &Demo{} }

// Sample returns the next synthetic snapshot.
func (d *Demo) Sample(ctx context.Context) (metrics.Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return metrics.Snapshot{}, err
	}
	d.n++
	return DemoSnapshot(d.n), nil
}

// DemoSnapshot builds a synthetic sample. n advances the sparklines.
func DemoSnapshot(n int) metrics.Snapshot {
	wave := func(base, amp float64, phase float64) float64 {
		return base + amp*math.Sin(float64(n)/6+phase)
	}
	cores := make([]float64, 8)
	for i := range cores {
		cores[i] = clamp(wave(18, 14, float64(i)), 1, 100)
	}
	cpu := clamp(wave(22, 12, 0), 1, 100)
	gpuUtil := clamp(wave(41, 18, 1.2), 0, 100)
	temp := clamp(wave(62, 6, 0.4), 35, 95)
	return metrics.Snapshot{
		Time: time.Now(),
		Host: metrics.Host{
			Hostname:      "forge",
			OS:            "linux",
			Platform:      "ubuntu 24.04",
			Arch:          "amd64",
			Kernel:        "6.8.0",
			UptimeSeconds: 4*86400 + 12*3600,
		},
		CPU: metrics.CPU{
			UsagePercent: cpu,
			PerCore:      cores,
			FrequencyMHz: metrics.OK(wave(4200, 250, 0.2)),
			Load1:        metrics.OK(wave(0.82, 0.3, 0)),
			Load5:        metrics.OK(0.74),
			Load15:       metrics.OK(0.61),
		},
		Memory: metrics.Memory{
			Total:           32 * 1024 * 1024 * 1024,
			Used:            u64(clamp(wave(12.4, 1.2, 0.3), 8, 24) * 1024 * 1024 * 1024),
			Available:       18 * 1024 * 1024 * 1024,
			Cached:          6 * 1024 * 1024 * 1024,
			CachedAvailable: true,
			UsedPercent:     clamp(wave(39, 4, 0.3), 10, 90),
			SwapTotal:       8 * 1024 * 1024 * 1024,
			SwapUsed:        400 * 1024 * 1024,
			SwapPercent:     4.8,
			SwapLabel:       "swap",
		},
		GPUs: []metrics.GPU{{
			Name:       "NVIDIA GeForce RTX 4090",
			Vendor:     "nvidia",
			Source:     "nvml",
			PCI:        "0000:01:00.0",
			Util:       metrics.OK(gpuUtil),
			Temp:       metrics.OK(clamp(wave(68, 5, 0.8), 40, 90)),
			FanRPM:     metrics.OK(wave(1450, 120, 0.5)),
			FanPercent: metrics.OK(clamp(wave(48, 8, 0.5), 20, 80)),
			VRAMUsed:   u64(8.2 * 1024 * 1024 * 1024),
			VRAMTotal:  24 * 1024 * 1024 * 1024,
			VRAMOK:     true,
			Power:      metrics.OK(wave(210, 30, 1)),
			ClockCore:  metrics.OK(2520),
			ClockMem:   metrics.OK(10501),
		}},
		Fans: []metrics.Fan{
			{Name: "cpu_fan", RPM: wave(1200, 80, 0.2), HasRPM: true, Source: "hwmon"},
			{Name: "gpu_fan", RPM: wave(1450, 120, 0.5), HasRPM: true, Percent: 48, HasPercent: true, Source: "hwmon"},
			{Name: "case_rear", RPM: 800, HasRPM: true, Source: "hwmon"},
		},
		Temps: []metrics.Temp{
			{Name: "Tctl", Celsius: temp, Source: "hwmon", Kind: "package"},
			{Name: "Tccd1", Celsius: temp - 4, Source: "hwmon", Kind: "core"},
			{Name: "nvme Composite", Celsius: 41, Source: "hwmon", Kind: "disk"},
			{Name: "acpitz", Celsius: 38, Source: "thermal", Kind: "other"},
		},
		Disks: []metrics.Disk{
			{Name: "nvme0n1 /", Mount: "/", Total: 1 << 40, Used: u64(0.42 * float64(uint64(1)<<40)), UsedPercent: 42, ReadBytesPerSec: wave(12, 8, 0) * 1024 * 1024, WriteBytesPerSec: 4 * 1024 * 1024, ThroughputOK: true},
			{Name: "nvme0n1 /home", Mount: "/home", Total: 2 << 40, Used: u64(0.71 * float64(uint64(2)<<40)), UsedPercent: 71, ReadBytesPerSec: 1 * 1024 * 1024, WriteBytesPerSec: wave(40, 20, 1) * 1024 * 1024, ThroughputOK: true},
		},
		Network: []metrics.NIC{
			{Name: "eth0", RxBytesPerSec: wave(18, 10, 0.6) * 1024 * 1024, TxBytesPerSec: 1.1 * 1024 * 1024},
			{Name: "wlan0", RxBytesPerSec: 0.2 * 1024 * 1024, TxBytesPerSec: 0.05 * 1024 * 1024},
		},
		Processes: []metrics.Process{
			{PID: 4210, Name: "rustc", CPUPercent: wave(24, 10, 0), MemPercent: 3.2, RSS: 1100 * 1024 * 1024},
			{PID: 1882, Name: "chrome", CPUPercent: 6.4, MemPercent: 8.1, RSS: u64(2.4 * 1024 * 1024 * 1024), GPUPercent: 4, GPUUtilOK: true, GPUMem: 900 << 20, GPUMemOK: true},
			{PID: 902, Name: "dockerd", CPUPercent: 2.1, MemPercent: 1.4, RSS: 420 * 1024 * 1024, GPUPercent: 1, GPUUtilOK: true, GPUMem: 64 << 20, GPUMemOK: true},
			{PID: 12, Name: "systemd", CPUPercent: 0.4, MemPercent: 0.2, RSS: 18 * 1024 * 1024},
			{PID: 7741, Name: "python", CPUPercent: wave(11, 6, 1), MemPercent: 2.2, RSS: 640 * 1024 * 1024, GPUPercent: 62, GPUUtilOK: true, GPUMem: 4 << 30, GPUMemOK: true},
		},
	}
}

func u64(v float64) uint64 { return uint64(v) }

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
