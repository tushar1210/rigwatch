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

func TestShutdownRequestShape(t *testing.T) {
	var buf strings.Builder
	if err := WriteShutdown(&buf); err != nil {
		t.Fatal(err)
	}
	line := []byte(strings.TrimSpace(buf.String()))
	if !IsShutdownRequest(line) {
		t.Fatalf("request = %s", line)
	}
	if err := WriteShutdownResult(&buf, jsonStringErr("must be root")); err != nil {
		t.Fatal(err)
	}
	rest := strings.TrimSpace(buf.String()[strings.Index(buf.String(), "\n")+1:])
	if IsShutdownRequest([]byte(rest)) {
		t.Fatalf("result looked like a request: %s", rest)
	}
	frame, err := DecodeFrame([]byte(rest))
	if err != nil || frame.Type != "shutdown" || frame.OK || frame.Error != "must be root" {
		t.Fatalf("frame %+v %v", frame, err)
	}
}

type jsonStringErr string

func (e jsonStringErr) Error() string { return string(e) }
