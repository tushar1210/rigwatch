package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tushar1210/rigwatch/internal/alert"
	"github.com/tushar1210/rigwatch/internal/metrics"
)

func TestReadSnapshots(t *testing.T) {
	var buf strings.Builder
	snap := metrics.Snapshot{Host: metrics.Host{Hostname: "forge"}}
	if err := WriteSnapshot(&buf, snap); err != nil {
		t.Fatal(err)
	}
	bare, err := json.Marshal(metrics.Snapshot{Host: metrics.Host{Hostname: "bare"}})
	if err != nil {
		t.Fatal(err)
	}
	buf.Write(bare)
	buf.WriteByte('\n')
	var got []string
	err = ReadSnapshots(strings.NewReader(buf.String()), func(s metrics.Snapshot) error {
		got = append(got, s.Host.Hostname)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "forge" || got[1] != "bare" {
		t.Fatalf("got %#v", got)
	}
}

func TestReadSnapshotsSkipsAlerts(t *testing.T) {
	var buf strings.Builder
	if err := WriteSnapshot(&buf, metrics.Snapshot{Host: metrics.Host{Hostname: "forge"}}); err != nil {
		t.Fatal(err)
	}
	if err := WriteAlert(&buf, alert.Event{Host: "forge", Key: "cpu temp", Level: "red", Kind: "threshold"}); err != nil {
		t.Fatal(err)
	}
	if err := WriteSnapshot(&buf, metrics.Snapshot{Host: metrics.Host{Hostname: "atlas"}}); err != nil {
		t.Fatal(err)
	}
	var got []string
	err := ReadSnapshots(strings.NewReader(buf.String()), func(s metrics.Snapshot) error {
		got = append(got, s.Host.Hostname)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "forge" || got[1] != "atlas" {
		t.Fatalf("got %#v", got)
	}
	frame, err := DecodeFrame([]byte(buf.String()[:strings.Index(buf.String(), "\n")+1]))
	if err != nil || frame.Type != "snapshot" || frame.Snapshot.Host.Hostname != "forge" {
		t.Fatalf("frame %+v %v", frame, err)
	}
}
