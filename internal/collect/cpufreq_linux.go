//go:build linux

package collect

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func currentFreqMHz() (float64, bool) {
	return readCPUFreq("/sys/devices/system/cpu")
}

func readCPUFreq(cpuRoot string) (float64, bool) {
	entries, err := os.ReadDir(cpuRoot)
	if err != nil {
		return 0, false
	}
	var sum float64
	var n int
	for _, ent := range entries {
		name := ent.Name()
		if !strings.HasPrefix(name, "cpu") || !digits(name[3:]) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(cpuRoot, name, "cpufreq", "scaling_cur_freq"))
		if err != nil {
			continue
		}
		khz, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
		if err != nil || khz <= 0 {
			continue
		}
		sum += khz / 1000
		n++
	}
	if n == 0 {
		return 0, false
	}
	return sum / float64(n), true
}
