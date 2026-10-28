//go:build darwin

package collect

import (
	"context"
	"os/exec"
	"time"

	"github.com/tushar1210/rigwatch/internal/metrics"
)

func readPlatformGPUs(ctx context.Context, _ bool, _ map[string]bool) []metrics.GPU {
	ctx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ioreg", "-r", "-c", "IOAccelerator", "-l")
	out, err := cmd.Output()
	if err != nil {
		return []metrics.GPU{{
			Name:   "Apple GPU",
			Vendor: "apple",
			Source: "ioreg",
			Util:   metrics.Missing("ioreg failed"),
			Temp:   metrics.Missing("SMC"),
		}}
	}
	gpus := ParseIORegAccelerator(string(out))
	if len(gpus) == 0 {
		return []metrics.GPU{{
			Name:   "Apple GPU",
			Vendor: "apple",
			Source: "ioreg",
			Util:   metrics.Missing("no IOAccelerator utilisation"),
			Temp:   metrics.Missing("SMC"),
		}}
	}
	return gpus
}
