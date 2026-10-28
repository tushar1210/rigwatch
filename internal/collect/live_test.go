package collect

import (
	"context"
	"testing"
	"time"
)

func TestLiveSample(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	live := NewLive("")
	snap, err := live.Sample(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Host.Hostname == "" {
		t.Fatal("empty hostname")
	}
	if snap.Memory.Total == 0 {
		t.Fatal("memory total is 0")
	}
	if len(snap.CPU.PerCore) == 0 {
		t.Fatal("no per-core cpu")
	}
}
