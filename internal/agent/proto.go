package agent

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/tushar1210/rigwatch/internal/alert"
	"github.com/tushar1210/rigwatch/internal/metrics"
)

// errSkipFrame means the line was a valid non-snapshot frame.
var errSkipFrame = errors.New("skip frame")

// Envelope is one line on the stdio stream and one WebSocket text frame.
// Snapshot samples keep their existing shape. Alert lines use type "alert".
type Envelope struct {
	Type     string            `json:"type"`
	Snapshot *metrics.Snapshot `json:"snapshot,omitempty"`
	Alert    *alert.Event      `json:"alert,omitempty"`
}

// Frame is a decoded stream message.
type Frame struct {
	Type     string
	Snapshot metrics.Snapshot
	Alert    *alert.Event
}

// WriteSnapshot writes one NDJSON snapshot envelope.
func WriteSnapshot(w io.Writer, snap metrics.Snapshot) error {
	s := snap
	enc := json.NewEncoder(w)
	return enc.Encode(Envelope{Type: "snapshot", Snapshot: &s})
}

// WriteAlert writes one NDJSON alert envelope.
func WriteAlert(w io.Writer, ev alert.Event) error {
	enc := json.NewEncoder(w)
	return enc.Encode(Envelope{Type: "alert", Alert: &ev})
}

// DecodeFrame parses a snapshot envelope, a bare snapshot, or an alert envelope.
// Unknown types are returned with Type set so callers can ignore them.
func DecodeFrame(line []byte) (Frame, error) {
	var env Envelope
	if err := json.Unmarshal(line, &env); err != nil {
		return Frame{}, fmt.Errorf("decode frame: %w", err)
	}
	switch env.Type {
	case "alert":
		return Frame{Type: "alert", Alert: env.Alert}, nil
	case "snapshot":
		if env.Snapshot == nil {
			return Frame{Type: "snapshot"}, nil
		}
		return Frame{Type: "snapshot", Snapshot: *env.Snapshot}, nil
	case "":
		var snap metrics.Snapshot
		if err := json.Unmarshal(line, &snap); err != nil {
			return Frame{}, fmt.Errorf("decode snapshot: %w", err)
		}
		return Frame{Type: "snapshot", Snapshot: snap}, nil
	default:
		return Frame{Type: env.Type}, nil
	}
}

// ReadSnapshots decodes NDJSON envelopes or bare snapshots until EOF.
// Alert lines and unknown frame types are skipped.
func ReadSnapshots(r io.Reader, fn func(metrics.Snapshot) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 8<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		frame, err := DecodeFrame(line)
		if err != nil {
			return err
		}
		if frame.Type != "snapshot" {
			continue
		}
		if err := fn(frame.Snapshot); err != nil {
			return err
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return nil
}

func decodeLine(line []byte) (metrics.Snapshot, error) {
	frame, err := DecodeFrame(line)
	if err != nil {
		return metrics.Snapshot{}, err
	}
	if frame.Type != "snapshot" {
		return metrics.Snapshot{}, errSkipFrame
	}
	return frame.Snapshot, nil
}
