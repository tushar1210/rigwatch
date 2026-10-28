package collect

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tushar1210/rigwatch/internal/metrics"
)

// ReadDRM walks a sysfs root for DRM adapters (AMD, Intel, and NVIDIA when
// NVML did not already report them).
func ReadDRM(root string, skipNVIDIA bool, skipPCI map[string]bool) []metrics.GPU {
	drm := filepath.Join(root, "class", "drm")
	entries, err := os.ReadDir(drm)
	if err != nil {
		return nil
	}
	var out []metrics.GPU
	for _, ent := range entries {
		name := ent.Name()
		if !cardName(name) {
			continue
		}
		dev := filepath.Join(drm, name, "device")
		vendor := strings.ToLower(strings.TrimSpace(readText(filepath.Join(dev, "vendor"))))
		if vendor == "" {
			continue
		}
		vendorName, ok := drmVendor(vendor)
		if !ok {
			continue
		}
		if skipNVIDIA && vendorName == "nvidia" {
			continue
		}
		pci := pciFromUevent(readText(filepath.Join(dev, "uevent")))
		if pci != "" && skipPCI[metrics.NormPCI(pci)] {
			continue
		}
		g := metrics.GPU{
			Name:       drmProduct(dev, vendorName),
			Vendor:     vendorName,
			Source:     "sysfs",
			PCI:        pci,
			Util:       metrics.Missing("no utilisation sensor"),
			Temp:       metrics.Missing("no hwmon temperature"),
			FanRPM:     metrics.Missing("no fan sensor"),
			FanPercent: metrics.Missing("no fan sensor"),
			Power:      metrics.Missing("no power sensor"),
			ClockCore:  metrics.Missing("no clock sensor"),
			ClockMem:   metrics.Missing("no clock sensor"),
			VRAMHint:   "no VRAM counters",
		}
		if busy, ok := readSysFloat(filepath.Join(dev, "gpu_busy_percent")); ok {
			g.Util = metrics.OK(busy)
		}
		if used, ok := readSysUint(filepath.Join(dev, "mem_info_vram_used")); ok {
			g.VRAMUsed = used
			g.VRAMOK = true
			g.VRAMHint = ""
		}
		if total, ok := readSysUint(filepath.Join(dev, "mem_info_vram_total")); ok {
			g.VRAMTotal = total
			g.VRAMOK = true
			g.VRAMHint = ""
		}
		if mhz, ok := readSysFloat(filepath.Join(dev, "gt_cur_freq_mhz")); ok && mhz > 0 {
			g.ClockCore = metrics.OK(mhz)
		}
		fillDRMHwmon(&g, filepath.Join(dev, "hwmon"))
		out = append(out, g)
	}
	return out
}

func fillDRMHwmon(g *metrics.GPU, hwmon string) {
	entries, err := os.ReadDir(hwmon)
	if err != nil {
		return
	}
	for _, ent := range entries {
		base := filepath.Join(hwmon, ent.Name())
		if temp, ok := readSysFloat(filepath.Join(base, "temp1_input")); ok {
			g.Temp = metrics.OK(temp / 1000)
		}
		if rpm, ok := readSysFloat(filepath.Join(base, "fan1_input")); ok {
			g.FanRPM = metrics.OK(rpm)
		}
		if pwm, ok := readSysFloat(filepath.Join(base, "pwm1")); ok {
			g.FanPercent = metrics.OK(pwm / 255 * 100)
		}
		if uw, ok := readSysFloat(filepath.Join(base, "power1_average")); ok && uw > 0 {
			g.Power = metrics.OK(uw / 1e6)
		} else if uw, ok := readSysFloat(filepath.Join(base, "power1_input")); ok && uw > 0 {
			g.Power = metrics.OK(uw / 1e6)
		}
		if hz, ok := readSysFloat(filepath.Join(base, "freq1_input")); ok && hz > 1000 && !g.ClockCore.Available {
			g.ClockCore = metrics.OK(hz / 1e6)
		}
		if hz, ok := readSysFloat(filepath.Join(base, "freq2_input")); ok && hz > 1000 {
			g.ClockMem = metrics.OK(hz / 1e6)
		}
	}
}

func cardName(name string) bool {
	if !strings.HasPrefix(name, "card") {
		return false
	}
	return digits(name[4:])
}

func drmVendor(id string) (string, bool) {
	switch id {
	case "0x1002":
		return "amd", true
	case "0x8086":
		return "intel", true
	case "0x10de":
		return "nvidia", true
	default:
		return "", false
	}
}

func drmProduct(dev, vendor string) string {
	if name := strings.TrimSpace(readText(filepath.Join(dev, "product_name"))); name != "" {
		return name
	}
	device := strings.TrimSpace(readText(filepath.Join(dev, "device")))
	label := map[string]string{"amd": "AMD GPU", "intel": "Intel GPU", "nvidia": "NVIDIA GPU"}[vendor]
	if label == "" {
		label = "GPU"
	}
	if device != "" {
		return label + " " + device
	}
	return label
}

func pciFromUevent(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "PCI_SLOT_NAME=") {
			return strings.TrimPrefix(line, "PCI_SLOT_NAME=")
		}
	}
	return ""
}

func readText(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

func readSysFloat(path string) (float64, bool) {
	s := strings.TrimSpace(readText(path))
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func readSysUint(path string) (uint64, bool) {
	s := strings.TrimSpace(readText(path))
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}
