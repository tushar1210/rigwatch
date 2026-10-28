// Package alert tracks threshold breaches and optionally runs a command.
package alert

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/tushar1210/rigwatch/internal/config"
	"github.com/tushar1210/rigwatch/internal/metrics"
)

// Event is a threshold that has just been crossed or has worsened.
type Event struct {
	Host  string
	Key   string
	Value float64
	Limit float64
	Level string
}

// Engine remembers the previous level of each key so alerts edge-trigger.
type Engine struct {
	th   config.Thresholds
	prev map[string]string
}

// New returns an engine for the given thresholds.
func New(th config.Thresholds) *Engine {
	return &Engine{th: th, prev: map[string]string{}}
}

// Evaluate returns alerts for levels that are new or more severe than last time.
func (e *Engine) Evaluate(host string, snap metrics.Snapshot) []Event {
	if e.prev == nil {
		e.prev = map[string]string{}
	}
	cur := map[string]level{}
	add := func(key string, value, amber, red float64) {
		lv := classify(value, amber, red)
		if lv.name == "" {
			return
		}
		cur[host+"|"+key] = level{key: key, value: value, limit: lv.limit, name: lv.name}
	}
	if t, ok := metrics.HeadlineTemp(snap.Temps); ok {
		add("cpu temp", t.Celsius, e.th.TempAmber, e.th.TempRed)
	}
	for _, t := range snap.Temps {
		add("temp "+t.Name, t.Celsius, e.th.TempAmber, e.th.TempRed)
	}
	add("cpu", snap.CPU.UsagePercent, e.th.CPUWarn, e.th.CPUWarn)
	add("memory", snap.Memory.UsedPercent, e.th.MemWarn, e.th.MemWarn)
	for _, g := range snap.GPUs {
		if g.Temp.Available {
			add("gpu temp "+g.Name, g.Temp.Value, e.th.TempAmber, e.th.TempRed)
		}
		if g.Util.Available {
			add("gpu util "+g.Name, g.Util.Value, e.th.GPUUtilWarn, e.th.GPUUtilWarn)
		}
	}

	var out []Event
	next := map[string]string{}
	for id, lv := range cur {
		next[id] = lv.name
		prev := e.prev[id]
		if rank(lv.name) > rank(prev) {
			out = append(out, Event{
				Host:  host,
				Key:   lv.key,
				Value: lv.value,
				Limit: lv.limit,
				Level: lv.name,
			})
		}
	}
	e.prev = next
	return out
}

type level struct {
	key   string
	value float64
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
			fmt.Sprintf("RIGWATCH_VALUE=%.2f", ev.Value),
			fmt.Sprintf("RIGWATCH_LIMIT=%.2f", ev.Limit),
		)
		_ = cmd.Run()
	}()
}
