package tui

import (
	"strings"
	"testing"
	"time"

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

func TestSparklinesAndProfiles(t *testing.T) {
	m := demoModel()
	view := m.View()
	for _, want := range []string{"cpu", "mem", "gpu", "temp"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing trend %q", want)
		}
	}
	if !strings.ContainsAny(view, sparkBlocks) {
		t.Fatal("missing sparkline")
	}
	cfg := config.Default()
	cfg.Panels = []string{"cpu", "temp"}
	narrow := New(Options{Config: cfg, Sources: []Source{{Name: "local"}}})
	narrow.width = 120
	narrow.height = 40
	updated, _ := narrow.Update(SampleMsg{Index: 0, Snap: collect.DemoSnapshot(2)})
	view = updated.View()
	if strings.Contains(view, "network") || strings.Contains(view, "memory") || strings.Contains(view, "fans") {
		t.Fatalf("hidden panel still visible:\n%s", view)
	}
	if !strings.Contains(view, "cpu") || !strings.Contains(view, "temp") {
		t.Fatalf("kept panels missing:\n%s", view)
	}
}

func TestStaleAndReconnect(t *testing.T) {
	m := demoModel()
	if strings.Contains(m.View(), "stale") {
		t.Fatal("fresh sample looks stale")
	}
	updated, _ := m.Update(tickMsg{now: time.Now().Add(10 * time.Second)})
	m = updated.(Model)
	view := m.View()
	if !strings.Contains(view, "stale") {
		t.Fatal("expected stale")
	}
	updated, _ = m.Update(StatusMsg{Index: 0, Err: "ssh closed", Reconnecting: true})
	m = updated.(Model)
	view = m.View()
	if !strings.Contains(view, "stale") || !strings.Contains(view, "reconnect") || !strings.Contains(view, "ssh closed") {
		t.Fatalf("reconnect state:\n%s", view)
	}
	updated, _ = m.Update(SampleMsg{Index: 0, Snap: collect.DemoSnapshot(5)})
	view = updated.View()
	if strings.Contains(view, "stale") || strings.Contains(view, "reconnect") {
		t.Fatalf("sample should clear stale:\n%s", view)
	}
}

func TestProcessGPUColumns(t *testing.T) {
	m := demoModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	m = updated.(Model)
	view := m.View()
	if !strings.Contains(view, "GPU") || !strings.Contains(view, "VRAM") || !strings.Contains(view, "rustc") {
		t.Fatalf("gpu columns missing:\n%s", view)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m = updated.(Model)
	if !strings.Contains(m.View(), "by gpu") {
		t.Fatal("gpu sort")
	}
	plain := New(Options{Config: config.Default(), Sources: []Source{{Name: "local"}}})
	plain.width = 100
	plain.height = 30
	snap := metrics.Snapshot{Processes: []metrics.Process{{PID: 1, Name: "sleep", CPUPercent: 1, MemPercent: 1, RSS: 4096}}}
	updated, _ = plain.Update(SampleMsg{Index: 0, Snap: snap})
	updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	view = updated.View()
	if strings.Contains(view, "GPU") || strings.Contains(view, "VRAM") {
		t.Fatalf("gpu columns should hide:\n%s", view)
	}
	if !strings.Contains(view, "sleep") {
		t.Fatal("plain process row")
	}
}

func TestAlertHistory(t *testing.T) {
	cfg := config.Default()
	cfg.Alerts.Baseline.Enabled = false
	m := New(Options{Config: cfg, Sources: []Source{{Name: "local"}}})
	m.width = 100
	m.height = 30
	hot := metrics.Snapshot{
		Host:  metrics.Host{Hostname: "forge"},
		Temps: []metrics.Temp{{Name: "Tctl", Celsius: 90, Kind: "package"}},
	}
	updated, _ := m.Update(SampleMsg{Index: 0, Snap: hot})
	m = updated.(Model)
	cool := hot
	cool.Temps[0].Celsius = 40
	updated, _ = m.Update(SampleMsg{Index: 0, Snap: cool})
	m = updated.(Model)
	if strings.Contains(m.View(), "alert") {
		t.Fatalf("banner should clear:\n%s", m.View())
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = updated.(Model)
	view := m.View()
	for _, want := range []string{"forge", "temp", "red"} {
		if !strings.Contains(view, want) {
			t.Fatalf("history missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "RTX") {
		t.Fatalf("history should cover the dashboard:\n%s", view)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if !strings.Contains(m.View(), "cpu") || m.showAlerts {
		t.Fatal("esc should return to the dashboard")
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
