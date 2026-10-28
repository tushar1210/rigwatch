// Package config loads rigwatch settings from a YAML file and CLI flags.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the runtime configuration.
type Config struct {
	Interval  time.Duration `yaml:"interval"`
	Listen    string        `yaml:"listen"`
	Token     string        `yaml:"token"`
	LHMURL    string        `yaml:"lhm_url"`
	Threshold Thresholds    `yaml:"thresholds"`
	Alerts    Alerts        `yaml:"alerts"`
}

// Thresholds control colour and alert edges.
type Thresholds struct {
	TempAmber   float64 `yaml:"temp_amber"`
	TempRed     float64 `yaml:"temp_red"`
	CPUWarn     float64 `yaml:"cpu_warn"`
	MemWarn     float64 `yaml:"mem_warn"`
	GPUUtilWarn float64 `yaml:"gpu_util_warn"`
}

// Alerts optionally rings the terminal bell or runs a command on a new breach.
type Alerts struct {
	Bell    bool   `yaml:"bell"`
	Command string `yaml:"command"`
}

// Default returns the built-in configuration.
func Default() Config {
	return Config{
		Interval: time.Second,
		Listen:   ":7777",
		LHMURL:   "http://127.0.0.1:8085/data.json",
		Threshold: Thresholds{
			TempAmber:   75,
			TempRed:     85,
			CPUWarn:     90,
			MemWarn:     90,
			GPUUtilWarn: 95,
		},
	}
}

type fileConfig struct {
	Interval  string     `yaml:"interval"`
	Listen    string     `yaml:"listen"`
	Token     string     `yaml:"token"`
	LHMURL    string     `yaml:"lhm_url"`
	Threshold Thresholds `yaml:"thresholds"`
	Alerts    Alerts     `yaml:"alerts"`
}

// Load reads path when it is non-empty. Otherwise it reads
// $RIGWATCH_CONFIG or the user config file when that file exists.
// A missing default file is not an error.
func Load(path string) (Config, error) {
	cfg := Default()
	if path == "" {
		if env := os.Getenv("RIGWATCH_CONFIG"); env != "" {
			path = env
		}
	}
	explicit := path != ""
	if path == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return cfg, nil
		}
		path = filepath.Join(dir, "rigwatch", "config.yaml")
		if _, err := os.Stat(path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return cfg, nil
			}
			return cfg, err
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if !explicit && errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("read config: %w", err)
	}
	var raw fileConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return cfg, fmt.Errorf("parse config: %w", err)
	}
	if raw.Interval != "" {
		d, err := time.ParseDuration(raw.Interval)
		if err != nil {
			return cfg, fmt.Errorf("interval: %w", err)
		}
		cfg.Interval = d
	}
	if raw.Listen != "" {
		cfg.Listen = raw.Listen
	}
	if raw.Token != "" {
		cfg.Token = raw.Token
	}
	if raw.LHMURL != "" {
		cfg.LHMURL = raw.LHMURL
	}
	cfg.Threshold = mergeThreshold(cfg.Threshold, raw.Threshold)
	cfg.Alerts = raw.Alerts
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func mergeThreshold(base, over Thresholds) Thresholds {
	if over.TempAmber != 0 {
		base.TempAmber = over.TempAmber
	}
	if over.TempRed != 0 {
		base.TempRed = over.TempRed
	}
	if over.CPUWarn != 0 {
		base.CPUWarn = over.CPUWarn
	}
	if over.MemWarn != 0 {
		base.MemWarn = over.MemWarn
	}
	if over.GPUUtilWarn != 0 {
		base.GPUUtilWarn = over.GPUUtilWarn
	}
	return base
}

// Validate checks ranges that would make the UI nonsense.
func (c Config) Validate() error {
	if c.Interval < 200*time.Millisecond {
		return fmt.Errorf("interval %s is below 200ms", c.Interval)
	}
	if c.Threshold.TempAmber <= 0 || c.Threshold.TempRed <= 0 {
		return fmt.Errorf("temperature thresholds must be positive")
	}
	if c.Threshold.TempAmber >= c.Threshold.TempRed {
		return fmt.Errorf("temp amber %.0f must be below temp red %.0f", c.Threshold.TempAmber, c.Threshold.TempRed)
	}
	return nil
}
