package tui

import (
	"math"
	"testing"
)

func TestSparkline(t *testing.T) {
	got := Sparkline([]float64{0, 1, 2, 3, 4}, 5)
	if len([]rune(got)) != 5 {
		t.Fatalf("spark = %q", got)
	}
	if got[0] == got[len(got)-1] && len(got) > 1 {
		// endpoints of a rising series should differ
		t.Fatalf("expected slope, got %q", got)
	}
	if Sparkline(nil, 3) != "▁▁▁" {
		t.Fatalf("empty = %q", Sparkline(nil, 3))
	}
	gap := []rune(Sparkline([]float64{1, math.NaN(), 3}, 3))
	if len(gap) != 3 || gap[1] != ' ' {
		t.Fatalf("gap = %q", string(gap))
	}
}

func TestSeedWave(t *testing.T) {
	w := seedWave(40, 8)
	if len(w) != 8 || w[7] != 40 {
		t.Fatalf("wave = %v", w)
	}
}
