package metrics

import "sort"

// ProcessSort selects the process ordering used by the TUI and the collector.
type ProcessSort int

const (
	// SortByCPU orders by CPU percent, highest first.
	SortByCPU ProcessSort = iota
	// SortByRSS orders by resident set (working set on Windows).
	SortByRSS
	// SortByGPU orders by GPU utilization, then GPU memory.
	SortByGPU
	// SortByVRAM orders by GPU memory, then GPU utilization.
	SortByVRAM
)

// SortProcesses returns a copy of rows ordered by by.
// Ties break on RSS, then on PID so the order is stable.
func SortProcesses(rows []Process, by ProcessSort) []Process {
	out := append([]Process(nil), rows...)
	sort.Slice(out, func(i, j int) bool {
		return processLess(out[i], out[j], by)
	})
	return out
}

func processLess(a, b Process, by ProcessSort) bool {
	switch by {
	case SortByRSS:
		if a.RSS != b.RSS {
			return a.RSS > b.RSS
		}
		if a.MemPercent != b.MemPercent {
			return a.MemPercent > b.MemPercent
		}
		return a.PID < b.PID
	case SortByGPU:
		if a.GPUUtilOK != b.GPUUtilOK {
			return a.GPUUtilOK
		}
		if a.GPUUtilOK && a.GPUPercent != b.GPUPercent {
			return a.GPUPercent > b.GPUPercent
		}
		if less, ok := vramLess(a, b); ok {
			return less
		}
		if a.RSS != b.RSS {
			return a.RSS > b.RSS
		}
		return a.PID < b.PID
	case SortByVRAM:
		if less, ok := vramLess(a, b); ok {
			return less
		}
		if a.GPUUtilOK != b.GPUUtilOK {
			return a.GPUUtilOK
		}
		if a.GPUUtilOK && a.GPUPercent != b.GPUPercent {
			return a.GPUPercent > b.GPUPercent
		}
		if a.RSS != b.RSS {
			return a.RSS > b.RSS
		}
		return a.PID < b.PID
	default:
		if a.CPUPercent != b.CPUPercent {
			return a.CPUPercent > b.CPUPercent
		}
		if a.RSS != b.RSS {
			return a.RSS > b.RSS
		}
		return a.PID < b.PID
	}
}

// vramLess reports whether a should rank above b when GPU memory differs.
// Bytes win over a percent-only reading. ok is false when the two tie.
func vramLess(a, b Process) (less bool, ok bool) {
	if a.GPUMemOK || b.GPUMemOK {
		if a.GPUMemOK != b.GPUMemOK {
			return a.GPUMemOK, true
		}
		if a.GPUMem != b.GPUMem {
			return a.GPUMem > b.GPUMem, true
		}
		return false, false
	}
	if a.GPUMemPctOK != b.GPUMemPctOK {
		return a.GPUMemPctOK, true
	}
	if a.GPUMemPctOK && a.GPUMemPercent != b.GPUMemPercent {
		return a.GPUMemPercent > b.GPUMemPercent, true
	}
	return false, false
}
