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
