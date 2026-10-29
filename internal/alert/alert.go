// Package alert tracks threshold breaches and optionally runs a command.
package alert

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/tushar1210/rigwatch/internal/config"
	"github.com/tushar1210/rigwatch/internal/metrics"
)

const (
	kindThreshold = "threshold"
	kindBaseline  = "baseline"
	unitC         = "C"
	unitPct       = "%"
	unitRPM       = "rpm"
)

// Event is a threshold or baseline breach that has just been notified.
type Event struct {
	Host   string    `json:"host"`
	Key    string    `json:"key"`
	Value  float64   `json:"value"`
	Limit  float64   `json:"limit"`
	Level  string    `json:"level"`
	Kind   string    `json:"kind"`
	Metric string    `json:"metric"`
	Unit   string    `json:"unit,omitempty"`
	At     time.Time `json:"at"`
}

// Summary is a short label for the banner and status line.
func (ev Event) Summary() string {
	switch ev.Unit {
	case unitC:
		return fmt.Sprintf("%s %.0f°C > %.0f°C", ev.Key, ev.Value, ev.Limit)
	case unitRPM:
		return fmt.Sprintf("%s %.0f rpm > %.0f", ev.Key, ev.Value, ev.Limit)
	default:
		return fmt.Sprintf("%s %.0f%% > %.0f%%", ev.Key, ev.Value, ev.Limit)
	}
}

// Engine remembers breach state so alerts edge-trigger, cool down, and recover.
type Engine struct {
	cfg      config.Config
	now      func() time.Time
	latches  map[string]*latch
	lastFire map[string]time.Time
	series   map[string]*series
}

type latch struct {
	ev        Event
	overSince time.Time
	fired     string
	visible   bool
}

// New returns a threshold-only engine. Baseline and cooldown stay off so
// existing edge-trigger behaviour is unchanged.
func New(th config.Thresholds) *Engine {
	cfg := config.Default()
	cfg.Threshold = th
	cfg.Alerts.Baseline.Enabled = false
	cfg.Alerts.Cooldown = 0
	cfg.Alerts.Enabled = true
	return newEngine(cfg)
}

// NewEngine returns an engine for the full config, including baseline alerts.
func NewEngine(cfg config.Config) *Engine {
	return newEngine(cfg)
}

func newEngine(cfg config.Config) *Engine {
	if cfg.Alerts.Baseline.Window <= 0 {
		cfg.Alerts.Baseline.Window = 10 * time.Minute
	}
	if cfg.Alerts.Baseline.MinSamples <= 0 {
		cfg.Alerts.Baseline.MinSamples = 12
	}
	return &Engine{
		cfg:      cfg,
		latches:  map[string]*latch{},
		lastFire: map[string]time.Time{},
		series:   map[string]*series{},
	}
}

// SetClock overrides the clock used for sustain and cooldown. Tests use it.
func (e *Engine) SetClock(now func() time.Time) {
	e.now = now
}

func (e *Engine) clock() time.Time {
	if e.now != nil {
		return e.now()
	}
	return time.Now()
}

// Active returns breaches currently shown. Recovered keys are omitted.
func (e *Engine) Active() []Event {
	out := make([]Event, 0, len(e.latches))
	for _, lat := range e.latches {
		if lat != nil && lat.visible {
			out = append(out, lat.ev)
		}
	}
	sortEvents(out)
	return out
}

// Evaluate returns alerts that should notify now.
// The same key does not notify again until it recovers and the cooldown
// has elapsed. A more severe level still notifies. Baseline alerts also
// wait until the margin has held for the sustain duration.
func (e *Engine) Evaluate(host string, snap metrics.Snapshot) []Event {
	if !e.cfg.Alerts.Enabled {
		e.latches = map[string]*latch{}
		return nil
	}
	now := e.clock()
	evs := e.thresholds(host, snap, now)
	evs = append(evs, e.baselines(host, snap, now)...)
	sortEvents(evs)
	return evs
}

func sortEvents(evs []Event) {
	sort.Slice(evs, func(i, j int) bool {
		if evs[i].Key == evs[j].Key {
			return evs[i].Kind < evs[j].Kind
		}
		return evs[i].Key < evs[j].Key
	})
}

func (e *Engine) thresholds(host string, snap metrics.Snapshot, now time.Time) []Event {
	seen := map[string]bool{}
	var out []Event
	emit := func(key, metric, unit string, value, amber, red float64) {
		if !e.cfg.Alerts.Allows(metric) {
			return
		}
		lid := latchID(host, kindThreshold, key)
		seen[lid] = true
		lv := classify(value, amber, red)
		if ev, ok := e.bump(host, kindThreshold, key, metric, unit, lv.name, value, lv.limit, 0, now); ok {
			out = append(out, ev)
		}
	}

	var headName string
	if t, ok := metrics.HeadlineTemp(snap.Temps); ok {
		headName = t.Name
		amber, red := e.cfg.Threshold.CPUTemp()
		emit("cpu temp", "temp", unitC, t.Celsius, amber, red)
	}
	for _, t := range snap.Temps {
		if headName != "" && t.Name == headName {
			continue
		}
		amber, red := e.tempLimits(t)
		emit("temp "+t.Name, e.tempMetric(t), unitC, t.Celsius, amber, red)
	}
	emit("cpu", "cpu", unitPct, snap.CPU.UsagePercent, e.cfg.Threshold.CPUWarn, e.cfg.Threshold.CPUWarn)
	emit("memory", "memory", unitPct, snap.Memory.UsedPercent, e.cfg.Threshold.MemWarn, e.cfg.Threshold.MemWarn)
	for _, g := range snap.GPUs {
		ga, gr := e.cfg.Threshold.GPUTemp()
		if g.Temp.Available {
			emit("gpu temp "+g.Name, "gpu", unitC, g.Temp.Value, ga, gr)
		}
		if g.Util.Available {
			emit("gpu util "+g.Name, "gpu", unitPct, g.Util.Value, e.cfg.Threshold.GPUUtilWarn, e.cfg.Threshold.GPUUtilWarn)
		}
	}
	for _, d := range snap.Disks {
		name := d.Mount
		if name == "" {
			name = d.Name
		}
		emit("disk "+name, "disk", unitPct, d.UsedPercent, e.cfg.Threshold.DiskWarn, 0)
	}
	for _, f := range snap.Fans {
		if f.HasPercent {
			emit("fan "+f.Name, "fan", unitPct, f.Percent, e.cfg.Threshold.FanPctWarn, 0)
		}
		if f.HasRPM {
			emit("fan rpm "+f.Name, "fan", unitRPM, f.RPM, e.cfg.Threshold.FanRPMWarn, 0)
		}
	}
	e.sweep(host, kindThreshold, seen)
	return out
}

func (e *Engine) baselines(host string, snap metrics.Snapshot, now time.Time) []Event {
	if !e.cfg.Alerts.Baseline.Enabled {
		e.sweep(host, kindBaseline, nil)
		return nil
	}
	seen := map[string]bool{}
	var out []Event
	one := func(key, metric, unit string, value float64, ok bool) {
		if !ok || !e.cfg.Alerts.Allows(metric) {
			return
		}
		seen[latchID(host, kindBaseline, key)] = true
		mean, _, over := e.observe(host+"\x00"+key, now, value, e.margin(unit))
		level := ""
		limit := 0.0
		if over && !e.thresholdHot(host, key) {
			level = "amber"
			limit = mean + e.margin(unit)
		}
		if ev, fired := e.bump(host, kindBaseline, key, metric, unit, level, value, limit, e.cfg.Alerts.Baseline.Sustain, now); fired {
			out = append(out, ev)
		}
	}

	var headName string
	if t, ok := metrics.HeadlineTemp(snap.Temps); ok {
		headName = t.Name
		one("cpu temp", "temp", unitC, t.Celsius, true)
	}
	for _, t := range snap.Temps {
		if headName != "" && t.Name == headName {
			continue
		}
		one("temp "+t.Name, e.tempMetric(t), unitC, t.Celsius, true)
	}
	one("cpu", "cpu", unitPct, snap.CPU.UsagePercent, true)
	one("memory", "memory", unitPct, snap.Memory.UsedPercent, true)
	for _, g := range snap.GPUs {
		if g.Util.Available {
			one("gpu util "+g.Name, "gpu", unitPct, g.Util.Value, true)
		}
		if g.Temp.Available {
			one("gpu temp "+g.Name, "gpu", unitC, g.Temp.Value, true)
		}
	}
	for _, f := range snap.Fans {
		if f.HasPercent {
			one("fan "+f.Name, "fan", unitPct, f.Percent, true)
		}
		if f.HasRPM {
			one("fan rpm "+f.Name, "fan", unitRPM, f.RPM, e.cfg.Alerts.Baseline.MarginRPM > 0)
		}
	}
	e.sweep(host, kindBaseline, seen)
	return out
}

func (e *Engine) observe(sid string, now time.Time, v, margin float64) (mean float64, n int, over bool) {
	s := e.series[sid]
	if s == nil {
		s = &series{}
		e.series[sid] = s
	}
	window := e.cfg.Alerts.Baseline.Window
	mean, n = s.Mean(now, window)
	s.Add(now, v, window)
	if n < e.cfg.Alerts.Baseline.MinSamples {
		return mean, n, false
	}
	return mean, n, v > mean+margin
}

func (e *Engine) margin(unit string) float64 {
	switch unit {
	case unitC:
		return e.cfg.Alerts.Baseline.MarginTemp
	case unitRPM:
		return e.cfg.Alerts.Baseline.MarginRPM
	default:
		return e.cfg.Alerts.Baseline.MarginPercent
	}
}

func (e *Engine) thresholdHot(host, key string) bool {
	lat := e.latches[latchID(host, kindThreshold, key)]
	return lat != nil && lat.visible
}

func (e *Engine) tempLimits(t metrics.Temp) (amber, red float64) {
	switch t.Kind {
	case "gpu":
		return e.cfg.Threshold.GPUTemp()
	case "package", "core", "cpu":
		return e.cfg.Threshold.CPUTemp()
	default:
		return e.cfg.Threshold.TempAmber, e.cfg.Threshold.TempRed
	}
}

func (e *Engine) tempMetric(t metrics.Temp) string {
	if t.Kind == "gpu" {
		return "gpu"
	}
	return "temp"
}

func (e *Engine) bump(host, kind, key, metric, unit, level string, value, limit float64, sustain time.Duration, now time.Time) (Event, bool) {
	lid := latchID(host, kind, key)
	if level == "" {
		delete(e.latches, lid)
		return Event{}, false
	}
	lat := e.latches[lid]
	if lat == nil {
		lat = &latch{overSince: now}
		e.latches[lid] = lat
	}
	lat.ev = Event{
		Host:   host,
		Key:    key,
		Value:  value,
		Limit:  limit,
		Level:  level,
		Kind:   kind,
		Metric: metric,
		Unit:   unit,
		At:     now,
	}
	if lat.fired != "" && rank(level) < rank(lat.fired) {
		lat.fired = level
	}
	if sustain <= 0 || lat.fired != "" {
		lat.visible = true
	}
	ready := sustain <= 0 || !now.Before(lat.overSince.Add(sustain))
	if !ready {
		return Event{}, false
	}
	if rank(level) <= rank(lat.fired) {
		return Event{}, false
	}
	if lat.fired == "" && !e.cooled(lid, now) {
		return Event{}, false
	}
	lat.fired = level
	lat.visible = true
	lat.ev.At = now
	e.lastFire[lid] = now
	return lat.ev, true
}

func (e *Engine) cooled(lid string, now time.Time) bool {
	cd := e.cfg.Alerts.Cooldown
	if cd <= 0 {
		return true
	}
	last, ok := e.lastFire[lid]
	if !ok {
		return true
	}
	return !now.Before(last.Add(cd))
}

func (e *Engine) sweep(host, kind string, seen map[string]bool) {
	prefix := host + "\x00" + kind + "\x00"
	for lid := range e.latches {
		if strings.HasPrefix(lid, prefix) && !seen[lid] {
			delete(e.latches, lid)
		}
	}
}

func latchID(host, kind, key string) string {
	return host + "\x00" + kind + "\x00" + key
}

type level struct {
	limit float64
	name  string
}

func classify(value, amber, red float64) level {
	switch {
	case red > 0 && value >= red:
		return level{limit: red, name: "red"}
	case amber > 0 && value >= amber:
		return level{limit: amber, name: "amber"}
	default:
		return level{}
	}
}

func rank(name string) int {
	switch name {
	case "amber":
		return 1
	case "red":
		return 2
	default:
		return 0
	}
}

// RunCommand starts the configured command without waiting.
// The command is executed through the platform shell.
func RunCommand(command string, ev Event) {
	if command == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.CommandContext(ctx, "cmd", "/C", command)
		} else {
			cmd = exec.CommandContext(ctx, "/bin/sh", "-c", command)
		}
		cmd.Env = append(os.Environ(),
			"RIGWATCH_HOST="+ev.Host,
			"RIGWATCH_KEY="+ev.Key,
			"RIGWATCH_LEVEL="+ev.Level,
			"RIGWATCH_KIND="+ev.Kind,
			"RIGWATCH_METRIC="+ev.Metric,
			fmt.Sprintf("RIGWATCH_VALUE=%.2f", ev.Value),
			fmt.Sprintf("RIGWATCH_LIMIT=%.2f", ev.Limit),
		)
		_ = cmd.Run()
	}()
}
