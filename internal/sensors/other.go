//go:build !linux && !darwin && !windows

package sensors

import "context"

func readPlatform(context.Context, string) Reading {
	return Reading{Hint: "no sensor backend for this OS"}
}
