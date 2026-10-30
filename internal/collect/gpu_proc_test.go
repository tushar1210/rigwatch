package collect

import (
	"testing"

	"github.com/tushar1210/rigwatch/internal/metrics"
)

func TestParseNvidiaComputeApps(t *testing.T) {
	out := "pid, process_name, used_gpu_memory\n" +
		"4210, python, 512\n" +
		"4210, python, 256\n" +
		"1882, chrome helper, [N/A]\n" +
		"bad, nope, 10\n"
	got := parseNvidiaComputeApps(out)
	if len(got) != 2 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].PID != 4210 || got[0].Name != "python" || !got[0].MemOK || got[0].Mem != 768*1024*1024 {
		t.Fatalf("python = %+v", got[0])
	}
	if got[1].PID != 1882 || got[1].MemOK {
		t.Fatalf("chrome = %+v", got[1])
	}
}

func TestParseNvidiaPmon(t *testing.T) {
	out := "# gpu         pid  type    sm   mem   enc   dec   command\n" +
		"# Idx           #   C/G     %     %     %     %   name\n" +
		"    0       4210     C     40    12      -      -    python train\n" +
		"    1       4210     C     15     4      -      -    python train\n" +
		"    0          -     -      -     -      -      -    -\n"
	got := parseNvidiaPmon(out)
	if len(got) != 1 {
		t.Fatalf("len = %d", len(got))
	}
	g := got[0]
	if g.PID != 4210 || !g.UtilOK || g.Util != 40 || !g.MemPctOK || g.MemPct != 12 || g.Name != "python train" {
		t.Fatalf("pmon = %+v", g)
	}
}

func TestMergeGPUProcs(t *testing.T) {
	rows := []metrics.Process{
		{PID: 7, Name: "local", CPUPercent: 3, RSS: 100},
	}
	stats := overlayGPUStats(
		[]gpuStat{{PID: 7, Util: 22, UtilOK: true, MemPct: 5, MemPctOK: true}},
		[]gpuStat{{PID: 7, Name: "local", Mem: 4096, MemOK: true}, {PID: 9, Name: "onlygpu", Util: 80, UtilOK: true}},
	)
	got := mergeGPUProcs(rows, stats)
	if len(got) != 2 {
		t.Fatalf("len = %d", len(got))
	}
	if !got[0].GPUUtilOK || got[0].GPUPercent != 22 || !got[0].GPUMemOK || got[0].GPUMem != 4096 || !got[0].GPUMemPctOK {
		t.Fatalf("merged = %+v", got[0])
	}
	if got[1].PID != 9 || got[1].Name != "onlygpu" || got[1].GPUPercent != 80 {
		t.Fatalf("extra = %+v", got[1])
	}
}

func TestTopProcessesKeepsGPUAndRSS(t *testing.T) {
	rows := []metrics.Process{
		{PID: 1, Name: "cpu", CPUPercent: 90, RSS: 10},
		{PID: 2, Name: "rss", CPUPercent: 0.1, RSS: 9 << 30, MemPercent: 1},
		{PID: 3, Name: "gpu", CPUPercent: 0.1, RSS: 10, GPUPercent: 70, GPUUtilOK: true},
	}
	for i := 0; i < 20; i++ {
		rows = append(rows, metrics.Process{PID: int32(100 + i), Name: "filler", CPUPercent: 0.2, RSS: 20})
	}
	got := topProcesses(rows, 2)
	seen := map[string]bool{}
	for _, p := range got {
		seen[p.Name] = true
	}
	for _, name := range []string{"cpu", "rss", "gpu"} {
		if !seen[name] {
			t.Fatalf("missing %s in %+v", name, got)
		}
	}
}
