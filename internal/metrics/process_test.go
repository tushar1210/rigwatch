package metrics

import "testing"

func TestSortByRSSNotPercent(t *testing.T) {
	rows := []Process{
		{PID: 2, Name: "percent", MemPercent: 80, RSS: 100},
		{PID: 3, Name: "working", MemPercent: 5, RSS: 5000},
		{PID: 1, Name: "tie", MemPercent: 5, RSS: 5000},
	}
	got := SortProcesses(rows, SortByRSS)
	if got[0].Name != "tie" || got[1].Name != "working" || got[2].Name != "percent" {
		t.Fatalf("order = %s %s %s", got[0].Name, got[1].Name, got[2].Name)
	}
}

func TestSortByCPU(t *testing.T) {
	rows := []Process{
		{PID: 2, Name: "b", CPUPercent: 1, RSS: 9},
		{PID: 1, Name: "a", CPUPercent: 9, RSS: 1},
	}
	got := SortProcesses(rows, SortByCPU)
	if got[0].Name != "a" || got[1].Name != "b" {
		t.Fatalf("order = %s %s", got[0].Name, got[1].Name)
	}
}

func TestSortByGPUUtilThenMemory(t *testing.T) {
	rows := []Process{
		{PID: 3, Name: "idle", RSS: 999},
		{PID: 2, Name: "mem", GPUUtilOK: true, GPUPercent: 10, GPUMemOK: true, GPUMem: 100},
		{PID: 4, Name: "hot", GPUUtilOK: true, GPUPercent: 10, GPUMemOK: true, GPUMem: 800},
		{PID: 1, Name: "top", GPUUtilOK: true, GPUPercent: 90, GPUMemOK: true, GPUMem: 1},
	}
	got := SortProcesses(rows, SortByGPU)
	want := []string{"top", "hot", "mem", "idle"}
	for i, name := range want {
		if got[i].Name != name {
			t.Fatalf("gpu order[%d] = %s, want %s", i, got[i].Name, name)
		}
	}
}

func TestSortByVRAMBytesOverPercent(t *testing.T) {
	rows := []Process{
		{PID: 1, Name: "pct", GPUMemPctOK: true, GPUMemPercent: 99},
		{PID: 2, Name: "bytes", GPUMemOK: true, GPUMem: 10},
		{PID: 3, Name: "none"},
	}
	got := SortProcesses(rows, SortByVRAM)
	if got[0].Name != "bytes" || got[1].Name != "pct" || got[2].Name != "none" {
		t.Fatalf("order = %s %s %s", got[0].Name, got[1].Name, got[2].Name)
	}
}

func TestSortByVRAMPercentWhenNoBytes(t *testing.T) {
	rows := []Process{
		{PID: 1, Name: "low", GPUMemPctOK: true, GPUMemPercent: 2},
		{PID: 2, Name: "high", GPUMemPctOK: true, GPUMemPercent: 40},
		{PID: 3, Name: "none"},
	}
	got := SortProcesses(rows, SortByVRAM)
	if got[0].Name != "high" || got[1].Name != "low" || got[2].Name != "none" {
		t.Fatalf("order = %s %s %s", got[0].Name, got[1].Name, got[2].Name)
	}
}
