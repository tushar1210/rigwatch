//go:build !linux

package collect

func currentFreqMHz() (float64, bool) {
	return 0, false
}
