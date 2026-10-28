package sensors

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tushar1210/rigwatch/internal/metrics"
)

// ReadSysfs reads hwmon and thermal-zone sensors under a sysfs root.
// root is "/sys" on Linux and a fixture directory in tests.
func ReadSysfs(root string) (temps []metrics.Temp, fans []metrics.Fan) {
	temps, fans = readHwmon(filepath.Join(root, "class", "hwmon"))
	temps = append(temps, readThermal(filepath.Join(root, "class", "thermal"))...)
	return temps, fans
}

func readHwmon(dir string) ([]metrics.Temp, []metrics.Fan) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil
	}
	var temps []metrics.Temp
	var fans []metrics.Fan
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		base := filepath.Join(dir, ent.Name())
		chip := strings.TrimSpace(readFile(filepath.Join(base, "name")))
		if chip == "" {
			chip = ent.Name()
		}
		for i := 1; i <= 32; i++ {
			n := strconv.Itoa(i)
			raw, ok := readFloat(filepath.Join(base, "temp"+n+"_input"))
			if !ok {
				continue
			}
			label := strings.TrimSpace(readFile(filepath.Join(base, "temp"+n+"_label")))
			name := label
			if name == "" {
				name = chip + " temp" + n
			} else if !strings.Contains(strings.ToLower(name), strings.ToLower(chip)) {
				name = chip + " " + label
			}
			temps = append(temps, metrics.Temp{
				Name:    name,
				Celsius: raw / 1000,
				Source:  "hwmon",
				Kind:    classifyTemp(chip, label),
			})
		}
		for i := 1; i <= 16; i++ {
			n := strconv.Itoa(i)
			rpm, ok := readFloat(filepath.Join(base, "fan"+n+"_input"))
			if !ok {
				continue
			}
			label := strings.TrimSpace(readFile(filepath.Join(base, "fan"+n+"_label")))
			name := label
			if name == "" {
				name = chip + " fan" + n
			} else if !strings.Contains(strings.ToLower(name), strings.ToLower(chip)) {
				name = chip + " " + label
			}
			fan := metrics.Fan{Name: name, RPM: rpm, HasRPM: true, Source: "hwmon"}
			if pwm, ok := readFloat(filepath.Join(base, "pwm"+n)); ok && pwm >= 0 {
				fan.Percent = pwm / 255 * 100
				fan.HasPercent = true
			}
			fans = append(fans, fan)
		}
	}
	return temps, fans
}

func readThermal(dir string) []metrics.Temp {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var temps []metrics.Temp
	for _, ent := range entries {
		if !ent.IsDir() || !strings.HasPrefix(ent.Name(), "thermal_zone") {
			continue
		}
		base := filepath.Join(dir, ent.Name())
		raw, ok := readFloat(filepath.Join(base, "temp"))
		if !ok {
			continue
		}
		kindName := strings.TrimSpace(readFile(filepath.Join(base, "type")))
		if kindName == "" {
			kindName = ent.Name()
		}
		temps = append(temps, metrics.Temp{
			Name:    kindName,
			Celsius: raw / 1000,
			Source:  "thermal",
			Kind:    classifyTemp(kindName, kindName),
		})
	}
	return temps
}

func classifyTemp(chip, label string) string {
	l := strings.ToLower(chip + " " + label)
	switch {
	case strings.Contains(l, "package") || strings.Contains(l, "tctl") || strings.Contains(l, "tdie") || strings.Contains(l, "x86_pkg"):
		return "package"
	case strings.Contains(l, "tccd") || strings.Contains(l, "core"):
		return "core"
	case strings.Contains(l, "nvme"):
		return "disk"
	case strings.Contains(l, "gpu") || strings.Contains(l, "junction") || strings.Contains(l, "edge") || strings.Contains(l, "amdgpu") || strings.Contains(l, "nvidia"):
		return "gpu"
	default:
		return "other"
	}
}

func readFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

func readFloat(path string) (float64, bool) {
	s := strings.TrimSpace(readFile(path))
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}
