package alert

import (
	"testing"
	"time"

	"github.com/tushar1210/rigwatch/internal/config"
	"github.com/tushar1210/rigwatch/internal/metrics"
)

func TestEdgeTrigger(t *testing.T) {
	eng := New(config.Default().Threshold)
	cool := metrics.Snapshot{Temps: []metrics.Temp{{Name: "Tctl", Celsius: 60, Kind: "package"}}}
	if ev := eng.Evaluate("forge", cool); len(ev) != 0 {
		t.Fatalf("cool events = %+v", ev)
	}
	hot := metrics.Snapshot{Temps: []metrics.Temp{{Name: "Tctl", Celsius: 76, Kind: "package"}}}
	ev := eng.Evaluate("forge", hot)
	if len(ev) == 0 {
		t.Fatal("expected amber")
	}
	again := eng.Evaluate("forge", hot)
	if len(again) != 0 {
		t.Fatalf("repeat events = %+v", again)
	}
	hotter := metrics.Snapshot{Temps: []metrics.Temp{{Name: "Tctl", Celsius: 90, Kind: "package"}}}
	ev = eng.Evaluate("forge", hotter)
	found := false
	for _, e := range ev {
		if e.Level == "red" {
			found = true
		}
	}
	if !found {
		t.Fatalf("events = %+v", ev)
	}
	if ev := eng.Evaluate("forge", cool); len(ev) != 0 {
		t.Fatalf("clear should not alert: %+v", ev)
	}
	if ev := eng.Evaluate("forge", hot); len(ev) == 0 {
		t.Fatal("expected re-entry alert")
	}
}

func TestRunCommandEmpty(t *testing.T) {
	RunCommand("", Event{Key: "cpu"})
}

func TestDiskAndFanThresholds(t *testing.T) {
	cfg := quietConfig()
	cfg.Threshold.DiskWarn = 80
	cfg.Threshold.FanPctWarn = 70
	cfg.Threshold.FanRPMWarn = 2000
	eng := NewEngine(cfg)
	snap := metrics.Snapshot{
		Disks: []metrics.Disk{{Mount: "/", UsedPercent: 91}},
		Fans: []metrics.Fan{
			{Name: "cpu_fan", HasPercent: true, Percent: 80, HasRPM: true, RPM: 1500},
			{Name: "case", HasRPM: true, RPM: 2500},
		},
	}
	ev := eng.Evaluate("box", snap)
	got := map[string]string{}
	for _, e := range ev {
		got[e.Key] = e.Level
		if e.Kind != "threshold" || e.Host != "box" {
			t.Fatalf("event = %+v", e)
		}
	}
	for _, key := range []string{"disk /", "fan cpu_fan", "fan rpm case"} {
		if got[key] != "amber" {
			t.Fatalf("missing %s in %+v", key, got)
		}
	}
	if _, ok := got["fan rpm cpu_fan"]; ok {
		t.Fatalf("rpm under ceiling fired: %+v", got)
	}
	cool := metrics.Snapshot{Disks: []metrics.Disk{{Mount: "/", UsedPercent: 10}}}
	if ev := eng.Evaluate("box", cool); len(ev) != 0 {
		t.Fatalf("recover events = %+v", ev)
	}
	if n := len(eng.Active()); n != 0 {
		t.Fatalf("active after recover = %d", n)
	}
}

func TestCPUAndGPUTempThresholds(t *testing.T) {
	cfg := quietConfig()
	cfg.Threshold.TempAmber = 70
	cfg.Threshold.TempRed = 90
	cfg.Threshold.CPUTempAmber = 50
	cfg.Threshold.CPUTempRed = 60
	cfg.Threshold.GPUTempAmber = 80
	cfg.Threshold.GPUTempRed = 95
	eng := NewEngine(cfg)
	snap := metrics.Snapshot{
		Temps: []metrics.Temp{{Name: "Tctl", Celsius: 55, Kind: "package"}},
		GPUs:  []metrics.GPU{{Name: "rtx", Temp: metrics.OK(82)}},
	}
	ev := eng.Evaluate("box", snap)
	got := map[string]string{}
	for _, e := range ev {
		got[e.Key] = e.Level
	}
	if got["cpu temp"] != "amber" {
		t.Fatalf("cpu temp = %+v", got)
	}
	if got["gpu temp rtx"] != "amber" {
		t.Fatalf("gpu temp = %+v", got)
	}
}

func TestBaselineMean(t *testing.T) {
	cfg := quietConfig()
	cfg.Alerts.Baseline.Enabled = true
	cfg.Alerts.Baseline.Window = time.Hour
	cfg.Alerts.Baseline.Sustain = 0
	cfg.Alerts.Baseline.MarginPercent = 20
	cfg.Alerts.Baseline.MinSamples = 3
	cfg.Threshold.CPUWarn = 100
	eng, now := scripted(cfg)
	start := *now
	for i := 0; i < 3; i++ {
		*now = start.Add(time.Duration(i) * time.Second)
		if ev := eng.Evaluate("box", cpuSnap(10)); len(ev) != 0 {
			t.Fatalf("warmup %+v", ev)
		}
	}
	*now = start.Add(3 * time.Second)
	ev := eng.Evaluate("box", cpuSnap(40))
	if len(ev) != 1 {
		t.Fatalf("events = %+v", ev)
	}
	if ev[0].Kind != "baseline" || ev[0].Metric != "cpu" || ev[0].Value != 40 {
		t.Fatalf("event = %+v", ev[0])
	}
	if ev[0].Limit < 29.9 || ev[0].Limit > 30.1 {
		t.Fatalf("limit = %v", ev[0].Limit)
	}
	if len(eng.Active()) != 1 {
		t.Fatal("expected active baseline")
	}
}

func TestBaselineSustainAndRecover(t *testing.T) {
	cfg := quietConfig()
	cfg.Alerts.Baseline.Enabled = true
	cfg.Alerts.Baseline.Window = time.Hour
	cfg.Alerts.Baseline.Sustain = time.Minute
	cfg.Alerts.Baseline.MarginPercent = 20
	cfg.Alerts.Baseline.MinSamples = 3
	cfg.Threshold.CPUWarn = 100
	eng, now := scripted(cfg)
	start := *now
	for i := 0; i < 3; i++ {
		*now = start.Add(time.Duration(i) * time.Second)
		_ = eng.Evaluate("box", cpuSnap(10))
	}
	*now = start.Add(3 * time.Second)
	if ev := eng.Evaluate("box", cpuSnap(80)); len(ev) != 0 {
		t.Fatalf("too soon %+v", ev)
	}
	*now = start.Add(30 * time.Second)
	if ev := eng.Evaluate("box", cpuSnap(80)); len(ev) != 0 {
		t.Fatalf("sustain not met %+v", ev)
	}
	*now = start.Add(63 * time.Second)
	ev := eng.Evaluate("box", cpuSnap(80))
	if len(ev) != 1 || ev[0].Kind != "baseline" {
		t.Fatalf("events = %+v", ev)
	}
	if ev[0].Limit < 57 || ev[0].Limit > 59 {
		t.Fatalf("limit = %v", ev[0].Limit)
	}
	*now = start.Add(80 * time.Second)
	if ev := eng.Evaluate("box", cpuSnap(80)); len(ev) != 0 {
		t.Fatalf("repeat %+v", ev)
	}
	if len(eng.Active()) != 1 {
		t.Fatal("still active")
	}
	*now = start.Add(90 * time.Second)
	if ev := eng.Evaluate("box", cpuSnap(10)); len(ev) != 0 {
		t.Fatalf("recover event %+v", ev)
	}
	if len(eng.Active()) != 0 {
		t.Fatal("banner should clear")
	}
}

func TestCooldownSuppressesRefire(t *testing.T) {
	cfg := quietConfig()
	cfg.Alerts.Cooldown = 5 * time.Minute
	eng, now := scripted(cfg)
	start := *now
	hot := metrics.Snapshot{Temps: []metrics.Temp{{Name: "Tctl", Celsius: 80, Kind: "package"}}}
	cool := metrics.Snapshot{Temps: []metrics.Temp{{Name: "Tctl", Celsius: 50, Kind: "package"}}}
	*now = start
	if ev := eng.Evaluate("box", hot); len(ev) == 0 {
		t.Fatal("expected fire")
	}
	*now = start.Add(time.Second)
	if ev := eng.Evaluate("box", cool); len(ev) != 0 || len(eng.Active()) != 0 {
		t.Fatalf("recover ev=%v active=%v", ev, eng.Active())
	}
	*now = start.Add(2 * time.Second)
	if ev := eng.Evaluate("box", hot); len(ev) != 0 {
		t.Fatalf("inside cooldown %+v", ev)
	}
	if len(eng.Active()) != 1 {
		t.Fatal("banner should show while hot")
	}
	*now = start.Add(5*time.Minute + time.Second)
	if ev := eng.Evaluate("box", hot); len(ev) != 1 {
		t.Fatalf("after cooldown %+v", ev)
	}
}

func TestUpgradeDuringCooldown(t *testing.T) {
	cfg := quietConfig()
	cfg.Alerts.Cooldown = 5 * time.Minute
	eng, now := scripted(cfg)
	start := *now
	*now = start
	amber := metrics.Snapshot{Temps: []metrics.Temp{{Name: "Tctl", Celsius: 76, Kind: "package"}}}
	if ev := eng.Evaluate("box", amber); len(ev) != 1 || ev[0].Level != "amber" {
		t.Fatalf("amber %+v", ev)
	}
	*now = start.Add(time.Second)
	red := metrics.Snapshot{Temps: []metrics.Temp{{Name: "Tctl", Celsius: 90, Kind: "package"}}}
	ev := eng.Evaluate("box", red)
	if len(ev) != 1 || ev[0].Level != "red" {
		t.Fatalf("upgrade %+v", ev)
	}
}

func TestBaselineSuppressedWhileThresholdHot(t *testing.T) {
	cfg := quietConfig()
	cfg.Alerts.Baseline.Enabled = true
	cfg.Alerts.Baseline.Sustain = 0
	cfg.Alerts.Baseline.Window = time.Hour
	cfg.Alerts.Baseline.MinSamples = 3
	cfg.Alerts.Baseline.MarginPercent = 20
	cfg.Threshold.CPUWarn = 90
	eng, now := scripted(cfg)
	start := *now
	for i := 0; i < 3; i++ {
		*now = start.Add(time.Duration(i) * time.Second)
		_ = eng.Evaluate("box", cpuSnap(10))
	}
	*now = start.Add(4 * time.Second)
	ev := eng.Evaluate("box", cpuSnap(95))
	for _, e := range ev {
		if e.Kind == "baseline" {
			t.Fatalf("baseline fired beside threshold: %+v", ev)
		}
	}
	found := false
	for _, e := range ev {
		if e.Kind == "threshold" && e.Key == "cpu" {
			found = true
		}
	}
	if !found {
		t.Fatalf("events = %+v", ev)
	}
}

func TestDisabledAndMetricFilter(t *testing.T) {
	cfg := quietConfig()
	cfg.Alerts.Enabled = false
	eng := NewEngine(cfg)
	hot := metrics.Snapshot{Temps: []metrics.Temp{{Name: "Tctl", Celsius: 95, Kind: "package"}}}
	if ev := eng.Evaluate("box", hot); len(ev) != 0 || len(eng.Active()) != 0 {
		t.Fatalf("disabled %+v", ev)
	}

	cfg = quietConfig()
	cfg.Alerts.Metrics = []string{"disk"}
	cfg.Threshold.DiskWarn = 80
	eng = NewEngine(cfg)
	snap := metrics.Snapshot{
		CPU:   metrics.CPU{UsagePercent: 100},
		Disks: []metrics.Disk{{Mount: "/", UsedPercent: 95}},
		Temps: []metrics.Temp{{Name: "Tctl", Celsius: 95, Kind: "package"}},
	}
	ev := eng.Evaluate("box", snap)
	if len(ev) != 1 || ev[0].Metric != "disk" {
		t.Fatalf("filter %+v", ev)
	}
}

func quietConfig() config.Config {
	cfg := config.Default()
	cfg.Alerts.Baseline.Enabled = false
	cfg.Alerts.Cooldown = 0
	cfg.Alerts.Metrics = nil
	cfg.Threshold.CPUWarn = 100
	cfg.Threshold.MemWarn = 100
	cfg.Threshold.GPUUtilWarn = 100
	cfg.Threshold.DiskWarn = 100
	cfg.Threshold.FanPctWarn = 100
	cfg.Threshold.FanRPMWarn = 0
	return cfg
}

func scripted(cfg config.Config) (*Engine, *time.Time) {
	eng := NewEngine(cfg)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	eng.SetClock(func() time.Time { return now })
	return eng, &now
}

func cpuSnap(v float64) metrics.Snapshot {
	return metrics.Snapshot{CPU: metrics.CPU{UsagePercent: v}}
}
