//go:build linux

package collect

import (
	"context"

	"github.com/tushar1210/rigwatch/internal/metrics"
)

func readPlatformGPUs(_ context.Context, skipNVIDIA bool, skipPCI map[string]bool) []metrics.GPU {
	return ReadDRM("/sys", skipNVIDIA, skipPCI)
}
