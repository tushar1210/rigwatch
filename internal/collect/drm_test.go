package collect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tushar1210/rigwatch/internal/metrics"
)

func TestReadDRM(t *testing.T) {
	root := t.TempDir()
	dev := filepath.Join(root, "class", "drm", "card0", "device")
	hw := filepath.Join(dev, "hwmon", "hwmon2")
	if err := os.MkdirAll(hw, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "class", "drm", "card0-DP-1", "device"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(dev, "vendor"), "0x1002\n")
	write(filepath.Join(dev, "device"), "0x73bf\n")
	write(filepath.Join(dev, "gpu_busy_percent"), "33\n")
	write(filepath.Join(dev, "mem_info_vram_used"), "1073741824\n")
	write(filepath.Join(dev, "mem_info_vram_total"), "8589934592\n")
	write(filepath.Join(dev, "uevent"), "PCI_SLOT_NAME=0000:03:00.0\n")
	write(filepath.Join(hw, "temp1_input"), "67000\n")
	write(filepath.Join(hw, "fan1_input"), "1450\n")
	write(filepath.Join(hw, "pwm1"), "128\n")
	write(filepath.Join(hw, "power1_average"), "150000000\n")
	write(filepath.Join(hw, "freq1_input"), "1800000000\n")
	write(filepath.Join(hw, "freq2_input"), "10000000000\n")

	gpus := ReadDRM(root, false, nil)
	if len(gpus) != 1 {
		t.Fatalf("gpus = %+v", gpus)
	}
	g := gpus[0]
	if g.Vendor != "amd" || g.Util.Value != 33 || g.Temp.Value != 67 || g.FanRPM.Value != 1450 {
		t.Fatalf("gpu = %+v", g)
	}
	if g.Power.Value < 149 || g.Power.Value > 151 || g.ClockCore.Value < 1799 || g.ClockMem.Value < 9999 {
		t.Fatalf("power/clock = %+v", g)
	}
	if g.VRAMTotal != 8589934592 || g.PCI != "0000:03:00.0" {
		t.Fatalf("vram/pci = %+v", g)
	}
	if got := ReadDRM(root, false, map[string]bool{metrics.NormPCI(g.PCI): true}); len(got) != 0 {
		t.Fatalf("expected pci skip, got %+v", got)
	}
}
