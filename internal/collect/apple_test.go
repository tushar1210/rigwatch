package collect

import "testing"

func TestParseIORegAccelerator(t *testing.T) {
	text := `
+-o AGXAcceleratorG16G  <class IOAccelerator, id 0x100000123, registered, matched, active, busy 0>
  |   "PerformanceStatistics" = {"Device Utilization %"=12,"In use system memory"=2147483648,"Alloc system memory"=100}
`
	gpus := ParseIORegAccelerator(text)
	if len(gpus) != 1 {
		t.Fatalf("gpus = %+v", gpus)
	}
	g := gpus[0]
	if g.Vendor != "apple" || !g.Unified || g.Util.Value != 12 || g.VRAMUsed != 2147483648 {
		t.Fatalf("gpu = %+v", g)
	}
	if g.Temp.Available || g.Temp.Hint == "" {
		t.Fatalf("temp = %+v", g.Temp)
	}
}
