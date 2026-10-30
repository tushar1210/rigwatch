package collect

import (
	"bytes"
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/tushar1210/rigwatch/internal/metrics"
)

// gpuProcStat is one PID reported by nvidia-smi.
// SM is streaming-multiprocessor percent. MemBytes is framebuffer bytes.
type gpuProcStat struct {
	PID      int32
	Name     string
	SM       float64
	HasSM    bool
	MemBytes uint64
	HasMem   bool
}

// ParseNvidiaPmon parses `nvidia-smi pmon` output.
// The sm column is GPU percent. An fb column, when present, is framebuffer MiB.
// The mem column is memory-controller percent and is not treated as VRAM.
func ParseNvidiaPmon(out string) []gpuProcStat {
	header := []string{"gpu", "pid", "type", "sm", "mem", "enc", "dec", "command"}
	sawHeader := false
	var rows []gpuProcStat
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			fields := strings.Fields(strings.TrimPrefix(line, "#"))
			if fieldIndex(fields, "pid") >= 0 && fieldIndex(fields, "sm") >= 0 {
				header = fields
				sawHeader = true
			}
			continue
		}
		if !sawHeader && len(strings.Fields(line)) < 4 {
			continue
		}
		fields := strings.Fields(line)
		pid, ok := parsePID(fieldAt(fields, fieldIndex(header, "pid")))
		if !ok || pid <= 0 {
			continue
		}
		st := gpuProcStat{PID: pid}
		if sm, ok := parseSMIFloat(fieldAt(fields, fieldIndex(header, "sm"))); ok {
			st.SM = sm
			st.HasSM = true
		}
		if fb := fieldIndex(header, "fb"); fb >= 0 {
			if mb, ok := parseSMIFloat(fieldAt(fields, fb)); ok {
				st.MemBytes = uint64(mb * 1024 * 1024)
				st.HasMem = true
			}
		}
		if cmd := fieldIndex(header, "command"); cmd >= 0 && cmd < len(fields) {
			st.Name = strings.Join(fields[cmd:], " ")
		}
		rows = append(rows, st)
	}
	return collapseGPUProcs(rows, true)
}

// ParseNvidiaComputeApps parses
// `nvidia-smi --query-compute-apps=pid,process_name,used_gpu_memory --format=csv,noheader,nounits`.
// The last column is used GPU memory in MiB.
func ParseNvidiaComputeApps(out string) []gpuProcStat {
	var rows []gpuProcStat
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) < 2 {
			continue
		}
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		pid, ok := parsePID(parts[0])
		if !ok || pid <= 0 {
			continue
		}
		st := gpuProcStat{PID: pid}
		if len(parts) >= 3 {
			st.Name = strings.Trim(parts[1], `"`)
		}
		if mb, ok := parseSMIFloat(parts[len(parts)-1]); ok {
			st.MemBytes = uint64(mb * 1024 * 1024)
			st.HasMem = true
		}
		rows = append(rows, st)
	}
	return collapseGPUProcs(rows, true)
}

func fieldIndex(fields []string, name string) int {
	for i, f := range fields {
		if strings.EqualFold(f, name) {
			return i
		}
	}
	return -1
}

func fieldAt(fields []string, i int) string {
	if i < 0 || i >= len(fields) {
		return ""
	}
	return fields[i]
}

func parsePID(s string) (int32, bool) {
	s = strings.TrimSpace(s)
	if s == "" || s == "-" {
		return 0, false
	}
	n, err := strconv.ParseInt(s, 10, 32)
	if err != nil || n <= 0 {
		return 0, false
	}
	return int32(n), true
}

// collapseGPUProcs merges rows that share a PID.
// Memory bytes are summed when sumMem is set (several GPUs). SM keeps the max.
func collapseGPUProcs(rows []gpuProcStat, sumMem bool) []gpuProcStat {
	idx := map[int32]int{}
	var out []gpuProcStat
	for _, s := range rows {
		i, ok := idx[s.PID]
		if !ok {
			idx[s.PID] = len(out)
			out = append(out, s)
			continue
		}
		cur := &out[i]
		if s.HasSM && (!cur.HasSM || s.SM > cur.SM) {
			cur.HasSM = true
			cur.SM = s.SM
		}
		if s.HasMem {
			if !cur.HasMem || !sumMem {
				if !cur.HasMem || s.MemBytes > cur.MemBytes {
					cur.MemBytes = s.MemBytes
				}
			} else {
				cur.MemBytes += s.MemBytes
			}
			cur.HasMem = true
		}
		if cur.Name == "" {
			cur.Name = s.Name
		}
	}
	return out
}

// mergeGPUProcStats combines two sources. Memory keeps the larger reading so
// pmon framebuffer and compute-apps bytes are not added twice.
func mergeGPUProcStats(dst, src []gpuProcStat) []gpuProcStat {
	return collapseGPUProcs(append(append([]gpuProcStat{}, dst...), src...), false)
}

func applyGPUProcs(rows []metrics.Process, stats []gpuProcStat) []metrics.Process {
	if len(stats) == 0 {
		return rows
	}
	idx := map[int32]int{}
	for i, row := range rows {
		idx[row.PID] = i
	}
	for _, st := range stats {
		i, ok := idx[st.PID]
		if !ok {
			name := st.Name
			if name == "" {
				name = "gpu"
			}
			rows = append(rows, metrics.Process{PID: st.PID, Name: name})
			i = len(rows) - 1
			idx[st.PID] = i
		}
		if st.HasSM {
			rows[i].HasGPU = true
			if st.SM > rows[i].GPUPercent {
				rows[i].GPUPercent = st.SM
			}
		}
		if st.HasMem {
			rows[i].HasGPUMem = true
			if st.MemBytes > rows[i].GPUMemBytes {
				rows[i].GPUMemBytes = st.MemBytes
			}
		}
		if rows[i].Name == "" && st.Name != "" {
			rows[i].Name = st.Name
		}
	}
	return rows
}

func nvidiaSMIBin() string {
	if bin, err := exec.LookPath("nvidia-smi"); err == nil && bin != "" {
		return bin
	}
	return windowsNvidiaSMI()
}

func runSMI(ctx context.Context, bin string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &bytes.Buffer{}
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return stdout.String(), nil
}

// gpuProcessStats returns the last nvidia-smi process sample.
// The refresh runs in the background so pmon's one-second delay does not stall the UI.
func (l *Live) gpuProcessStats() []gpuProcStat {
	l.gpuMu.Lock()
	defer l.gpuMu.Unlock()
	stats := append([]gpuProcStat(nil), l.gpuStats...)
	if l.gpuMissing || l.gpuBusy || time.Now().Before(l.gpuNext) {
		return stats
	}
	l.gpuBusy = true
	go l.refreshGPUProcs()
	return stats
}

func (l *Live) refreshGPUProcs() {
	bin := nvidiaSMIBin()
	if bin == "" {
		l.gpuMu.Lock()
		l.gpuMissing = true
		l.gpuBusy = false
		l.gpuMu.Unlock()
		return
	}
	var stats []gpuProcStat
	okN := 0
	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	out, err := runSMI(ctx, bin, "--query-compute-apps=pid,process_name,used_gpu_memory", "--format=csv,noheader,nounits")
	cancel()
	if err == nil {
		okN++
		stats = mergeGPUProcStats(stats, ParseNvidiaComputeApps(out))
	}
	ctx, cancel = context.WithTimeout(context.Background(), 1500*time.Millisecond)
	pout, err := runSMI(ctx, bin, "pmon", "-c", "1")
	cancel()
	if err == nil {
		okN++
		stats = mergeGPUProcStats(stats, ParseNvidiaPmon(pout))
	}
	wait := 2 * time.Second
	if okN == 0 {
		wait = 30 * time.Second
	}
	l.gpuMu.Lock()
	l.gpuStats = stats
	l.gpuNext = time.Now().Add(wait)
	l.gpuBusy = false
	l.gpuMu.Unlock()
}
