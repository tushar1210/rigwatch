//go:build !linux && !darwin && !windows

package collect

import (
	"context"

	"github.com/tushar1210/rigwatch/internal/metrics"
)

func readPlatformGPUs(context.Context, bool, map[string]bool) []metrics.GPU {
	return nil
}
