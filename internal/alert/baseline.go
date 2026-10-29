package alert

import "time"

type point struct {
	t time.Time
	v float64
}

// series is a time-bounded window of samples.
// Mean reports points already stored. Add appends the current sample afterwards
// so a spike is compared with the earlier mean.
type series struct {
	pts []point
}

func (s *series) Mean(now time.Time, window time.Duration) (float64, int) {
	if len(s.pts) == 0 {
		return 0, 0
	}
	cutoff := now.Add(-window)
	var sum float64
	n := 0
	for _, p := range s.pts {
		if window > 0 && p.t.Before(cutoff) {
			continue
		}
		sum += p.v
		n++
	}
	if n == 0 {
		return 0, 0
	}
	return sum / float64(n), n
}

func (s *series) Add(now time.Time, v float64, window time.Duration) {
	s.pts = append(s.pts, point{t: now, v: v})
	if window <= 0 {
		return
	}
	cutoff := now.Add(-window)
	i := 0
	for i < len(s.pts) && s.pts[i].t.Before(cutoff) {
		i++
	}
	if i > 0 {
		s.pts = append([]point(nil), s.pts[i:]...)
	}
}
