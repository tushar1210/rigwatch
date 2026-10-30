package collect

import (
	"testing"

	"github.com/tushar1210/rigwatch/internal/metrics"
)

func TestParseNvidiaPmon(t *testing.T) {
	out := `
# gpu        pid  type    sm   mem   enc   dec   command
# Idx          #   C/G     %     %     %     %   name
    0       4210     C     62    14     -     -   rustc
    0          -     -      -     -     -     -   -
    1       4210     C     10     1     0     0   rustc
`
	rows := ParseNvidiaPmon(out)
	if len(rows) != 1 {
		t.Fatalf("rows = %+v", rows)
	}
	if rows[0].PID != 4210 || !rows[0].HasSM || rows[0].SM != 62 || rows[0].HasMem {
		t.Fatalf("row = %+v", rows[0])
	}
	if rows[0].Name != "rustc" {
		t.Fatalf("name = %q", rows[0].Name)
	}
}

func TestParseNvidiaPmonFramebuffer(t *testing.T) {
	out := `
# gpu   pid  type  sm  mem  enc  dec  fb  command
# Idx     #   C/G   %    %    %    %  MB  name
  0    1882    G    5    1    -    -  240  chrome
`
	rows := ParseNvidiaPmon(out)
	if len(rows) != 1 || !rows[0].HasMem || rows[0].MemBytes != 240*1024*1024 || rows[0].SM != 5 {
		t.Fatalf("row = %+v", rows)
	}
}

func TestParseNvidiaComputeApps(t *testing.T) {
	out := "4210, rustc, 1638\n1882, chrome, [N/A]\n"
	rows := ParseNvidiaComputeApps(out)
	if len(rows) != 2 {
		t.Fatalf("rows = %+v", rows)
	}
	if rows[0].PID != 4210 || !rows[0].HasMem || rows[0].MemBytes != 1638*1024*1024 || rows[0].Name != "rustc" {
		t.Fatalf("row0 = %+v", rows[0])
	}
	if rows[1].HasMem {
		t.Fatalf("missing memory should stay unset: %+v", rows[1])
	}
}

func TestApplyGPUProcs(t *testing.T) {
	rows := []metrics.Process{
		{PID: 4210, Name: "rustc", CPUPercent: 10},
		{PID: 12, Name: "systemd"},
	}
	stats := mergeGPUProcStats(
		ParseNvidiaComputeApps("4210, rustc, 512\n99, train, 2048\n"),
		ParseNvidiaPmon("0 4210 C 40 1 - - rustc\n"),
	)
	got := applyGPUProcs(rows, stats)
	if len(got) != 3 {
		t.Fatalf("len = %d %+v", len(got), got)
	}
	if !got[0].HasGPU || got[0].GPUPercent != 40 || !got[0].HasGPUMem || got[0].GPUMemBytes != 512*1024*1024 {
		t.Fatalf("rustc = %+v", got[0])
	}
	if got[1].HasGPU || got[1].HasGPUMem {
		t.Fatalf("systemd should stay plain: %+v", got[1])
	}
	if got[2].PID != 99 || got[2].Name != "train" || !got[2].HasGPUMem {
		t.Fatalf("added = %+v", got[2])
	}
	top := topProcesses(got, 1)
	found := false
	for _, p := range top {
		if p.PID == 99 {
			found = true
		}
	}
	if !found {
		t.Fatalf("gpu process dropped from top: %+v", top)
	}
}
