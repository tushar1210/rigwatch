package agent

import (
	"encoding/json"
	"strings"
	"testing"

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
