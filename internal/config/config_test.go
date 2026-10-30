package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := []byte("interval: 2s\nlisten: \":7799\"\ntoken: secret\nthresholds:\n  temp_amber: 70\n  temp_red: 90\n  cpu_warn: 80\nalerts:\n  bell: true\n  command: echo hi\n")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Interval != 2*time.Second || cfg.Listen != ":7799" || cfg.Token != "secret" {
		t.Fatalf("cfg = %+v", cfg)
	}
	if cfg.Threshold.TempAmber != 70 || cfg.Threshold.TempRed != 90 || cfg.Threshold.CPUWarn != 80 {
		t.Fatalf("thresholds = %+v", cfg.Threshold)
	}
	if cfg.Threshold.MemWarn != 90 {
		t.Fatalf("mem warn default lost: %+v", cfg.Threshold)
	}
	if !cfg.Alerts.Bell || cfg.Alerts.Command != "echo hi" {
		t.Fatalf("alerts = %+v", cfg.Alerts)
	}
}

func TestLoadMissingExplicit(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
		t.Fatal("expected error")
	}
}

func TestRejectAmberAboveRed(t *testing.T) {
	cfg := Default()
	cfg.Threshold.TempAmber = 90
	cfg.Threshold.TempRed = 80
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error")
	}
}

func TestAlertDefaultsSurvivePartialYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := []byte("alerts:\n  bell: true\n")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Alerts.Enabled || !cfg.Alerts.Bell {
		t.Fatalf("alerts = %+v", cfg.Alerts)
	}
	if !cfg.Alerts.Baseline.Enabled || cfg.Alerts.Baseline.Window != 10*time.Minute {
		t.Fatalf("baseline = %+v", cfg.Alerts.Baseline)
	}
	if cfg.Alerts.Baseline.MarginPercent != 20 || cfg.Alerts.Baseline.MarginTemp != 10 {
		t.Fatalf("margins = %+v", cfg.Alerts.Baseline)
	}
	if cfg.Threshold.DiskWarn != 90 || cfg.Threshold.FanPctWarn != 95 {
		t.Fatalf("thresholds = %+v", cfg.Threshold)
	}
	if cfg.Alerts.Cooldown != 2*time.Minute {
		t.Fatalf("cooldown = %s", cfg.Alerts.Cooldown)
	}
}

func TestParseAlertConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := []byte(`
thresholds:
  temp_amber: 70
  temp_red: 90
  cpu_temp_amber: 65
  cpu_temp_red: 88
  gpu_temp_amber: 72
  gpu_temp_red: 92
  disk_warn: 80
  fan_pct_warn: 70
  fan_rpm_warn: 3200
alerts:
  enabled: true
  bell: true
  command: notify-send
  cooldown: 2m
  metrics: [cpu, temp, disk]
  baseline:
    enabled: true
    window: 5m
    sustain: 30s
    margin_percent: 15
    margin_temp: 8
    margin_rpm: 250
    min_samples: 4
`)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Threshold.CPUTempAmber != 65 || cfg.Threshold.GPUTempRed != 92 || cfg.Threshold.DiskWarn != 80 {
		t.Fatalf("thresholds = %+v", cfg.Threshold)
	}
	if cfg.Threshold.FanPctWarn != 70 || cfg.Threshold.FanRPMWarn != 3200 {
		t.Fatalf("fans = %+v", cfg.Threshold)
	}
	if cfg.Alerts.Cooldown != 2*time.Minute || cfg.Alerts.Command != "notify-send" {
		t.Fatalf("alerts = %+v", cfg.Alerts)
	}
	if len(cfg.Alerts.Metrics) != 3 || cfg.Alerts.Metrics[0] != "cpu" {
		t.Fatalf("metrics = %#v", cfg.Alerts.Metrics)
	}
	b := cfg.Alerts.Baseline
	if !b.Enabled || b.Window != 5*time.Minute || b.Sustain != 30*time.Second || b.MinSamples != 4 {
		t.Fatalf("baseline = %+v", b)
	}
	if b.MarginPercent != 15 || b.MarginTemp != 8 || b.MarginRPM != 250 {
		t.Fatalf("margins = %+v", b)
	}
	amber, red := cfg.Threshold.CPUTemp()
	if amber != 65 || red != 88 {
		t.Fatalf("cpu temp %v %v", amber, red)
	}
}

func TestRejectAlertConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("alerts:\n  cooldown: nope\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected cooldown error")
	}
	if err := os.WriteFile(path, []byte("alerts:\n  metrics: [cpu, nope]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected metric error")
	}
	cfg := Default()
	cfg.Threshold.CPUTempAmber = 90
	cfg.Threshold.CPUTempRed = 80
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected cpu temp pair error")
	}
}

func TestExampleConfig(t *testing.T) {
	cfg, err := Load(filepath.Join("..", "..", "rigwatch.example.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Alerts.Cooldown != 2*time.Minute || !cfg.Alerts.Baseline.Enabled {
		t.Fatalf("example alerts = %+v", cfg.Alerts)
	}
	if cfg.Threshold.DiskWarn != 90 || cfg.Threshold.TempAmber != 75 || cfg.Threshold.TempRed != 85 {
		t.Fatalf("example thresholds = %+v", cfg.Threshold)
	}
	if cfg.Alerts.Baseline.Sustain != time.Minute || cfg.Alerts.Baseline.MinSamples != 12 {
		t.Fatalf("example baseline = %+v", cfg.Alerts.Baseline)
	}
	if cfg.Alerts.History != 32 || cfg.Profile != "" || len(cfg.Profiles) != 2 {
		t.Fatalf("example profiles = %q %+v history %d", cfg.Profile, cfg.Profiles, cfg.Alerts.History)
	}
	if cfg.EffectiveStaleAfter() != 3*time.Second {
		t.Fatalf("stale = %s", cfg.EffectiveStaleAfter())
	}
}

func TestProfiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := []byte(`
interval: 2s
stale_after: 5s
profile: gaming
thresholds:
  temp_amber: 75
  temp_red: 85
  cpu_warn: 90
alerts:
  history: 8
profiles:
  gaming:
    panels: [memory, GPU, temps]
    thresholds:
      temp_red: 95
      gpu_util_warn: 99
  render:
    panels: [cpu, disk, network]
    thresholds:
      cpu_warn: 70
`)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Profile != "gaming" || cfg.StaleAfter != 5*time.Second || cfg.Alerts.History != 8 {
		t.Fatalf("cfg = %+v", cfg)
	}
	if cfg.Threshold.TempRed != 95 || cfg.Threshold.TempAmber != 75 || cfg.Threshold.GPUUtilWarn != 99 || cfg.Threshold.CPUWarn != 90 {
		t.Fatalf("thresholds = %+v", cfg.Threshold)
	}
	if len(cfg.Panels) != 3 || cfg.Panels[0] != "mem" || cfg.Panels[1] != "gpu" || cfg.Panels[2] != "temp" {
		t.Fatalf("panels = %#v", cfg.Panels)
	}
	cleared, err := cfg.WithProfile("")
	if err != nil {
		t.Fatal(err)
	}
	if cleared.Profile != "" || cleared.Threshold.TempRed != 85 || len(cleared.Panels) != 0 {
		t.Fatalf("cleared = %+v panels %#v", cleared.Threshold, cleared.Panels)
	}
	render, err := cfg.WithProfile("render")
	if err != nil {
		t.Fatal(err)
	}
	if render.Threshold.CPUWarn != 70 || render.Threshold.TempRed != 85 || render.Panels[2] != "net" {
		t.Fatalf("render = %+v %#v", render.Threshold, render.Panels)
	}
	if _, err := cfg.WithProfile("missing"); err == nil {
		t.Fatal("expected unknown profile")
	}
}

func TestRejectProfile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("profiles:\n  bad:\n    panels: [nope]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected panel error")
	}
	if err := os.WriteFile(path, []byte("profile: gone\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected missing profile")
	}
	if err := os.WriteFile(path, []byte("stale_after: 100ms\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected stale_after error")
	}
}
