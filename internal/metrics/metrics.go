// Package metrics holds the snapshot rigwatch collects, streams, and draws.
package metrics

import "time"

// Snapshot is one sample of machine health.
type Snapshot struct {
	Time      time.Time `json:"time"`
	Host      Host      `json:"host"`
	CPU       CPU       `json:"cpu"`
	Memory    Memory    `json:"memory"`
	GPUs      []GPU     `json:"gpus"`
	Fans      []Fan     `json:"fans"`
	Temps     []Temp    `json:"temps"`
	Disks     []Disk    `json:"disks"`
	Network   []NIC     `json:"network"`
	Processes []Process `json:"processes"`
	Notes     []string  `json:"notes,omitempty"`
}

// Host identifies the machine the sample came from.
type Host struct {
	Hostname      string `json:"hostname"`
	OS            string `json:"os"`
	Platform      string `json:"platform"`
	Arch          string `json:"arch"`
	Kernel        string `json:"kernel"`
	UptimeSeconds uint64 `json:"uptime_seconds"`
}

// CPU is total and per-core utilisation plus frequency and load.
type CPU struct {
	UsagePercent float64   `json:"usage_percent"`
	PerCore      []float64 `json:"per_core_percent"`
	FrequencyMHz Gauge     `json:"frequency_mhz"`
	Load1        Gauge     `json:"load1"`
	Load5        Gauge     `json:"load5"`
	Load15       Gauge     `json:"load15"`
}

// Memory is RAM and swap or pagefile.
type Memory struct {
	Total           uint64  `json:"total_bytes"`
	Used            uint64  `json:"used_bytes"`
	Available       uint64  `json:"available_bytes"`
	Cached          uint64  `json:"cached_bytes"`
	CachedAvailable bool    `json:"cached_available"`
	UsedPercent     float64 `json:"used_percent"`
	SwapTotal       uint64  `json:"swap_total_bytes"`
	SwapUsed        uint64  `json:"swap_used_bytes"`
	SwapPercent     float64 `json:"swap_percent"`
	SwapLabel       string  `json:"swap_label"`
}

// GPU is one graphics adapter. Unavailable fields carry a short hint.
type GPU struct {
	Name       string `json:"name"`
	Vendor     string `json:"vendor"`
	Source     string `json:"source"`
	PCI        string `json:"pci,omitempty"`
	Util       Gauge  `json:"util_percent"`
	Temp       Gauge  `json:"temp_c"`
	FanRPM     Gauge  `json:"fan_rpm"`
	FanPercent Gauge  `json:"fan_percent"`
	VRAMUsed   uint64 `json:"vram_used_bytes"`
	VRAMTotal  uint64 `json:"vram_total_bytes"`
	VRAMOK     bool   `json:"vram_available"`
	VRAMHint   string `json:"vram_hint,omitempty"`
	Power      Gauge  `json:"power_watts"`
	ClockCore  Gauge  `json:"clock_core_mhz"`
	ClockMem   Gauge  `json:"clock_mem_mhz"`
	Unified    bool   `json:"unified_memory,omitempty"`
}

// Fan is one exposed fan tachometer or duty reading.
type Fan struct {
	Name       string  `json:"name"`
	RPM        float64 `json:"rpm,omitempty"`
	Percent    float64 `json:"percent,omitempty"`
	HasRPM     bool    `json:"has_rpm"`
	HasPercent bool    `json:"has_percent"`
	Source     string  `json:"source"`
}

// Temp is one temperature sensor in Celsius.
type Temp struct {
	Name    string  `json:"name"`
	Celsius float64 `json:"celsius"`
	Source  string  `json:"source"`
	Kind    string  `json:"kind,omitempty"`
}

// Disk is a mounted filesystem and its recent throughput.
type Disk struct {
	Name             string  `json:"name"`
	Mount            string  `json:"mount"`
	Total            uint64  `json:"total_bytes"`
	Used             uint64  `json:"used_bytes"`
	UsedPercent      float64 `json:"used_percent"`
	ReadBytesPerSec  float64 `json:"read_bytes_per_sec"`
	WriteBytesPerSec float64 `json:"write_bytes_per_sec"`
	ThroughputOK     bool    `json:"throughput_available"`
}

// NIC is one non-loopback interface and its recent throughput.
type NIC struct {
	Name          string  `json:"name"`
	RxBytesPerSec float64 `json:"rx_bytes_per_sec"`
	TxBytesPerSec float64 `json:"tx_bytes_per_sec"`
}

// Process is a process row for the top list.
// GPU fields are set only when a sampler such as nvidia-smi reported them.
type Process struct {
	PID         int32   `json:"pid"`
	Name        string  `json:"name"`
	CPUPercent  float64 `json:"cpu_percent"`
	MemPercent  float64 `json:"mem_percent"`
	RSS         uint64  `json:"rss_bytes"`
	GPUPercent  float64 `json:"gpu_percent,omitempty"`
	HasGPU      bool    `json:"has_gpu,omitempty"`
	GPUMemBytes uint64  `json:"gpu_mem_bytes,omitempty"`
	HasGPUMem   bool    `json:"has_gpu_mem,omitempty"`
}

// Gauge is a float reading that may be missing.
type Gauge struct {
	Value     float64 `json:"value"`
	Available bool    `json:"available"`
	Hint      string  `json:"hint,omitempty"`
}

// OK returns an available gauge.
func OK(v float64) Gauge {
	return Gauge{Value: v, Available: true}
}

// Missing returns an unavailable gauge with a short hint.
func Missing(hint string) Gauge {
	return Gauge{Hint: hint}
}

// DisplayName is the label used in the header for this host.
func (h Host) DisplayName() string {
	if h.Hostname != "" {
		return h.Hostname
	}
	return "unknown"
}

// PlatformLine is a short OS description such as "ubuntu amd64".
func (h Host) PlatformLine() string {
	name := h.Platform
	if name == "" {
		name = h.OS
	}
	if name == "" {
		name = "unknown"
	}
	if h.Arch != "" {
		return name + " " + h.Arch
	}
	return name
}
