package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

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
	if strings.Contains(view, "1 alert") || strings.Contains(ansi.Strip(view), "alert  ") {
		t.Fatalf("banner should clear:\n%s", view)
	}
}

func sampleProcs() []metrics.Process {
	return []metrics.Process{
		{PID: 40, Name: "alpha", CPUPercent: 10, MemPercent: 80, RSS: 100},
		{PID: 50, Name: "beta", CPUPercent: 40, MemPercent: 5, RSS: 5000, GPUPercent: 10, GPUUtilOK: true, GPUMem: 100, GPUMemOK: true},
		{PID: 60, Name: "gamma", CPUPercent: 30, MemPercent: 4, RSS: 4000, GPUPercent: 90, GPUUtilOK: true, GPUMem: 50, GPUMemOK: true},
		{PID: 70, Name: "delta", CPUPercent: 20, MemPercent: 3, RSS: 9000, GPUMemPctOK: true, GPUMemPercent: 70},
	}
}

func procModel(rows []metrics.Process, kind string, demo bool, kill func(int32, int) error) Model {
	m := New(Options{
		Config:  config.Default(),
		Demo:    demo,
		Sources: []Source{{Name: "box", Kind: kind}},
		Kill:    kill,
	})
	m.width = 100
	m.height = 36
	updated, _ := m.Update(SampleMsg{Index: 0, Snap: metrics.Snapshot{
		Host:      metrics.Host{Hostname: "box"},
		Processes: rows,
	}})
	m = updated.(Model)
	updated, _ = m.Update(keyRune('p'))
	return updated.(Model)
}

func keyRune(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func press(m Model, r rune) Model {
	updated, _ := m.Update(keyRune(r))
	return updated.(Model)
}

func plain(m Model) string {
	return ansi.Strip(m.View())
}

func lineWith(view, name string) string {
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, name) {
			return line
		}
	}
	return ""
}

func TestProcessSortKeysAndTopThree(t *testing.T) {
	m := procModel(sampleProcs(), "local", false, nil)
	view := plain(m)
	if !strings.Contains(m.View(), styleAccent().Render("1")) || !strings.Contains(m.View(), styleText().Bold(true).Render("2")) || !strings.Contains(m.View(), styleValue(colorAmber).Render("3")) {
		t.Fatal("top 3 ranks are not colored")
	}
	if !strings.Contains(view, "by cpu") {
		t.Fatalf("cpu label:\n%s", view)
	}
	if !strings.Contains(lineWith(view, "beta"), " 1 ") || !strings.Contains(lineWith(view, "gamma"), " 2 ") || !strings.Contains(lineWith(view, "delta"), " 3 ") {
		t.Fatalf("cpu ranks:\n%s", view)
	}
	if strings.Contains(lineWith(view, "alpha"), " 4 ") {
		t.Fatalf("fourth row should not be ranked:\n%s", view)
	}

	m = press(m, 'm')
	view = plain(m)
	if !strings.Contains(view, "by memory") {
		t.Fatalf("memory label:\n%s", view)
	}
	if !strings.Contains(lineWith(view, "delta"), " 1 ") || !strings.Contains(lineWith(view, "beta"), " 2 ") {
		t.Fatalf("rss ranks:\n%s", view)
	}
	if strings.Contains(lineWith(view, "alpha"), " 1 ") {
		t.Fatalf("low rss ranked first:\n%s", view)
	}

	m = press(m, 'g')
	view = plain(m)
	if !strings.Contains(view, "by gpu") || !strings.Contains(lineWith(view, "gamma"), " 1 ") || !strings.Contains(lineWith(view, "beta"), " 2 ") {
		t.Fatalf("gpu order:\n%s", view)
	}

	m = press(m, 'G')
	view = plain(m)
	if !strings.Contains(view, "by vram") || !strings.Contains(lineWith(view, "beta"), " 1 ") {
		t.Fatalf("vram order:\n%s", view)
	}
	if strings.Contains(view, "VRAM") == false {
		t.Fatalf("missing vram column:\n%s", view)
	}
}

func TestProcessKillConfirmAndPermission(t *testing.T) {
	var gotPID int32
	var gotSig int
	calls := 0
	m := procModel(sampleProcs(), "local", false, func(pid int32, sig int) error {
		calls++
		gotPID = pid
		gotSig = sig
		return nil
	})
	m = press(m, 'y')
	if calls != 0 {
		t.Fatal("y without confirm killed")
	}
	m = press(m, 'x')
	if !strings.Contains(plain(m), "TERM 50 beta") || !strings.Contains(plain(m), "9 kill") {
		t.Fatalf("confirm:\n%s", plain(m))
	}
	m = press(m, 'y')
	if calls != 1 || gotPID != 50 || gotSig != sigTerm {
		t.Fatalf("term pid=%d sig=%d calls=%d", gotPID, gotSig, calls)
	}
	m = press(m, 'x')
	m = press(m, 'n')
	if calls != 1 || strings.Contains(plain(m), "TERM") {
		t.Fatalf("cancel still armed:\n%s", plain(m))
	}

	m = press(m, 'x')
	m = press(m, '9')
	if !strings.Contains(plain(m), "KILL 50 beta") {
		t.Fatalf("sigkill prompt:\n%s", plain(m))
	}
	m = press(m, 'y')
	if calls != 2 || gotPID != 50 || gotSig != sigKill {
		t.Fatalf("kill pid=%d sig=%d calls=%d", gotPID, gotSig, calls)
	}
	if !strings.Contains(plain(m), "sent KILL 50") {
		t.Fatalf("sent note:\n%s", plain(m))
	}
	m = press(press(m, 'j'), 'x')
	if !strings.Contains(plain(m), "TERM 60 gamma") {
		t.Fatalf("selected row:\n%s", plain(m))
	}
	cancelled := plain(press(m, 'n'))
	if calls != 2 || strings.Contains(cancelled, "TERM") {
		t.Fatalf("cancel after selection:\n%s", cancelled)
	}

	m = procModel(sampleProcs(), "local", false, func(int32, int) error {
		return errors.New("operation not permitted")
	})
	m = press(press(m, 'x'), 'y')
	if !strings.Contains(plain(m), "50 access denied") {
		t.Fatalf("permission:\n%s", plain(m))
	}

	called := false
	m = procModel(sampleProcs(), "ws", false, func(int32, int) error {
		called = true
		return nil
	})
	m = press(m, 'x')
	if called || !strings.Contains(plain(m), "local only") {
		t.Fatalf("remote:\n%s", plain(m))
	}

	m = procModel(sampleProcs(), "local", true, func(int32, int) error {
		called = true
		return nil
	})
	m = press(m, 'x')
	if called || !strings.Contains(plain(m), "demo") {
		t.Fatalf("demo:\n%s", plain(m))
	}
}

func TestRefuseInitPID(t *testing.T) {
	called := false
	rows := []metrics.Process{{PID: 1, Name: "init", CPUPercent: 1, RSS: 10}}
	m := procModel(rows, "local", false, func(int32, int) error {
		called = true
		return nil
	})
	m = press(m, 'x')
	if called || !strings.Contains(plain(m), "refused") {
		t.Fatalf("pid 1:\n%s", plain(m))
	}
}

func TestKillEscStaysOnProcesses(t *testing.T) {
	m := procModel(sampleProcs(), "local", false, func(int32, int) error { return nil })
	m = press(m, 'x')
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if !m.showProc || m.killAsk != nil {
		t.Fatalf("esc should cancel confirm, showProc=%v ask=%v", m.showProc, m.killAsk)
	}
}

func TestNoGPUColumnWithoutStats(t *testing.T) {
	rows := []metrics.Process{{PID: 40, Name: "alpha", CPUPercent: 1, RSS: 10}}
	m := procModel(rows, "local", false, nil)
	if strings.Contains(plain(m), "VRAM") {
		t.Fatalf("empty gpu column:\n%s", plain(m))
	}
}

func TestHostShutdownConfirm(t *testing.T) {
	calls := 0
	m := New(Options{
		Config:  config.Default(),
		Sources: []Source{{Name: "forge", Kind: "ssh"}},
		Shutdown: func(index int) error {
			calls++
			if index != 0 {
				t.Fatalf("index %d", index)
			}
			return nil
		},
	})
	m.width = 80
	m.height = 24
	m = press(m, 'y')
	if calls != 0 {
		t.Fatal("y without the switch shut the host down")
	}
	m = press(m, '!')
	view := plain(m)
	if calls != 0 || !strings.Contains(view, "SHUT DOWN forge") {
		t.Fatalf("prompt:\n%s", view)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.powerAsk || strings.Contains(plain(m), "SHUT DOWN") {
		t.Fatalf("esc should cancel:\n%s", plain(m))
	}
	m = press(m, '!')
	m = press(m, 'n')
	if calls != 0 || m.powerAsk {
		t.Fatal("n should cancel")
	}
	m = press(m, '!')
	updated, cmd := m.Update(keyRune('y'))
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("confirm did not start")
	}
	msg := cmd()
	updated, _ = m.Update(msg)
	m = updated.(Model)
	if calls != 1 || !strings.Contains(plain(m), "shutting down") {
		t.Fatalf("calls=%d view:\n%s", calls, plain(m))
	}
}

func TestHostShutdownDemoRefuses(t *testing.T) {
	m := New(Options{
		Config:  config.Default(),
		Demo:    true,
		Sources: []Source{{Name: "forge", Kind: "local"}},
		Shutdown: func(int) error {
			t.Fatal("demo shutdown")
			return nil
		},
	})
	m.width = 80
	m.height = 20
	m = press(m, '!')
	if m.powerAsk || !strings.Contains(plain(m), "demo") {
		t.Fatalf("demo:\n%s", plain(m))
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
	plainSnap := New(Options{Config: config.Default(), Sources: []Source{{Name: "local"}}})
	plainSnap.width = 100
	plainSnap.height = 30
	snap := metrics.Snapshot{Processes: []metrics.Process{{PID: 1, Name: "sleep", CPUPercent: 1, MemPercent: 1, RSS: 4096}}}
	updated, _ = plainSnap.Update(SampleMsg{Index: 0, Snap: snap})
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
	if strings.Contains(m.View(), "1 alert") || strings.Contains(ansi.Strip(m.View()), "alert  ") {
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
