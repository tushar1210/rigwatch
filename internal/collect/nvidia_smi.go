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

const nvidiaQuery = "name,utilization.gpu,temperature.gpu,fan.speed,memory.used,memory.total,power.draw,clocks.gr,clocks.mem,pci.bus_id"

// ParseNvidiaSMI parses `nvidia-smi --format=csv,noheader,nounits` output.
func ParseNvidiaSMI(out string) []metrics.GPU {
	var gpus []metrics.GPU
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "NVIDIA-SMI") || strings.HasPrefix(strings.ToLower(line), "failed") {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) < 10 {
			continue
		}
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		name := parts[0]
		if name == "" || strings.EqualFold(name, "name") {
			continue
		}
		g := metrics.GPU{
			Name:       name,
			Vendor:     "nvidia",
			Source:     "nvidia-smi",
			Util:       parseSMIGauge(parts[1], "unsupported"),
			Temp:       parseSMIGauge(parts[2], "unsupported"),
			FanPercent: parseSMIGauge(parts[3], "unsupported"),
			FanRPM:     metrics.Missing("nvidia-smi reports fan percent"),
			Power:      parseSMIGauge(parts[6], "unsupported"),
			ClockCore:  parseSMIGauge(parts[7], "unsupported"),
			ClockMem:   parseSMIGauge(parts[8], "unsupported"),
			PCI:        parts[9],
		}
		used, usedOK := parseSMIFloat(parts[4])
		total, totalOK := parseSMIFloat(parts[5])
		if usedOK || totalOK {
			g.VRAMOK = true
			if usedOK {
				g.VRAMUsed = uint64(used * 1024 * 1024)
			}
			if totalOK {
				g.VRAMTotal = uint64(total * 1024 * 1024)
			}
		} else {
			g.VRAMHint = "unsupported"
		}
		gpus = append(gpus, g)
	}
	return gpus
}

func parseSMIGauge(s, hint string) metrics.Gauge {
	v, ok := parseSMIFloat(s)
	if !ok {
		return metrics.Missing(hint)
	}
	return metrics.OK(v)
}

func parseSMIFloat(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" || strings.EqualFold(s, "[N/A]") || strings.EqualFold(s, "[Not Supported]") || strings.EqualFold(s, "N/A") {
		return 0, false
	}
	s = strings.Trim(s, "[]")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func readNvidiaSMI(ctx context.Context) ([]metrics.GPU, error) {
	bin, err := exec.LookPath("nvidia-smi")
	if err != nil {
		bin = windowsNvidiaSMI()
	}
	if bin == "" {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "--query-gpu="+nvidiaQuery, "--format=csv,noheader,nounits")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	gpus := ParseNvidiaSMI(stdout.String())
	if len(gpus) == 0 {
		return nil, errEmptySMI
	}
	return gpus, nil
}

type smiError string

func (e smiError) Error() string { return string(e) }

const errEmptySMI smiError = "nvidia-smi returned no gpus"
