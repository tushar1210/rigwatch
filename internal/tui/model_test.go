package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tushar1210/rigwatch/internal/collect"
	"github.com/tushar1210/rigwatch/internal/config"
	"github.com/tushar1210/rigwatch/internal/metrics"
)

func demoModel() Model {
	m := New(Options{
		Config: config.Default(),
		Demo:   true,
		Sources: []Source{
			{Name: "local", Kind: "local"},
			{Name: "other", Kind: "ws"},
		},
	})
	m.width = 120
	m.height = 42
	updated, _ := m.Update(SampleMsg{Index: 0, Snap: collect.DemoSnapshot(4)})
	updated, _ = updated.Update(SampleMsg{Index: 1, Snap: metrics.Snapshot{
		Host: metrics.Host{Hostname: "atlas", OS: "windows", Platform: "windows 11", Arch: "amd64", UptimeSeconds: 3600},
		GPUs: []metrics.GPU{{
			Name:   "Radeon RX 6800",
			Vendor: "amd",
			Temp:   metrics.Missing("start LibreHardwareMonitor"),
			Util:   metrics.Missing("start LibreHardwareMonitor"),
		}},
		Notes: []string{"start LibreHardwareMonitor"},
	}})
	return updated.(Model)
}

func TestDashboardView(t *testing.T) {
	view := demoModel().View()
	for _, want := range []string{"rigwatch", "forge", "cpu", "memory", "gpu", "fans", "temps", "RTX 4090", "Tctl", "eth0"} {
		if !strings.Contains(strings.ToLower(view), strings.ToLower(want)) {
			t.Fatalf("missing %q", want)
		}
	}
}

func TestTabAndProcesses(t *testing.T) {
	m := demoModel()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	if !strings.Contains(m.View(), "atlas") {
		t.Fatal("tab did not switch host")
	}
	if !strings.Contains(m.View(), "LibreHardwareMonitor") {
		t.Fatal("missing sensor hint")
	}
	back, _ := m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = back.(Model)
	proc, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	m = proc.(Model)
	if !m.showProc || !strings.Contains(m.View(), "rustc") {
		t.Fatal("process view")
	}
	help, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = help.(Model)
	if !strings.Contains(m.View(), "thresholds") {
		t.Fatal("help view")
	}
}

func TestFooterSurvivesResize(t *testing.T) {
	m := demoModel()
	if !strings.Contains(m.View(), "quit") {
		t.Fatal("footer missing before resize")
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 42})
	view := updated.View()
	if !strings.Contains(view, "quit") || !strings.Contains(view, "processes") {
		t.Fatalf("footer missing after resize:\n%s", view)
	}
}

func TestAlertBannerClears(t *testing.T) {
	cfg := config.Default()
	cfg.Alerts.Baseline.Enabled = false
	m := New(Options{Config: cfg, Sources: []Source{{Name: "local"}}})
	m.width = 100
	m.height = 40
	hot := metrics.Snapshot{Temps: []metrics.Temp{{Name: "Tctl", Celsius: 90, Kind: "package"}}}
	updated, _ := m.Update(SampleMsg{Index: 0, Snap: hot})
	view := updated.View()
	if !strings.Contains(view, "alert") || !strings.Contains(view, "90") {
		t.Fatalf("banner missing:\n%s", view)
	}
	if !strings.Contains(view, "1 alert") {
		t.Fatalf("status missing:\n%s", view)
	}
	cool := hot
	cool.Temps[0].Celsius = 50
	updated, _ = updated.Update(SampleMsg{Index: 0, Snap: cool})
	view = updated.View()
	if strings.Contains(view, "alert") {
		t.Fatalf("banner should clear:\n%s", view)
	}
}

func TestHotTempIsMarked(t *testing.T) {
	m := New(Options{Config: config.Default(), Sources: []Source{{Name: "local"}}})
	m.width = 100
	m.height = 40
	snap := collect.DemoSnapshot(1)
	snap.Temps[0].Celsius = 90
	updated, _ := m.Update(SampleMsg{Index: 0, Snap: snap})
	view := updated.View()
	if !strings.Contains(view, "90") {
		t.Fatal("expected hot temperature in the view")
	}
}
