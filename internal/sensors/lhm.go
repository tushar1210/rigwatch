package sensors

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/tushar1210/rigwatch/internal/metrics"
)

// LHMNode is one node in LibreHardwareMonitor's data.json tree.
type LHMNode struct {
	Text     string    `json:"Text"`
	Value    string    `json:"Value"`
	Min      string    `json:"Min"`
	Max      string    `json:"Max"`
	Children []LHMNode `json:"Children"`
}

// LHMReading is the hardware LibreHardwareMonitor reported.
type LHMReading struct {
	Temps []metrics.Temp
	Fans  []metrics.Fan
	GPUs  []metrics.GPU
}

// ParseLHMJSON walks a LibreHardwareMonitor data.json document.
func ParseLHMJSON(data []byte) (LHMReading, error) {
	var node LHMNode
	if err := json.Unmarshal(data, &node); err != nil {
		return LHMReading{}, err
	}
	var out LHMReading
	walkLHM(&node, "", "", &out)
	return out, nil
}

func walkLHM(n *LHMNode, hardware, group string, out *LHMReading) {
	name := strings.TrimSpace(n.Text)
	if isHardwareNode(n) {
		hardware = name
		for i := range n.Children {
			walkLHM(&n.Children[i], hardware, "", out)
		}
		return
	}
	if isGroup(name) {
		for i := range n.Children {
			walkLHM(&n.Children[i], hardware, canonicalGroup(name), out)
		}
		return
	}
	if n.Value != "" && hardware != "" && group != "" {
		applyLHMSensor(hardware, group, name, n.Value, out)
	}
	for i := range n.Children {
		walkLHM(&n.Children[i], hardware, group, out)
	}
}

func isGroup(name string) bool {
	switch canonicalGroup(name) {
	case "temperatures", "fans", "load", "clocks", "power", "data", "controls":
		return true
	default:
		return false
	}
}

func canonicalGroup(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "temperatures", "temperature":
		return "temperatures"
	case "fans", "fan":
		return "fans"
	case "load", "loads":
		return "load"
	case "clocks", "clock":
		return "clocks"
	case "powers", "power":
		return "power"
	case "data":
		return "data"
	case "controls", "control":
		return "controls"
	default:
		return ""
	}
}

func isHardwareNode(n *LHMNode) bool {
	if n == nil || len(n.Children) == 0 {
		return false
	}
	groups := 0
	for _, c := range n.Children {
		if isGroup(c.Text) {
			groups++
		}
	}
	return groups > 0
}

func applyLHMSensor(hardware, group, sensor, value string, out *LHMReading) {
	num, unit, ok := parseLHMValue(value)
	if !ok {
		return
	}
	gpu := gpuByName(out, hardware)
	switch group {
	case "temperatures":
		kind := "other"
		if gpu != nil {
			kind = "gpu"
			low := strings.ToLower(sensor)
			if !strings.Contains(low, "memory") && (!gpu.Temp.Available || strings.Contains(low, "core")) {
				gpu.Temp = metrics.OK(num)
			}
		} else if looksCPU(hardware) {
			kind = cpuTempKind(sensor)
		}
		out.Temps = append(out.Temps, metrics.Temp{
			Name:    hardware + " " + sensor,
			Celsius: num,
			Source:  "lhm",
			Kind:    kind,
		})
	case "fans":
		fan := metrics.Fan{Name: strings.TrimSpace(hardware + " " + sensor), Source: "lhm"}
		switch unit {
		case "rpm":
			fan.RPM = num
			fan.HasRPM = true
			if gpu != nil && !gpu.FanRPM.Available {
				gpu.FanRPM = metrics.OK(num)
			}
		case "%":
			fan.Percent = num
			fan.HasPercent = true
			if gpu != nil && !gpu.FanPercent.Available {
				gpu.FanPercent = metrics.OK(num)
			}
		}
		if fan.HasRPM || fan.HasPercent {
			out.Fans = append(out.Fans, fan)
		}
	case "load":
		if gpu != nil && unit == "%" && strings.Contains(strings.ToLower(sensor), "core") && !gpu.Util.Available {
			gpu.Util = metrics.OK(num)
		}
	case "power":
		if gpu != nil && unit == "w" && !gpu.Power.Available {
			gpu.Power = metrics.OK(num)
		}
	case "clocks":
		if gpu == nil || unit != "mhz" {
			return
		}
		low := strings.ToLower(sensor)
		if strings.Contains(low, "memory") && !gpu.ClockMem.Available {
			gpu.ClockMem = metrics.OK(num)
		} else if strings.Contains(low, "core") && !gpu.ClockCore.Available {
			gpu.ClockCore = metrics.OK(num)
		}
	case "data":
		if gpu == nil {
			return
		}
		low := strings.ToLower(sensor)
		bytes := lhmBytes(num, unit)
		if strings.Contains(low, "memory") && strings.Contains(low, "used") {
			gpu.VRAMUsed = bytes
			gpu.VRAMOK = true
		}
		if strings.Contains(low, "memory") && strings.Contains(low, "total") {
			gpu.VRAMTotal = bytes
			gpu.VRAMOK = true
		}
	case "controls":
		if gpu != nil && unit == "%" && !gpu.FanPercent.Available {
			gpu.FanPercent = metrics.OK(num)
		}
	}
}

func gpuByName(out *LHMReading, hardware string) *metrics.GPU {
	if !looksGPU(hardware) {
		return nil
	}
	for i := range out.GPUs {
		if out.GPUs[i].Name == hardware {
			return &out.GPUs[i]
		}
	}
	out.GPUs = append(out.GPUs, metrics.GPU{
		Name:   hardware,
		Vendor: vendorFromName(hardware),
		Source: "lhm",
		Util:   metrics.Missing("start LibreHardwareMonitor"),
		Temp:   metrics.Missing("start LibreHardwareMonitor"),
	})
	return &out.GPUs[len(out.GPUs)-1]
}

func looksGPU(name string) bool {
	l := strings.ToLower(name)
	keys := []string{"gpu", "geforce", "rtx", "gtx", "radeon", "nvidia", "arc ", "intel arc"}
	for _, k := range keys {
		if strings.Contains(l, k) {
			return true
		}
	}
	return false
}

func looksCPU(name string) bool {
	l := strings.ToLower(name)
	return strings.Contains(l, "cpu") || strings.Contains(l, "ryzen") || strings.Contains(l, "core i") || strings.Contains(l, "intel")
}

func cpuTempKind(sensor string) string {
	l := strings.ToLower(sensor)
	switch {
	case strings.Contains(l, "tctl") || strings.Contains(l, "tdie") || strings.Contains(l, "package"):
		return "package"
	case strings.Contains(l, "core"):
		return "core"
	default:
		return "cpu"
	}
}

func vendorFromName(name string) string {
	l := strings.ToLower(name)
	switch {
	case strings.Contains(l, "nvidia") || strings.Contains(l, "geforce") || strings.Contains(l, "rtx") || strings.Contains(l, "gtx"):
		return "nvidia"
	case strings.Contains(l, "radeon") || strings.Contains(l, "amd"):
		return "amd"
	case strings.Contains(l, "arc") || strings.Contains(l, "intel"):
		return "intel"
	default:
		return "unknown"
	}
}

func parseLHMValue(v string) (float64, string, bool) {
	v = strings.TrimSpace(v)
	if v == "" || v == "-" {
		return 0, "", false
	}
	fields := strings.Fields(v)
	if len(fields) == 0 {
		return 0, "", false
	}
	n, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, "", false
	}
	unit := ""
	if len(fields) > 1 {
		unit = strings.ToLower(fields[1])
	}
	return n, unit, true
}

func lhmBytes(n float64, unit string) uint64 {
	switch unit {
	case "kb":
		return uint64(n * 1024)
	case "mb":
		return uint64(n * 1024 * 1024)
	case "gb":
		return uint64(n * 1024 * 1024 * 1024)
	case "tb":
		return uint64(n * 1024 * 1024 * 1024 * 1024)
	default:
		return uint64(n)
	}
}

// SensorFromWMI records one LibreHardwareMonitor WMI sensor.
type SensorFromWMI struct {
	Hardware     string
	HardwareType string
	Name         string
	SensorType   string
	Value        float64
}

// FromWMISensors builds a reading from flattened WMI rows.
func FromWMISensors(rows []SensorFromWMI) LHMReading {
	var out LHMReading
	for _, row := range rows {
		hardware := row.Hardware
		if hardware == "" {
			hardware = row.HardwareType
		}
		group := wmiGroup(row.SensorType)
		value := formatWMI(row.Value, row.SensorType)
		applyLHMSensor(hardware, group, row.Name, value, &out)
	}
	return out
}

func wmiGroup(sensorType string) string {
	return canonicalWMI(sensorType)
}

func canonicalWMI(sensorType string) string {
	switch strings.ToLower(strings.TrimSpace(sensorType)) {
	case "temperature", "4":
		return "temperatures"
	case "fan", "7":
		return "fans"
	case "load", "5":
		return "load"
	case "clock", "3":
		return "clocks"
	case "power", "2":
		return "power"
	case "data", "12":
		return "data"
	case "smalldata", "13":
		return "data"
	case "control", "9":
		return "controls"
	default:
		return ""
	}
}

func formatWMI(v float64, sensorType string) string {
	switch canonicalWMI(sensorType) {
	case "temperatures":
		return strconv.FormatFloat(v, 'f', 2, 64) + " °C"
	case "fans":
		return strconv.FormatFloat(v, 'f', 0, 64) + " RPM"
	case "load", "controls":
		return strconv.FormatFloat(v, 'f', 1, 64) + " %"
	case "power":
		return strconv.FormatFloat(v, 'f', 2, 64) + " W"
	case "clocks":
		return strconv.FormatFloat(v, 'f', 0, 64) + " MHz"
	case "data":
		return strconv.FormatFloat(v, 'f', 2, 64) + " GB"
	default:
		return strconv.FormatFloat(v, 'f', 2, 64)
	}
}
