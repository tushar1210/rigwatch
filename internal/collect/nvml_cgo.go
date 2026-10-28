//go:build linux && cgo

package collect

import (
	"fmt"
	"sync"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	"github.com/tushar1210/rigwatch/internal/metrics"
)

// readNVML loads libnvidia-ml.so on demand through go-nvml.
// The binary still starts when the driver is absent; Init fails and the
// caller falls back to nvidia-smi.
func readNVML() ([]metrics.GPU, error) {
	if err := nvmlInit(); err != nil {
		return nil, err
	}
	count, ret := nvml.DeviceGetCount()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("%s", ret.Error())
	}
	gpus := make([]metrics.GPU, 0, count)
	for i := 0; i < count; i++ {
		dev, ret := nvml.DeviceGetHandleByIndex(i)
		if ret != nvml.SUCCESS {
			continue
		}
		gpus = append(gpus, gpuFromNVML(dev, i))
	}
	if len(gpus) == 0 {
		return nil, fmt.Errorf("nvml reported no devices")
	}
	return gpus, nil
}

var (
	nvmlOnce sync.Once
	nvmlErr  error
)

func nvmlInit() error {
	nvmlOnce.Do(func() {
		ret := nvml.Init()
		if ret != nvml.SUCCESS {
			nvmlErr = fmt.Errorf("%s", ret.Error())
		}
	})
	return nvmlErr
}

func gpuFromNVML(dev nvml.Device, index int) metrics.GPU {
	g := metrics.GPU{
		Name:       fmt.Sprintf("NVIDIA GPU %d", index),
		Vendor:     "nvidia",
		Source:     "nvml",
		Util:       metrics.Missing("unsupported"),
		Temp:       metrics.Missing("unsupported"),
		FanRPM:     metrics.Missing("unsupported"),
		FanPercent: metrics.Missing("unsupported"),
		Power:      metrics.Missing("unsupported"),
		ClockCore:  metrics.Missing("unsupported"),
		ClockMem:   metrics.Missing("unsupported"),
		VRAMHint:   "unsupported",
	}
	if name, ret := dev.GetName(); ret == nvml.SUCCESS && name != "" {
		g.Name = name
	}
	if pci, ret := dev.GetPciInfo(); ret == nvml.SUCCESS {
		g.PCI = cString(pci.BusId[:])
	}
	if util, ret := dev.GetUtilizationRates(); ret == nvml.SUCCESS {
		g.Util = metrics.OK(float64(util.Gpu))
	}
	if temp, ret := dev.GetTemperature(nvml.TEMPERATURE_GPU); ret == nvml.SUCCESS {
		g.Temp = metrics.OK(float64(temp))
	}
	if fan, ret := dev.GetFanSpeed(); ret == nvml.SUCCESS {
		g.FanPercent = metrics.OK(float64(fan))
	}
	if rpm, ret := dev.GetFanSpeedRPM(); ret == nvml.SUCCESS && rpm.Speed > 0 {
		g.FanRPM = metrics.OK(float64(rpm.Speed))
	}
	if mem, ret := dev.GetMemoryInfo(); ret == nvml.SUCCESS && mem.Total > 0 {
		g.VRAMOK = true
		g.VRAMHint = ""
		g.VRAMUsed = mem.Used
		g.VRAMTotal = mem.Total
	}
	if mw, ret := dev.GetPowerUsage(); ret == nvml.SUCCESS {
		g.Power = metrics.OK(float64(mw) / 1000)
	}
	if clk, ret := dev.GetClockInfo(nvml.CLOCK_GRAPHICS); ret == nvml.SUCCESS {
		g.ClockCore = metrics.OK(float64(clk))
	}
	if clk, ret := dev.GetClockInfo(nvml.CLOCK_MEM); ret == nvml.SUCCESS {
		g.ClockMem = metrics.OK(float64(clk))
	}
	return g
}

func cString(b []int8) string {
	buf := make([]byte, 0, len(b))
	for _, c := range b {
		if c == 0 {
			break
		}
		buf = append(buf, byte(c))
	}
	return string(buf)
}
