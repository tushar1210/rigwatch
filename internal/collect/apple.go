package collect

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/tushar1210/rigwatch/internal/metrics"
)

var (
	reAccelNode = regexp.MustCompile(`\+-o\s+(\S+)\s+<class IOAccelerator`)
	reDevUtil   = regexp.MustCompile(`"Device Utilization %"\s*=\s*(\d+)`)
	reWired     = regexp.MustCompile(`"In use system memory"\s*=\s*(\d+)`)
)

// ParseIORegAccelerator reads `ioreg -r -c IOAccelerator -l` text.
// Apple Silicon exposes utilisation and wired memory here without sudo.
func ParseIORegAccelerator(text string) []metrics.GPU {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	name := "Apple GPU"
	if m := reAccelNode.FindStringSubmatch(text); len(m) == 2 && m[1] != "" {
		name = "Apple GPU"
	}
	g := metrics.GPU{
		Name:       name,
		Vendor:     "apple",
		Source:     "ioreg",
		Unified:    true,
		Util:       metrics.Missing("no IOAccelerator utilisation"),
		Temp:       metrics.Missing("SMC"),
		FanRPM:     metrics.Missing("SMC"),
		FanPercent: metrics.Missing("no fan sensor"),
		Power:      metrics.Missing("not exposed"),
		ClockCore:  metrics.Missing("not exposed"),
		ClockMem:   metrics.Missing("not exposed"),
		VRAMHint:   "unified memory",
	}
	if m := reDevUtil.FindStringSubmatch(text); len(m) == 2 {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			g.Util = metrics.OK(v)
		}
	}
	if m := reWired.FindStringSubmatch(text); len(m) == 2 {
		if v, err := strconv.ParseUint(m[1], 10, 64); err == nil {
			g.VRAMUsed = v
			g.VRAMOK = true
		}
	}
	if !g.Util.Available && !g.VRAMOK && !reAccelNode.MatchString(text) {
		return nil
	}
	return []metrics.GPU{g}
}
