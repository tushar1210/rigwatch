package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNextBackoff(t *testing.T) {
	if nextBackoff(time.Second) != 2*time.Second {
		t.Fatalf("backoff = %s", nextBackoff(time.Second))
	}
	if nextBackoff(20*time.Second) != 30*time.Second {
		t.Fatalf("cap = %s", nextBackoff(20*time.Second))
	}
	if nextBackoff(backoffMax) != backoffMax {
		t.Fatal("max")
	}
}

func TestProfileFlag(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := []byte(`
profiles:
  gaming:
    panels: [cpu, gpu]
    thresholds:
      temp_amber: 80
      temp_red: 95
      gpu_util_warn: 99
`)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := New()
	cmd.SetArgs([]string{"--config", path, "--profile", "missing", "--json", "--demo"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected unknown profile")
	}

	cmd = New()
	cmd.SetArgs([]string{"--config", path, "--profile", "gaming", "--json", "--demo"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}
