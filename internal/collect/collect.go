// Package collect samples host vitals. Platform gaps are reported on the
// snapshot instead of failing the sample.
package collect

import (
	"context"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"

	"github.com/tushar1210/rigwatch/internal/metrics"
	"github.com/tushar1210/rigwatch/internal/sensors"
)

// Collector samples a machine once.
type Collector interface {
	Sample(ctx context.Context) (metrics.Snapshot, error)
}

// Live reads the current machine through gopsutil and the platform sensors.
type Live struct {
	LHMURL   string
	prevDisk map[string]diskCount
	prevNet  map[string]netCount
	prevAt   time.Time

	gpuMu      sync.Mutex
	gpuStats   []gpuProcStat
	gpuNext    time.Time
	gpuBusy    bool
	gpuMissing bool
}

type diskCount struct {
	read  uint64
	write uint64
}

type netCount struct {
	rx uint64
	tx uint64
}

// NewLive returns a collector for this machine.
// lhmURL is the LibreHardwareMonitor JSON endpoint, used on Windows.
func NewLive(lhmURL string) *Live {
	_, _ = cpu.Percent(0, false)
	_, _ = cpu.Percent(0, true)
	return &Live{
		LHMURL:   lhmURL,
		prevDisk: map[string]diskCount{},
		prevNet:  map[string]netCount{},
		prevAt:   time.Now(),
	}
}

// Sample gathers one snapshot. It returns an error only when ctx is cancelled.
func (l *Live) Sample(ctx context.Context) (metrics.Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return metrics.Snapshot{}, err
	}
	now := time.Now()
	elapsed := now.Sub(l.prevAt)
	if elapsed <= 0 {
		elapsed = time.Second
	}
	snap := metrics.Snapshot{Time: now}
	l.fillHost(ctx, &snap)
	l.fillCPU(ctx, &snap)
	l.fillMem(ctx, &snap)
	l.fillDisks(ctx, &snap, elapsed)
	l.fillNet(ctx, &snap, elapsed)
	l.fillProcs(ctx, &snap)
	gpus := collectGPUs(ctx)
	reading := sensors.Read(ctx, l.LHMURL)
	snap.Temps = reading.Temps
	snap.Fans = reading.Fans
	snap.GPUs = mergeGPUs(gpus, reading.GPUs)
	if reading.Hint != "" {
		snap.Notes = append(snap.Notes, reading.Hint)
	}
	foldSensors(&snap)
	snap.Notes = metrics.DedupNotes(snap.Notes)
	l.prevAt = now
	return snap, nil
}

func (l *Live) fillHost(ctx context.Context, snap *metrics.Snapshot) {
	info, err := host.InfoWithContext(ctx)
	if err != nil || info == nil {
		name, _ := hostFallback()
		snap.Host.Hostname = name
		snap.Host.OS = runtime.GOOS
		snap.Host.Arch = runtime.GOARCH
		return
	}
	snap.Host = metrics.Host{
		Hostname:      info.Hostname,
		OS:            info.OS,
		Platform:      strings.TrimSpace(info.Platform + " " + info.PlatformVersion),
		Arch:          info.KernelArch,
		Kernel:        info.KernelVersion,
		UptimeSeconds: info.Uptime,
	}
	if snap.Host.Arch == "" {
		snap.Host.Arch = runtime.GOARCH
	}
}

func hostFallback() (string, error) {
	return osHostname()
}

func (l *Live) fillCPU(ctx context.Context, snap *metrics.Snapshot) {
	if total, err := cpu.PercentWithContext(ctx, 0, false); err == nil && len(total) > 0 {
		snap.CPU.UsagePercent = total[0]
	}
	if cores, err := cpu.PercentWithContext(ctx, 0, true); err == nil {
		snap.CPU.PerCore = cores
	}
	if mhz, ok := currentFreqMHz(); ok && mhz > 0 {
		snap.CPU.FrequencyMHz = metrics.OK(mhz)
	} else if info, err := cpu.InfoWithContext(ctx); err == nil && len(info) > 0 && info[0].Mhz > 0 {
		snap.CPU.FrequencyMHz = metrics.OK(info[0].Mhz)
	} else {
		snap.CPU.FrequencyMHz = metrics.Missing("frequency not exposed")
	}
	avg, err := load.AvgWithContext(ctx)
	if err != nil || avg == nil {
		snap.CPU.Load1 = metrics.Missing("no load average")
		snap.CPU.Load5 = metrics.Missing("no load average")
		snap.CPU.Load15 = metrics.Missing("no load average")
		return
	}
	snap.CPU.Load1 = metrics.OK(avg.Load1)
	snap.CPU.Load5 = metrics.OK(avg.Load5)
	snap.CPU.Load15 = metrics.OK(avg.Load15)
}

func (l *Live) fillMem(ctx context.Context, snap *metrics.Snapshot) {
	vm, err := mem.VirtualMemoryWithContext(ctx)
	if err == nil && vm != nil {
		snap.Memory.Total = vm.Total
		snap.Memory.Used = vm.Used
		snap.Memory.Available = vm.Available
		snap.Memory.UsedPercent = vm.UsedPercent
		snap.Memory.Cached = vm.Cached
		snap.Memory.CachedAvailable = vm.Cached > 0 || runtime.GOOS == "linux"
	}
	sw, err := mem.SwapMemoryWithContext(ctx)
	label := "swap"
	if runtime.GOOS == "windows" {
		label = "pagefile"
	}
	snap.Memory.SwapLabel = label
	if err == nil && sw != nil {
		snap.Memory.SwapTotal = sw.Total
		snap.Memory.SwapUsed = sw.Used
		snap.Memory.SwapPercent = sw.UsedPercent
	}
}

func (l *Live) fillDisks(ctx context.Context, snap *metrics.Snapshot, elapsed time.Duration) {
	counters, _ := disk.IOCountersWithContext(ctx)
	next := map[string]diskCount{}
	for name, c := range counters {
		next[name] = diskCount{read: c.ReadBytes, write: c.WriteBytes}
	}
	parts, err := disk.PartitionsWithContext(ctx, false)
	if err != nil {
		l.prevDisk = next
		return
	}
	for _, p := range parts {
		if skipPartition(p) {
			continue
		}
		usage, err := disk.UsageWithContext(ctx, p.Mountpoint)
		if err != nil || usage == nil || usage.Total == 0 {
			continue
		}
		d := metrics.Disk{
			Name:        diskLabel(p.Device, p.Mountpoint),
			Mount:       p.Mountpoint,
			Total:       usage.Total,
			Used:        usage.Used,
			UsedPercent: usage.UsedPercent,
		}
		dev := parentDevice(p.Device)
		if cur, ok := next[dev]; ok {
			if prev, ok := l.prevDisk[dev]; ok {
				d.ReadBytesPerSec = perSec(prev.read, cur.read, elapsed)
				d.WriteBytesPerSec = perSec(prev.write, cur.write, elapsed)
				d.ThroughputOK = true
			}
		}
		snap.Disks = append(snap.Disks, d)
	}
	sort.Slice(snap.Disks, func(i, j int) bool {
		return snap.Disks[i].Total > snap.Disks[j].Total
	})
	l.prevDisk = next
}

func (l *Live) fillNet(ctx context.Context, snap *metrics.Snapshot, elapsed time.Duration) {
	counters, err := net.IOCountersWithContext(ctx, true)
	if err != nil {
		return
	}
	next := map[string]netCount{}
	for _, c := range counters {
		if skipNIC(c.Name) {
			continue
		}
		next[c.Name] = netCount{rx: c.BytesRecv, tx: c.BytesSent}
		nic := metrics.NIC{Name: c.Name}
		if prev, ok := l.prevNet[c.Name]; ok {
			nic.RxBytesPerSec = perSec(prev.rx, c.BytesRecv, elapsed)
			nic.TxBytesPerSec = perSec(prev.tx, c.BytesSent, elapsed)
		}
		snap.Network = append(snap.Network, nic)
	}
	sort.Slice(snap.Network, func(i, j int) bool {
		a := snap.Network[i].RxBytesPerSec + snap.Network[i].TxBytesPerSec
		b := snap.Network[j].RxBytesPerSec + snap.Network[j].TxBytesPerSec
		if a == b {
			return snap.Network[i].Name < snap.Network[j].Name
		}
		return a > b
	})
	l.prevNet = next
}

func (l *Live) fillProcs(ctx context.Context, snap *metrics.Snapshot) {
	list, err := process.ProcessesWithContext(ctx)
	if err != nil {
		snap.Notes = append(snap.Notes, "process list unavailable")
		return
	}
	rows := make([]metrics.Process, 0, len(list))
	for _, p := range list {
		if ctx.Err() != nil {
			return
		}
		name, err := p.NameWithContext(ctx)
		if err != nil || name == "" {
			continue
		}
		cpuPct, _ := p.CPUPercentWithContext(ctx)
		memPct, _ := p.MemoryPercentWithContext(ctx)
		var rss uint64
		if info, err := p.MemoryInfoWithContext(ctx); err == nil && info != nil {
			rss = info.RSS
		}
		rows = append(rows, metrics.Process{
			PID:        p.Pid,
			Name:       name,
			CPUPercent: cpuPct,
			MemPercent: float64(memPct),
			RSS:        rss,
		})
	}
	rows = applyGPUProcs(rows, l.gpuProcessStats())
	snap.Processes = topProcesses(rows, 12)
}

func topProcesses(rows []metrics.Process, n int) []metrics.Process {
	if len(rows) == 0 {
		return nil
	}
	byCPU := append([]metrics.Process(nil), rows...)
	sort.Slice(byCPU, func(i, j int) bool { return byCPU[i].CPUPercent > byCPU[j].CPUPercent })
	byMem := append([]metrics.Process(nil), rows...)
	sort.Slice(byMem, func(i, j int) bool { return byMem[i].MemPercent > byMem[j].MemPercent })
	seen := map[int32]struct{}{}
	var out []metrics.Process
	take := func(list []metrics.Process, limit int) {
		added := 0
		for _, p := range list {
			if added >= limit || len(out) >= n+4 {
				return
			}
			if _, ok := seen[p.PID]; ok {
				continue
			}
			seen[p.PID] = struct{}{}
			out = append(out, p)
			added++
		}
	}
	take(byCPU, n)
	take(byMem, 8)
	for _, p := range rows {
		if !p.HasGPU && !p.HasGPUMem {
			continue
		}
		if _, ok := seen[p.PID]; ok {
			continue
		}
		seen[p.PID] = struct{}{}
		out = append(out, p)
	}
	return out
}

func perSec(prev, cur uint64, elapsed time.Duration) float64 {
	if elapsed <= 0 || cur < prev {
		return 0
	}
	return float64(cur-prev) / elapsed.Seconds()
}

// Fake is a collector that returns a fixed snapshot.
type Fake struct {
	Snap  metrics.Snapshot
	Err   error
	Calls int
}

// Sample returns the canned snapshot.
func (f *Fake) Sample(ctx context.Context) (metrics.Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return metrics.Snapshot{}, err
	}
	f.Calls++
	if f.Err != nil {
		return metrics.Snapshot{}, f.Err
	}
	s := f.Snap
	if s.Time.IsZero() {
		s.Time = time.Now()
	}
	return s, nil
}
