package metrics

import "testing"

func TestFormatBytes(t *testing.T) {
	if got := FormatBytes(1536); got != "1.5 KB" {
		t.Fatalf("FormatBytes = %q", got)
	}
	if got := FormatBytes(32 * 1024 * 1024 * 1024); got != "32 GB" {
		t.Fatalf("FormatBytes = %q", got)
	}
}

func TestFormatUptime(t *testing.T) {
	if got := FormatUptime(4*86400 + 12*3600); got != "4d 12h" {
		t.Fatalf("uptime = %q", got)
	}
	if got := FormatUptime(45 * 60); got != "45m" {
		t.Fatalf("uptime = %q", got)
	}
}

func TestNormPCI(t *testing.T) {
	a := NormPCI("00000000:01:00.0")
	b := NormPCI("0000:01:00.0")
	if a != b {
		t.Fatalf("pci %q != %q", a, b)
	}
	if a != "0:01:00.0" {
		t.Fatalf("pci = %q", a)
	}
}

func TestHeadlineTemp(t *testing.T) {
	temps := []Temp{
		{Name: "Core 0", Celsius: 70, Kind: "core"},
		{Name: "Tctl", Celsius: 62, Kind: "package"},
		{Name: "nvme", Celsius: 40, Kind: "disk"},
	}
	got, ok := HeadlineTemp(temps)
	if !ok || got.Name != "Tctl" {
		t.Fatalf("headline = %+v ok=%v", got, ok)
	}
}

func TestDedupNotes(t *testing.T) {
	got := DedupNotes([]string{" run as admin ", "", "run as admin", "start LibreHardwareMonitor"})
	if len(got) != 2 || got[0] != "run as admin" {
		t.Fatalf("notes = %#v", got)
	}
}
