// Package config loads rigwatch settings from a YAML file and CLI flags.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the runtime configuration.
type Config struct {
	Interval   time.Duration      `yaml:"interval"`
	Listen     string             `yaml:"listen"`
	Token      string             `yaml:"token"`
	LHMURL     string             `yaml:"lhm_url"`
	StaleAfter time.Duration      `yaml:"stale_after"`
	Profile    string             `yaml:"profile"`
	Profiles   map[string]Profile `yaml:"profiles"`
	Panels     []string           `yaml:"-"`
	Threshold  Thresholds         `yaml:"thresholds"`
	Alerts     Alerts             `yaml:"alerts"`

	// baseThreshold is the file thresholds before a profile overlay.
	baseThreshold Thresholds
}

// Profile is a named panel layout and threshold overlay.
type Profile struct {
	Panels    []string
	Threshold Thresholds
}

// Thresholds control colour and alert edges.
// CPU and GPU temperature fields fall back to TempAmber and TempRed when zero.
// FanRPMWarn of zero does not apply an RPM ceiling.
type Thresholds struct {
	TempAmber    float64 `yaml:"temp_amber"`
	TempRed      float64 `yaml:"temp_red"`
	CPUTempAmber float64 `yaml:"cpu_temp_amber"`
	CPUTempRed   float64 `yaml:"cpu_temp_red"`
	GPUTempAmber float64 `yaml:"gpu_temp_amber"`
	GPUTempRed   float64 `yaml:"gpu_temp_red"`
	CPUWarn      float64 `yaml:"cpu_warn"`
	MemWarn      float64 `yaml:"mem_warn"`
	GPUUtilWarn  float64 `yaml:"gpu_util_warn"`
	DiskWarn     float64 `yaml:"disk_warn"`
	FanPctWarn   float64 `yaml:"fan_pct_warn"`
	FanRPMWarn   float64 `yaml:"fan_rpm_warn"`
}

// CPUTemp returns the CPU temperature pair, inheriting the shared limits.
func (t Thresholds) CPUTemp() (amber, red float64) {
	return t.pair(t.CPUTempAmber, t.CPUTempRed)
}

// GPUTemp returns the GPU temperature pair, inheriting the shared limits.
func (t Thresholds) GPUTemp() (amber, red float64) {
	return t.pair(t.GPUTempAmber, t.GPUTempRed)
}

func (t Thresholds) pair(amber, red float64) (float64, float64) {
	if amber <= 0 {
		amber = t.TempAmber
	}
	if red <= 0 {
		red = t.TempRed
	}
	return amber, red
}

// Alerts configures threshold and baseline notifications.
type Alerts struct {
	Enabled  bool
	Bell     bool
	Command  string
	Cooldown time.Duration
	Metrics  []string
	Baseline Baseline
	// History is how many notified alerts the TUI keeps in memory.
	// Nothing is written to disk. Zero disables the list.
	History int
}

// Allows reports whether metric should be evaluated.
// An empty Metrics list means every known metric.
func (a Alerts) Allows(metric string) bool {
	if !a.Enabled {
		return false
	}
	if len(a.Metrics) == 0 {
		return true
	}
	for _, m := range a.Metrics {
		if strings.EqualFold(strings.TrimSpace(m), metric) {
			return true
		}
	}
	return false
}

// Baseline is a rolling mean compared with a margin.
type Baseline struct {
	Enabled       bool
	Window        time.Duration
	Sustain       time.Duration
	MarginPercent float64
	MarginTemp    float64
	MarginRPM     float64
	MinSamples    int
}

// Default returns the built-in configuration.
func Default() Config {
	cfg := Config{
		Interval: time.Second,
		Listen:   ":7777",
		LHMURL:   "http://127.0.0.1:8085/data.json",
		Threshold: Thresholds{
			TempAmber:   75,
			TempRed:     85,
			CPUWarn:     90,
			MemWarn:     90,
			GPUUtilWarn: 95,
			DiskWarn:    90,
			FanPctWarn:  95,
		},
		Alerts: Alerts{
			Enabled:  true,
			Cooldown: 2 * time.Minute,
			History:  32,
			Baseline: Baseline{
				Enabled:       true,
				Window:        10 * time.Minute,
				Sustain:       time.Minute,
				MarginPercent: 20,
				MarginTemp:    10,
				MarginRPM:     400,
				MinSamples:    12,
			},
		},
	}
	cfg.baseThreshold = cfg.Threshold
	return cfg
}

// EffectiveStaleAfter is how long a host may go without a sample before
// the TUI marks it stale. An unset stale_after is three intervals, at least 3s.
func (c Config) EffectiveStaleAfter() time.Duration {
	if c.StaleAfter > 0 {
		return c.StaleAfter
	}
	d := 3 * c.Interval
	if d < 3*time.Second {
		return 3 * time.Second
	}
	return d
}

// WithProfile overlays a named profile onto the base thresholds.
// An empty name clears the overlay and shows every panel.
func (c Config) WithProfile(name string) (Config, error) {
	out := c
	if out.baseThreshold == (Thresholds{}) {
		out.baseThreshold = c.Threshold
	}
	out.Threshold = out.baseThreshold
	out.Panels = nil
	name = strings.TrimSpace(name)
	if name == "" {
		out.Profile = ""
		if err := out.Validate(); err != nil {
			return Config{}, err
		}
		return out, nil
	}
	spec, key, ok := out.findProfile(name)
	if !ok {
		return Config{}, fmt.Errorf("unknown profile %q", name)
	}
	out.Profile = key
	out.Threshold = mergeThreshold(out.baseThreshold, spec.Threshold)
	out.Panels = append([]string(nil), spec.Panels...)
	if err := out.Validate(); err != nil {
		return Config{}, err
	}
	return out, nil
}

func (c Config) findProfile(name string) (Profile, string, bool) {
	if p, ok := c.Profiles[name]; ok {
		return p, name, true
	}
	for k, p := range c.Profiles {
		if strings.EqualFold(k, name) {
			return p, k, true
		}
	}
	return Profile{}, "", false
}

type fileConfig struct {
	Interval   string                 `yaml:"interval"`
	Listen     string                 `yaml:"listen"`
	Token      string                 `yaml:"token"`
	LHMURL     string                 `yaml:"lhm_url"`
	StaleAfter string                 `yaml:"stale_after"`
	Profile    string                 `yaml:"profile"`
	Profiles   map[string]profileFile `yaml:"profiles"`
	Threshold  Thresholds             `yaml:"thresholds"`
	Alerts     *alertsFile            `yaml:"alerts"`
}

type profileFile struct {
	Panels    []string   `yaml:"panels"`
	Threshold Thresholds `yaml:"thresholds"`
}

type alertsFile struct {
	Enabled  *bool         `yaml:"enabled"`
	Bell     *bool         `yaml:"bell"`
	Command  *string       `yaml:"command"`
	Cooldown string        `yaml:"cooldown"`
	Metrics  []string      `yaml:"metrics"`
	History  *int          `yaml:"history"`
	Baseline *baselineFile `yaml:"baseline"`
}

type baselineFile struct {
	Enabled       *bool   `yaml:"enabled"`
	Window        string  `yaml:"window"`
	Sustain       string  `yaml:"sustain"`
	MarginPercent float64 `yaml:"margin_percent"`
	MarginTemp    float64 `yaml:"margin_temp"`
	MarginRPM     float64 `yaml:"margin_rpm"`
	MinSamples    int     `yaml:"min_samples"`
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
	if raw.StaleAfter != "" {
		d, err := time.ParseDuration(raw.StaleAfter)
		if err != nil {
			return cfg, fmt.Errorf("stale_after: %w", err)
		}
		cfg.StaleAfter = d
	}
	cfg.Threshold = mergeThreshold(cfg.Threshold, raw.Threshold)
	cfg.baseThreshold = cfg.Threshold
	alerts, err := applyAlerts(cfg.Alerts, raw.Alerts)
	if err != nil {
		return cfg, err
	}
	cfg.Alerts = alerts
	profiles, err := parseProfiles(raw.Profiles)
	if err != nil {
		return cfg, err
	}
	cfg.Profiles = profiles
	cfg.Profile = strings.TrimSpace(raw.Profile)
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	for name := range cfg.Profiles {
		if _, err := cfg.WithProfile(name); err != nil {
			return cfg, fmt.Errorf("profile %s: %w", name, err)
		}
	}
	if cfg.Profile != "" {
		cfg, err = cfg.WithProfile(cfg.Profile)
		if err != nil {
			return cfg, err
		}
	}
	return cfg, nil
}

func applyAlerts(base Alerts, raw *alertsFile) (Alerts, error) {
	if raw == nil {
		return base, nil
	}
	if raw.Enabled != nil {
		base.Enabled = *raw.Enabled
	}
	if raw.Bell != nil {
		base.Bell = *raw.Bell
	}
	if raw.Command != nil {
		base.Command = *raw.Command
	}
	if raw.Cooldown != "" {
		d, err := time.ParseDuration(raw.Cooldown)
		if err != nil {
			return base, fmt.Errorf("alerts.cooldown: %w", err)
		}
		base.Cooldown = d
	}
	if raw.Metrics != nil {
		base.Metrics = append([]string(nil), raw.Metrics...)
	}
	if raw.History != nil {
		base.History = *raw.History
	}
	if raw.Baseline != nil {
		b, err := applyBaseline(base.Baseline, raw.Baseline)
		if err != nil {
			return base, err
		}
		base.Baseline = b
	}
	return base, nil
}

func applyBaseline(base Baseline, raw *baselineFile) (Baseline, error) {
	if raw.Enabled != nil {
		base.Enabled = *raw.Enabled
	}
	if raw.Window != "" {
		d, err := time.ParseDuration(raw.Window)
		if err != nil {
			return base, fmt.Errorf("alerts.baseline.window: %w", err)
		}
		base.Window = d
	}
	if raw.Sustain != "" {
		d, err := time.ParseDuration(raw.Sustain)
		if err != nil {
			return base, fmt.Errorf("alerts.baseline.sustain: %w", err)
		}
		base.Sustain = d
	}
	if raw.MarginPercent != 0 {
		base.MarginPercent = raw.MarginPercent
	}
	if raw.MarginTemp != 0 {
		base.MarginTemp = raw.MarginTemp
	}
	if raw.MarginRPM != 0 {
		base.MarginRPM = raw.MarginRPM
	}
	if raw.MinSamples != 0 {
		base.MinSamples = raw.MinSamples
	}
	return base, nil
}

func mergeThreshold(base, over Thresholds) Thresholds {
	if over.TempAmber != 0 {
		base.TempAmber = over.TempAmber
	}
	if over.TempRed != 0 {
		base.TempRed = over.TempRed
	}
	if over.CPUTempAmber != 0 {
		base.CPUTempAmber = over.CPUTempAmber
	}
	if over.CPUTempRed != 0 {
		base.CPUTempRed = over.CPUTempRed
	}
	if over.GPUTempAmber != 0 {
		base.GPUTempAmber = over.GPUTempAmber
	}
	if over.GPUTempRed != 0 {
		base.GPUTempRed = over.GPUTempRed
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
	if over.DiskWarn != 0 {
		base.DiskWarn = over.DiskWarn
	}
	if over.FanPctWarn != 0 {
		base.FanPctWarn = over.FanPctWarn
	}
	if over.FanRPMWarn != 0 {
		base.FanRPMWarn = over.FanRPMWarn
	}
	return base
}

// Validate checks ranges that would make the UI nonsense.
func (c Config) Validate() error {
	if c.Interval < 200*time.Millisecond {
		return fmt.Errorf("interval %s is below 200ms", c.Interval)
	}
	if err := checkPair("temp", c.Threshold.TempAmber, c.Threshold.TempRed); err != nil {
		return err
	}
	amber, red := c.Threshold.CPUTemp()
	if err := checkPair("cpu temp", amber, red); err != nil {
		return err
	}
	amber, red = c.Threshold.GPUTemp()
	if err := checkPair("gpu temp", amber, red); err != nil {
		return err
	}
	if c.Alerts.Cooldown < 0 {
		return fmt.Errorf("alert cooldown must be >= 0")
	}
	if c.Alerts.History < 0 {
		return fmt.Errorf("alert history must be >= 0")
	}
	if c.StaleAfter < 0 {
		return fmt.Errorf("stale_after must be >= 0")
	}
	if c.StaleAfter > 0 && c.StaleAfter < time.Second {
		return fmt.Errorf("stale_after %s is below 1s", c.StaleAfter)
	}
	for _, m := range c.Alerts.Metrics {
		if !knownMetric(m) {
			return fmt.Errorf("unknown alert metric %q", m)
		}
	}
	b := c.Alerts.Baseline
	if c.Alerts.Enabled && b.Enabled {
		if b.Window <= 0 {
			return fmt.Errorf("baseline window must be positive")
		}
		if b.Sustain < 0 {
			return fmt.Errorf("baseline sustain must be >= 0")
		}
		if b.MinSamples < 1 {
			return fmt.Errorf("baseline min_samples must be >= 1")
		}
		if b.MarginPercent < 0 || b.MarginTemp < 0 || b.MarginRPM < 0 {
			return fmt.Errorf("baseline margins must be >= 0")
		}
	}
	return nil
}

func checkPair(name string, amber, red float64) error {
	if amber <= 0 || red <= 0 {
		return fmt.Errorf("%s thresholds must be positive", name)
	}
	if amber >= red {
		return fmt.Errorf("%s amber %.0f must be below red %.0f", name, amber, red)
	}
	return nil
}

func parseProfiles(raw map[string]profileFile) (map[string]Profile, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make(map[string]Profile, len(raw))
	for name, p := range raw {
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, fmt.Errorf("profile name is empty")
		}
		panels, err := normalizePanels(p.Panels)
		if err != nil {
			return nil, fmt.Errorf("profile %s: %w", name, err)
		}
		out[name] = Profile{Panels: panels, Threshold: p.Threshold}
	}
	return out, nil
}

func normalizePanels(in []string) ([]string, error) {
	if len(in) == 0 {
		return nil, nil
	}
	var out []string
	seen := map[string]bool{}
	for _, n := range in {
		c, ok := canonicalPanel(n)
		if !ok {
			return nil, fmt.Errorf("unknown panel %q", n)
		}
		if seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	return out, nil
}

func canonicalPanel(name string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "cpu":
		return "cpu", true
	case "gpu":
		return "gpu", true
	case "mem", "memory":
		return "mem", true
	case "temp", "temps", "temperature":
		return "temp", true
	case "fan", "fans":
		return "fan", true
	case "disk", "disks":
		return "disk", true
	case "net", "network":
		return "net", true
	default:
		return "", false
	}
}

func knownMetric(metric string) bool {
	switch strings.ToLower(strings.TrimSpace(metric)) {
	case "cpu", "memory", "gpu", "temp", "disk", "fan":
		return true
	default:
		return false
	}
}
