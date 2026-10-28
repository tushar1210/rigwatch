package collect

import (
	"testing"

	"github.com/tushar1210/rigwatch/internal/metrics"
)

func TestMergeGPUs(t *testing.T) {
	primary := []metrics.GPU{{
		Name:   "NVIDIA GeForce RTX 3080",
		Vendor: "nvidia",
		Source: "nvml",
		PCI:    "00000000:01:00.0",
		Util:   metrics.OK(10),
		Temp:   metrics.Missing("unsupported"),
	}}
	extra := []metrics.GPU{{
		Name:   "NVIDIA GeForce RTX 3080",
		Vendor: "nvidia",
		Source: "lhm",
		PCI:    "0000:01:00.0",
		Temp:   metrics.OK(66),
		FanRPM: metrics.OK(1100),
	}}
	got := mergeGPUs(primary, extra)
	if len(got) != 1 {
		t.Fatalf("len = %d", len(got))
	}
	if !got[0].Temp.Available || got[0].Temp.Value != 66 || got[0].Util.Value != 10 || !got[0].FanRPM.Available {
		t.Fatalf("merged = %+v", got[0])
	}
}
