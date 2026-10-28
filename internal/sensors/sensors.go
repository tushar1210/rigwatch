// Package sensors reads temperatures and fans from the host.
// Missing sources come back as an empty reading plus a short hint.
package sensors

import (
	"context"

	"github.com/tushar1210/rigwatch/internal/metrics"
)

// Reading is what a platform sensor source could provide.
type Reading struct {
	Temps []metrics.Temp
	Fans  []metrics.Fan
	GPUs  []metrics.GPU
	Hint  string
}

// Read samples the platform. lhmURL is consulted on Windows.
func Read(ctx context.Context, lhmURL string) Reading {
	if ctx == nil {
		ctx = context.Background()
	}
	return readPlatform(ctx, lhmURL)
}
