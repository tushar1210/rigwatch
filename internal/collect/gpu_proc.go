package collect

import (
	"bytes"
	"context"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tushar1210/rigwatch/internal/metrics"
)

// gpuStat is one process as reported by nvidia-smi.
type gpuStat struct {
	PID      int32
	Name     string
	Util     float64
	UtilOK   bool
	Mem      uint64
	MemOK    bool
	MemPct   float64
	MemPctOK bool
}

var (
	smiOnce sync.Once
	smiBin  string

	gpuProcMu    sync.RWMutex
	gpuProcCache []gpuStat
	gpuPollOnce  sync.Once
)

func nvidiaSMIBin() string {
	smiOnce.Do(func() {
		p, err := exec.LookPath("nvidia-smi")
		if err != nil || p == "" {
			p = windowsNvidiaSMI()
		}
		smiBin = p
	})
	return smiBin
}

// attachGPUProcs fills GPU columns from nvidia-smi when it is installed.
// Utilization is sampled in the background because `nvidia-smi pmon` waits
// about a second. Memory from the compute-apps query is read inline.
func attachGPUProcs(ctx context.Context, rows []metrics.Process) []metrics.Process {
	if nvidiaSMIBin() == "" || ctx.Err() != nil {
		return rows
	}
	startGPUPoller()
	stats := gpuProcSnapshot()
	if apps, ok := queryComputeApps(ctx); ok {
		stats = overlayGPUStats(stats, apps)
	}
	return mergeGPUProcs(rows, stats)
}

func startGPUPoller() {
	gpuPollOnce.Do(func() {
		go gpuPollLoop()
	})
}

func gpuPollLoop() {
	collectGPUUtil()
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for range tick.C {
		collectGPUUtil()
	}
}

func collectGPUUtil() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	pmon, ok := runSMI(ctx, "pmon", "-c", "1", "-s", "um")
	if !ok {
		return
	}
	stats := parseNvidiaPmon(pmon)
	gpuProcMu.Lock()
	gpuProcCache = stats
	gpuProcMu.Unlock()
}

func gpuProcSnapshot() []gpuStat {
	gpuProcMu.RLock()
	defer gpuProcMu.RUnlock()
	return append([]gpuStat(nil), gpuProcCache...)
}

func queryComputeApps(ctx context.Context) ([]gpuStat, bool) {
	ctx, cancel := context.WithTimeout(ctx, 700*time.Millisecond)
	defer cancel()
	out, ok := runSMI(ctx, "--query-compute-apps=pid,process_name,used_gpu_memory", "--format=csv,noheader,nounits")
	if !ok {
		return nil, false
	}
	return parseNvidiaComputeApps(out), true
}

func runSMI(ctx context.Context, args ...string) (string, bool) {
	bin := nvidiaSMIBin()
	if bin == "" {
		return "", false
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &bytes.Buffer{}
	if err := cmd.Run(); err != nil {
		return "", false
	}
	return stdout.String(), true
}

// parseNvidiaComputeApps reads `nvidia-smi --query-compute-apps` CSV.
// used_gpu_memory is MiB. The same PID on two GPUs has its memory summed.
func parseNvidiaComputeApps(out string) []gpuStat {
	var rows []gpuStat
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "NVIDIA-SMI") {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) < 2 {
			continue
		}
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		if strings.EqualFold(parts[0], "pid") {
			continue
		}
		pid, err := strconv.ParseInt(parts[0], 10, 32)
		if err != nil || pid <= 0 {
			continue
		}
		memField := parts[len(parts)-1]
		name := ""
		if len(parts) > 2 {
			name = strings.TrimSpace(strings.Join(parts[1:len(parts)-1], ","))
		}
		st := gpuStat{PID: int32(pid), Name: name}
		if mem, ok := parseSMIFloat(memField); ok && mem >= 0 {
			st.Mem = uint64(mem * 1024 * 1024)
			st.MemOK = true
		}
		rows = append(rows, st)
	}
	return sumGPUMem(rows)
}

// parseNvidiaPmon reads `nvidia-smi pmon -c 1 -s um`.
// sm is utilization percent. mem is memory percent, not bytes.
func parseNvidiaPmon(out string) []gpuStat {
	pidI, smI, memI, nameI := -1, -1, -1, -1
	var rows []gpuStat
	for _, line := range strings.Split(out, "\n") {
		fields := pmonFields(line)
		if len(fields) == 0 {
			continue
		}
		if pidI < 0 {
			for i, f := range fields {
				switch strings.ToLower(f) {
				case "pid":
					pidI = i
				case "sm":
					smI = i
				case "mem":
					memI = i
				case "command", "name":
					nameI = i
				}
			}
			if pidI >= 0 {
				continue
			}
			pidI, smI, memI, nameI = -1, -1, -1, -1
			continue
		}
		if pidI >= len(fields) {
			continue
		}
		pid, err := strconv.ParseInt(fields[pidI], 10, 32)
		if err != nil || pid <= 0 {
			continue
		}
		st := gpuStat{PID: int32(pid)}
		if smI >= 0 && smI < len(fields) {
			if v, ok := parseSMIFloat(fields[smI]); ok {
				st.Util = v
				st.UtilOK = true
			}
		}
		if memI >= 0 && memI < len(fields) {
			if v, ok := parseSMIFloat(fields[memI]); ok {
				st.MemPct = v
				st.MemPctOK = true
			}
		}
		if nameI >= 0 && nameI < len(fields) {
			st.Name = strings.Join(fields[nameI:], " ")
		}
		rows = append(rows, st)
	}
	return maxGPUUtil(rows)
}

func pmonFields(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "#")
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	return strings.Fields(line)
}

func sumGPUMem(rows []gpuStat) []gpuStat {
	return foldGPUStats(rows, true)
}

func maxGPUUtil(rows []gpuStat) []gpuStat {
	return foldGPUStats(rows, false)
}

func foldGPUStats(rows []gpuStat, sumMem bool) []gpuStat {
	order := make([]int32, 0, len(rows))
	by := map[int32]*gpuStat{}
	for _, s := range rows {
		cur, ok := by[s.PID]
		if !ok {
			cp := s
			by[s.PID] = &cp
			order = append(order, s.PID)
			continue
		}
		if cur.Name == "" {
			cur.Name = s.Name
		}
		if s.UtilOK && (!cur.UtilOK || s.Util > cur.Util) {
			cur.Util = s.Util
			cur.UtilOK = true
		}
		if s.MemOK {
			if !cur.MemOK || !sumMem {
				if !cur.MemOK || s.Mem > cur.Mem {
					cur.Mem = s.Mem
				}
				cur.MemOK = true
			} else {
				cur.Mem += s.Mem
			}
		}
		if s.MemPctOK && (!cur.MemPctOK || s.MemPct > cur.MemPct) {
			cur.MemPct = s.MemPct
			cur.MemPctOK = true
		}
	}
	out := make([]gpuStat, 0, len(order))
	for _, pid := range order {
		out = append(out, *by[pid])
	}
	return out
}

// overlayGPUStats keeps base and fills gaps from extra.
// Utilization and byte totals use the larger reading so two sources do not double-count.
func overlayGPUStats(base, extra []gpuStat) []gpuStat {
	return foldGPUStats(append(append([]gpuStat{}, base...), extra...), false)
}

func mergeGPUProcs(rows []metrics.Process, stats []gpuStat) []metrics.Process {
	if len(stats) == 0 {
		return rows
	}
	index := make(map[int32]int, len(rows))
	for i, row := range rows {
		index[row.PID] = i
	}
	for _, s := range stats {
		if s.PID <= 0 {
			continue
		}
		if i, ok := index[s.PID]; ok {
			applyGPUStat(&rows[i], s)
			continue
		}
		row := metrics.Process{PID: s.PID, Name: s.Name}
		applyGPUStat(&row, s)
		index[s.PID] = len(rows)
		rows = append(rows, row)
	}
	return rows
}

func applyGPUStat(row *metrics.Process, s gpuStat) {
	if row.Name == "" && s.Name != "" {
		row.Name = s.Name
	}
	if s.UtilOK {
		row.GPUPercent = s.Util
		row.GPUUtilOK = true
	}
	if s.MemOK {
		row.GPUMem = s.Mem
		row.GPUMemOK = true
	}
	if s.MemPctOK {
		row.GPUMemPercent = s.MemPct
		row.GPUMemPctOK = true
	}
}
