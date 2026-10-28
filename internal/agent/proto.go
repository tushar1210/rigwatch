package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"

	"github.com/tushar1210/rigwatch/internal/metrics"
)

// Envelope is one line on the stdio stream and one WebSocket text frame.
type Envelope struct {
	Type     string           `json:"type"`
	Snapshot metrics.Snapshot `json:"snapshot"`
}

// WriteSnapshot writes one NDJSON envelope.
func WriteSnapshot(w io.Writer, snap metrics.Snapshot) error {
	enc := json.NewEncoder(w)
	return enc.Encode(Envelope{Type: "snapshot", Snapshot: snap})
}

// ReadSnapshots decodes NDJSON envelopes or bare snapshots until EOF.
func ReadSnapshots(r io.Reader, fn func(metrics.Snapshot) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 8<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		snap, err := decodeLine(line)
		if err != nil {
			return err
		}
		if err := fn(snap); err != nil {
			return err
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return nil
}

func decodeLine(line []byte) (metrics.Snapshot, error) {
	var env Envelope
	if err := json.Unmarshal(line, &env); err == nil && env.Type == "snapshot" {
		return env.Snapshot, nil
	}
	var snap metrics.Snapshot
	if err := json.Unmarshal(line, &snap); err != nil {
		return metrics.Snapshot{}, fmt.Errorf("decode snapshot: %w", err)
	}
	return snap, nil
}
