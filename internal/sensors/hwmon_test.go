package sensors

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadSysfs(t *testing.T) {
	root := t.TempDir()
	hw := filepath.Join(root, "class", "hwmon", "hwmon0")
	if err := os.MkdirAll(hw, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(hw, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("name", "k10temp\n")
	write("temp1_input", "62500\n")
	write("temp1_label", "Tctl\n")
	write("temp2_input", "58000\n")
	write("temp2_label", "Tccd1\n")
	write("fan1_input", "1200\n")
	write("fan1_label", "cpu_fan\n")
	write("pwm1", "128\n")

	tz := filepath.Join(root, "class", "thermal", "thermal_zone0")
	if err := os.MkdirAll(tz, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tz, "type"), []byte("acpitz\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tz, "temp"), []byte("39000\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	temps, fans := ReadSysfs(root)
	if len(temps) != 3 {
		t.Fatalf("temps = %+v", temps)
	}
	if temps[0].Celsius != 62.5 || temps[0].Kind != "package" {
		t.Fatalf("tctl = %+v", temps[0])
	}
	if temps[1].Kind != "core" {
		t.Fatalf("tccd = %+v", temps[1])
	}
	if len(fans) != 1 || fans[0].RPM != 1200 || !fans[0].HasPercent {
		t.Fatalf("fans = %+v", fans)
	}
	if fans[0].Percent < 49 || fans[0].Percent > 51 {
		t.Fatalf("pwm percent = %v", fans[0].Percent)
	}
}
