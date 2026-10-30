package tui

import (
	"math"
	"strings"
)

const sparkBlocks = "▁▂▃▄▅▆▇█"

// Sparkline draws a one-line history. Missing samples (NaN) are left blank.
func Sparkline(values []float64, width int) string {
	if width <= 0 {
		return ""
	}
	if len(values) == 0 {
		return strings.Repeat("▁", width)
	}
	vals := values
	if len(vals) > width {
		vals = vals[len(vals)-width:]
	}
	min, max := math.NaN(), math.NaN()
	for _, v := range vals {
		if !finite(v) {
			continue
		}
		if math.IsNaN(min) || v < min {
			min = v
		}
		if math.IsNaN(max) || v > max {
			max = v
		}
	}
	span := max - min
	var b strings.Builder
	if pad := width - len(vals); pad > 0 {
		b.WriteString(strings.Repeat(" ", pad))
	}
	blocks := []rune(sparkBlocks)
	for _, v := range vals {
		if !finite(v) || math.IsNaN(min) {
			b.WriteRune(' ')
			continue
		}
		idx := sparkIndex(v, min, max, span, len(blocks))
		b.WriteRune(blocks[idx])
	}
	return b.String()
}

func finite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

func hasFinite(vals []float64) bool {
	for _, v := range vals {
		if finite(v) {
			return true
		}
	}
	return false
}

func sparkIndex(v, min, max, span float64, n int) int {
	if span <= 0 {
		if max == 0 {
			return 0
		}
		return n / 2
	}
	idx := int(math.Round((v - min) / span * float64(n-1)))
	if idx < 0 {
		return 0
	}
	if idx >= n {
		return n - 1
	}
	return idx
}

func appendCap(dst []float64, v float64, n int) []float64 {
	dst = append(dst, v)
	if len(dst) > n {
		dst = dst[len(dst)-n:]
	}
	return dst
}

func seedWave(end float64, n int) []float64 {
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		// Ease toward the current reading so the line is not a flat bar.
		phase := float64(i) / float64(n) * 6.2
		out[i] = end*0.65 + end*0.35*math.Sin(phase)
		if out[i] < 0 {
			out[i] = 0
		}
	}
	out[n-1] = end
	return out
}
