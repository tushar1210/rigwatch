package collect

import "testing"

func TestParseNvidiaSMI(t *testing.T) {
	out := "NVIDIA GeForce RTX 3080, 12, 55, 30, 1024, 10240, 80.5, 1800, 9501, 00000000:01:00.0\n" +
		"NVIDIA A2, [N/A], 40, [Not Supported], 100, 8192, [N/A], 1200, 5000, 0000:02:00.0\n"
	gpus := ParseNvidiaSMI(out)
	if len(gpus) != 2 {
		t.Fatalf("len = %d", len(gpus))
	}
	g := gpus[0]
	if g.Name != "NVIDIA GeForce RTX 3080" || g.Util.Value != 12 || g.Temp.Value != 55 || g.FanPercent.Value != 30 {
		t.Fatalf("gpu0 = %+v", g)
	}
	if g.Power.Value != 80.5 || g.ClockCore.Value != 1800 || g.VRAMTotal != 10240*1024*1024 {
		t.Fatalf("details = %+v", g)
	}
	if gpus[1].Util.Available || gpus[1].FanPercent.Available || gpus[1].Temp.Value != 40 {
		t.Fatalf("gpu1 = %+v", gpus[1])
	}
}
