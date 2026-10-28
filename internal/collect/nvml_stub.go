//go:build !linux && !windows

package collect

import (
	"errors"

	"github.com/tushar1210/rigwatch/internal/metrics"
)

func readNVML() ([]metrics.GPU, error) {
	return nil, errors.New("nvml is not available on this OS")
}
