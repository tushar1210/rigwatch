//go:build (linux && !cgo) || windows

package collect

import (
	"fmt"

	"github.com/ebitengine/purego"

	"github.com/tushar1210/rigwatch/internal/metrics"
)

// dynLib is a pure-Go NVML binding used when cgo is off, including goreleaser
// binaries. go-nvml itself needs cgo and only builds on Linux.
type dynLib struct {
	init   func() int32
	count  func(*uint32) int32
	handle func(uint32, *uintptr) int32
	name   func(uintptr, *byte, uint32) int32
	util   func(uintptr, *dynUtil) int32
	temp   func(uintptr, uint32, *uint32) int32
	fan    func(uintptr, *uint32) int32
	mem    func(uintptr, *dynMem) int32
	power  func(uintptr, *uint32) int32
	clock  func(uintptr, uint32, *uint32) int32
	pci    func(uintptr, *dynPCI) int32
}

type dynUtil struct {
	GPU uint32
	Mem uint32
}

type dynMem struct {
	Total uint64
	Free  uint64
	Used  uint64
}

type dynPCI struct {
	Legacy [16]byte
	Domain uint32
	Bus    uint32
	Device uint32
	DevID  uint32
	SubID  uint32
	BusID  [32]byte
	Pad    [16]byte
}

func loadDyn(sym func(string) (uintptr, error)) (*dynLib, error) {
	lib := &dynLib{}
	var err error
	if lib.init, err = bindRet(sym, "nvmlInit_v2"); err != nil {
		if lib.init, err = bindRet(sym, "nvmlInit"); err != nil {
			return nil, err
		}
	}
	if lib.count, err = bindCount(sym); err != nil {
		return nil, err
	}
	if lib.handle, err = bindHandle(sym); err != nil {
		return nil, err
	}
	if lib.name, err = bindName(sym); err != nil {
		return nil, err
	}
	lib.util, _ = bindUtil(sym)
	lib.temp, _ = bindTemp(sym)
	lib.fan, _ = bindFan(sym)
	lib.mem, _ = bindMem(sym)
	lib.power, _ = bindPower(sym)
	lib.clock, _ = bindClock(sym)
	lib.pci, _ = bindPCI(sym)
	if ret := lib.init(); ret != 0 {
		return nil, fmt.Errorf("nvml init returned %d", ret)
	}
	return lib, nil
}

func (lib *dynLib) gpus() ([]metrics.GPU, error) {
	var count uint32
	if ret := lib.count(&count); ret != 0 {
		return nil, fmt.Errorf("nvml device count returned %d", ret)
	}
	out := make([]metrics.GPU, 0, count)
	for i := uint32(0); i < count; i++ {
		var dev uintptr
		if ret := lib.handle(i, &dev); ret != 0 || dev == 0 {
			continue
		}
		g := metrics.GPU{
			Name:       fmt.Sprintf("NVIDIA GPU %d", i),
			Vendor:     "nvidia",
			Source:     "nvml",
			Util:       metrics.Missing("unsupported"),
			Temp:       metrics.Missing("unsupported"),
			FanRPM:     metrics.Missing("fan rpm not in NVML"),
			FanPercent: metrics.Missing("unsupported"),
			Power:      metrics.Missing("unsupported"),
			ClockCore:  metrics.Missing("unsupported"),
			ClockMem:   metrics.Missing("unsupported"),
			VRAMHint:   "unsupported",
		}
		buf := make([]byte, 96)
		if ret := lib.name(dev, &buf[0], uint32(len(buf))); ret == 0 {
			if name := cBytes(buf); name != "" {
				g.Name = name
			}
		}
		if lib.util != nil {
			var u dynUtil
			if ret := lib.util(dev, &u); ret == 0 {
				g.Util = metrics.OK(float64(u.GPU))
			}
		}
		if lib.temp != nil {
			var t uint32
			if ret := lib.temp(dev, 0, &t); ret == 0 {
				g.Temp = metrics.OK(float64(t))
			}
		}
		if lib.fan != nil {
			var fan uint32
			if ret := lib.fan(dev, &fan); ret == 0 {
				g.FanPercent = metrics.OK(float64(fan))
			}
		}
		if lib.mem != nil {
			var m dynMem
			if ret := lib.mem(dev, &m); ret == 0 && m.Total > 0 {
				g.VRAMOK = true
				g.VRAMHint = ""
				g.VRAMUsed = m.Used
				g.VRAMTotal = m.Total
			}
		}
		if lib.power != nil {
			var mw uint32
			if ret := lib.power(dev, &mw); ret == 0 {
				g.Power = metrics.OK(float64(mw) / 1000)
			}
		}
		if lib.clock != nil {
			var core, mem uint32
			if ret := lib.clock(dev, 0, &core); ret == 0 {
				g.ClockCore = metrics.OK(float64(core))
			}
			if ret := lib.clock(dev, 2, &mem); ret == 0 {
				g.ClockMem = metrics.OK(float64(mem))
			}
		}
		if lib.pci != nil {
			var pci dynPCI
			if ret := lib.pci(dev, &pci); ret == 0 {
				g.PCI = cBytes(pci.BusID[:])
			}
		}
		out = append(out, g)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("nvml reported no devices")
	}
	return out, nil
}

func cBytes(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

func bindRet(sym func(string) (uintptr, error), name string) (func() int32, error) {
	addr, err := sym(name)
	if err != nil {
		return nil, err
	}
	var fn func() int32
	purego.RegisterFunc(&fn, addr)
	return fn, nil
}

func bindCount(sym func(string) (uintptr, error)) (func(*uint32) int32, error) {
	addr, err := sym("nvmlDeviceGetCount")
	if err != nil {
		return nil, err
	}
	var fn func(*uint32) int32
	purego.RegisterFunc(&fn, addr)
	return fn, nil
}

func bindHandle(sym func(string) (uintptr, error)) (func(uint32, *uintptr) int32, error) {
	addr, err := sym("nvmlDeviceGetHandleByIndex")
	if err != nil {
		return nil, err
	}
	var fn func(uint32, *uintptr) int32
	purego.RegisterFunc(&fn, addr)
	return fn, nil
}

func bindName(sym func(string) (uintptr, error)) (func(uintptr, *byte, uint32) int32, error) {
	addr, err := sym("nvmlDeviceGetName")
	if err != nil {
		return nil, err
	}
	var fn func(uintptr, *byte, uint32) int32
	purego.RegisterFunc(&fn, addr)
	return fn, nil
}

func bindUtil(sym func(string) (uintptr, error)) (func(uintptr, *dynUtil) int32, error) {
	addr, err := sym("nvmlDeviceGetUtilizationRates")
	if err != nil {
		return nil, err
	}
	var fn func(uintptr, *dynUtil) int32
	purego.RegisterFunc(&fn, addr)
	return fn, nil
}

func bindTemp(sym func(string) (uintptr, error)) (func(uintptr, uint32, *uint32) int32, error) {
	addr, err := sym("nvmlDeviceGetTemperature")
	if err != nil {
		return nil, err
	}
	var fn func(uintptr, uint32, *uint32) int32
	purego.RegisterFunc(&fn, addr)
	return fn, nil
}

func bindFan(sym func(string) (uintptr, error)) (func(uintptr, *uint32) int32, error) {
	addr, err := sym("nvmlDeviceGetFanSpeed")
	if err != nil {
		return nil, err
	}
	var fn func(uintptr, *uint32) int32
	purego.RegisterFunc(&fn, addr)
	return fn, nil
}

func bindMem(sym func(string) (uintptr, error)) (func(uintptr, *dynMem) int32, error) {
	addr, err := sym("nvmlDeviceGetMemoryInfo")
	if err != nil {
		return nil, err
	}
	var fn func(uintptr, *dynMem) int32
	purego.RegisterFunc(&fn, addr)
	return fn, nil
}

func bindPower(sym func(string) (uintptr, error)) (func(uintptr, *uint32) int32, error) {
	addr, err := sym("nvmlDeviceGetPowerUsage")
	if err != nil {
		return nil, err
	}
	var fn func(uintptr, *uint32) int32
	purego.RegisterFunc(&fn, addr)
	return fn, nil
}

func bindClock(sym func(string) (uintptr, error)) (func(uintptr, uint32, *uint32) int32, error) {
	addr, err := sym("nvmlDeviceGetClockInfo")
	if err != nil {
		return nil, err
	}
	var fn func(uintptr, uint32, *uint32) int32
	purego.RegisterFunc(&fn, addr)
	return fn, nil
}

func bindPCI(sym func(string) (uintptr, error)) (func(uintptr, *dynPCI) int32, error) {
	addr, err := sym("nvmlDeviceGetPciInfo_v3")
	if err != nil {
		addr, err = sym("nvmlDeviceGetPciInfo")
		if err != nil {
			return nil, err
		}
	}
	var fn func(uintptr, *dynPCI) int32
	purego.RegisterFunc(&fn, addr)
	return fn, nil
}
