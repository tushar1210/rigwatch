//go:build linux

package collect

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadCPUFreq(t *testing.T) {
	root := t.TempDir()
	for _, cpu := range []string{"cpu0", "cpu1"} {
		dir := filepath.Join(root, cpu, "cpufreq")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		body := "3600000\n"
		if cpu == "cpu1" {
			body = "4200000\n"
		}
		if err := os.WriteFile(filepath.Join(dir, "scaling_cur_freq"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mhz, ok := readCPUFreq(root)
	if !ok || mhz < 3899 || mhz > 3901 {
		t.Fatalf("mhz = %v ok=%v", mhz, ok)
	}
}
