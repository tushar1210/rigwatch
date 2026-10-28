package alert

import (
	"testing"

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
