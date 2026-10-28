//go:build windows

package sensors

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/yusufpapurcu/wmi"

	"github.com/tushar1210/rigwatch/internal/metrics"
)

func readPlatform(ctx context.Context, lhmURL string) Reading {
	var notes []string
	if rows, err := queryLHM(); err == nil && len(rows) > 0 {
		parsed := FromWMISensors(rows)
		return Reading{Temps: parsed.Temps, Fans: parsed.Fans, GPUs: parsed.GPUs}
	}
	notes = append(notes, "start LibreHardwareMonitor")

	if lhmURL != "" {
		if parsed, err := fetchLHM(ctx, lhmURL); err == nil && (len(parsed.Temps) > 0 || len(parsed.GPUs) > 0 || len(parsed.Fans) > 0) {
			return Reading{Temps: parsed.Temps, Fans: parsed.Fans, GPUs: parsed.GPUs}
		}
	}

	temps, acpiErr := readACPI()
	if acpiErr != nil && len(temps) == 0 {
		notes = append(notes, "run as admin")
	}
	hint := strings.Join(unique(notes), " · ")
	return Reading{Temps: temps, Hint: hint}
}

func unique(in []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

type wmiHardware struct {
	Identifier   string
	Name         string
	HardwareType string
}

type wmiSensor struct {
	Name       string
	SensorType string
	Parent     string
	Value      float64
}

func queryLHM() ([]SensorFromWMI, error) {
	var hw []wmiHardware
	_ = wmi.QueryNamespace("SELECT Identifier, Name, HardwareType FROM Hardware", &hw, `root\LibreHardwareMonitor`)
	byID := map[string]wmiHardware{}
	for _, h := range hw {
		byID[h.Identifier] = h
	}
	rows, err := queryLHMStrings(byID)
	if err == nil {
		return rows, nil
	}
	return queryLHMNumbers(byID)
}

func queryLHMStrings(byID map[string]wmiHardware) ([]SensorFromWMI, error) {
	var sensors []wmiSensor
	if err := wmi.QueryNamespace("SELECT Name, SensorType, Parent, Value FROM Sensor", &sensors, `root\LibreHardwareMonitor`); err != nil {
		return nil, err
	}
	rows := make([]SensorFromWMI, 0, len(sensors))
	for _, s := range sensors {
		parent := byID[s.Parent]
		name := parent.Name
		if name == "" {
			name = s.Parent
		}
		rows = append(rows, SensorFromWMI{
			Hardware:     name,
			HardwareType: parent.HardwareType,
			Name:         s.Name,
			SensorType:   s.SensorType,
			Value:        s.Value,
		})
	}
	return rows, nil
}

type wmiSensorNum struct {
	Name       string
	SensorType uint32
	Parent     string
	Value      float64
}

func queryLHMNumbers(byID map[string]wmiHardware) ([]SensorFromWMI, error) {
	var sensors []wmiSensorNum
	if err := wmi.QueryNamespace("SELECT Name, SensorType, Parent, Value FROM Sensor", &sensors, `root\LibreHardwareMonitor`); err != nil {
		return nil, err
	}
	rows := make([]SensorFromWMI, 0, len(sensors))
	for _, s := range sensors {
		parent := byID[s.Parent]
		name := parent.Name
		if name == "" {
			name = s.Parent
		}
		rows = append(rows, SensorFromWMI{
			Hardware:     name,
			HardwareType: parent.HardwareType,
			Name:         s.Name,
			SensorType:   strconv.FormatUint(uint64(s.SensorType), 10),
			Value:        s.Value,
		})
	}
	return rows, nil
}

func fetchLHM(ctx context.Context, url string) (LHMReading, error) {
	ctx, cancel := context.WithTimeout(ctx, 400*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return LHMReading{}, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return LHMReading{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return LHMReading{}, err
	}
	return ParseLHMJSON(body)
}

type acpiZone struct {
	InstanceName       string
	CurrentTemperature uint32
}

func readACPI() ([]metrics.Temp, error) {
	var zones []acpiZone
	err := wmi.QueryNamespace("SELECT InstanceName, CurrentTemperature FROM MSAcpi_ThermalZoneTemperature", &zones, `root\wmi`)
	if err != nil {
		return nil, err
	}
	var temps []metrics.Temp
	for _, z := range zones {
		c := float64(z.CurrentTemperature)/10 - 273.15
		if c < -20 || c > 150 {
			continue
		}
		name := z.InstanceName
		if name == "" {
			name = "ACPI thermal zone"
		}
		temps = append(temps, metrics.Temp{
			Name:    name,
			Celsius: c,
			Source:  "acpi",
			Kind:    "package",
		})
	}
	return temps, nil
}
