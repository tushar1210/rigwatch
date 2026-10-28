package metrics

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// FormatBytes renders a byte count with a binary unit.
func FormatBytes(n uint64) string {
	return human(float64(n), []string{"B", "KB", "MB", "GB", "TB"})
}

// FormatRate renders bytes per second.
func FormatRate(bps float64) string {
	if bps < 0 || math.IsNaN(bps) || math.IsInf(bps, 0) {
		bps = 0
	}
	return human(bps, []string{"B/s", "KB/s", "MB/s", "GB/s"})
}

// FormatUptime renders a duration in a compact form.
func FormatUptime(seconds uint64) string {
	d := time.Duration(seconds) * time.Second
	days := d / (24 * time.Hour)
	d -= days * 24 * time.Hour
	hours := d / time.Hour
	d -= hours * time.Hour
	mins := d / time.Minute
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, mins)
	case mins > 0:
		return fmt.Sprintf("%dm", mins)
	default:
		return fmt.Sprintf("%ds", seconds)
	}
}

// FormatPercent renders a percentage with no decimals at or above 10.
func FormatPercent(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		v = 0
	}
	if v >= 10 {
		return fmt.Sprintf("%.0f%%", v)
	}
	return fmt.Sprintf("%.1f%%", v)
}

// FormatFixed renders a float with one decimal, or none when large.
func FormatFixed(v float64, unit string) string {
	if math.Abs(v) >= 100 {
		return fmt.Sprintf("%.0f%s", v, unit)
	}
	return fmt.Sprintf("%.1f%s", v, unit)
}

func human(n float64, units []string) string {
	if n < 0 || math.IsNaN(n) || math.IsInf(n, 0) {
		n = 0
	}
	i := 0
	for n >= 1024 && i < len(units)-1 {
		n /= 1024
		i++
	}
	return trimFloat(n) + " " + units[i]
}

func trimFloat(n float64) string {
	if math.Abs(n-math.Round(n)) < 0.05 || n >= 100 {
		return fmt.Sprintf("%.0f", n)
	}
	return fmt.Sprintf("%.1f", n)
}

// NormPCI canonicalises a PCI bus id so NVML and sysfs ids compare equal.
func NormPCI(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "pci:")
	parts := strings.Split(s, ":")
	if len(parts) < 3 {
		return s
	}
	domain := strings.TrimLeft(parts[len(parts)-3], "0")
	if domain == "" {
		domain = "0"
	}
	return domain + ":" + parts[len(parts)-2] + ":" + parts[len(parts)-1]
}

// HeadlineTemp picks the temperature that should sit on the CPU card.
func HeadlineTemp(temps []Temp) (Temp, bool) {
	prefer := []string{"tctl", "tdie", "package id 0", "cpu package", "tc0p", "tp0p", "cpu"}
	for _, name := range prefer {
		for _, t := range temps {
			if strings.EqualFold(t.Name, name) {
				return t, true
			}
		}
	}
	for _, t := range temps {
		if t.Kind == "package" {
			return t, true
		}
	}
	var best Temp
	found := false
	for _, t := range temps {
		if t.Kind != "core" && t.Kind != "cpu" {
			continue
		}
		if !found || t.Celsius > best.Celsius {
			best = t
			found = true
		}
	}
	return best, found
}

// DedupNotes returns notes with blanks and duplicates removed, order kept.
func DedupNotes(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, n := range in {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	return out
}
