package alert

import (
	"testing"
	"time"
)

func TestSeriesDropsOldSamples(t *testing.T) {
	var s series
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.Add(t0, 10, time.Minute)
	s.Add(t0.Add(time.Second), 30, time.Minute)
	mean, n := s.Mean(t0.Add(2*time.Second), time.Minute)
	if n != 2 || mean != 20 {
		t.Fatalf("mean %v n %d", mean, n)
	}
	s.Add(t0.Add(2*time.Minute), 100, time.Minute)
	mean, n = s.Mean(t0.Add(2*time.Minute+time.Second), time.Minute)
	if n != 1 || mean != 100 {
		t.Fatalf("after window mean %v n %d pts %d", mean, n, len(s.pts))
	}
}
