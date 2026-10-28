package sensors

import "testing"

func TestParseLHMJSON(t *testing.T) {
	raw := []byte(`{
	  "Text": "Computer",
	  "Children": [
	    {
	      "Text": "AMD Ryzen 7 5800X",
	      "Children": [
	        {"Text": "Temperatures", "Children": [
	          {"Text": "Core (Tctl/Tdie)", "Value": "62.5 °C"}
	        ]}
	      ]
	    },
	    {
	      "Text": "NVIDIA GeForce RTX 3080",
	      "Children": [
	        {"Text": "Temperatures", "Children": [
	          {"Text": "GPU Core", "Value": "55 °C"}
	        ]},
	        {"Text": "Fans", "Children": [
	          {"Text": "GPU Fan", "Value": "1200 RPM"},
	          {"Text": "GPU Fan", "Value": "45 %"}
	        ]},
	        {"Text": "Load", "Children": [
	          {"Text": "GPU Core", "Value": "30 %"}
	        ]},
	        {"Text": "Data", "Children": [
	          {"Text": "GPU Memory Used", "Value": "4.2 GB"},
	          {"Text": "GPU Memory Total", "Value": "10 GB"}
	        ]},
	        {"Text": "Power", "Children": [
	          {"Text": "GPU Package", "Value": "120 W"}
	        ]},
	        {"Text": "Clocks", "Children": [
	          {"Text": "GPU Core", "Value": "1800 MHz"},
	          {"Text": "GPU Memory", "Value": "9500 MHz"}
	        ]}
	      ]
	    }
	  ]
	}`)
	got, err := ParseLHMJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.GPUs) != 1 {
		t.Fatalf("gpus = %+v", got.GPUs)
	}
	g := got.GPUs[0]
	if g.Vendor != "nvidia" || g.Temp.Value != 55 || g.Util.Value != 30 || g.Power.Value != 120 {
		t.Fatalf("gpu = %+v", g)
	}
	if g.FanRPM.Value != 1200 || g.FanPercent.Value != 45 {
		t.Fatalf("fans on gpu = %+v %+v", g.FanRPM, g.FanPercent)
	}
	if g.ClockCore.Value != 1800 || g.ClockMem.Value != 9500 {
		t.Fatalf("clocks = %+v %+v", g.ClockCore, g.ClockMem)
	}
	if !g.VRAMOK || g.VRAMTotal == 0 || g.VRAMUsed == 0 {
		t.Fatalf("vram = %d/%d", g.VRAMUsed, g.VRAMTotal)
	}
	foundCPU := false
	for _, temp := range got.Temps {
		if temp.Kind == "package" && temp.Celsius == 62.5 {
			foundCPU = true
		}
	}
	if !foundCPU {
		t.Fatalf("temps = %+v", got.Temps)
	}
}

func TestFromWMISensors(t *testing.T) {
	rows := []SensorFromWMI{
		{Hardware: "AMD Ryzen", HardwareType: "Cpu", Name: "Core (Tctl/Tdie)", SensorType: "Temperature", Value: 70},
		{Hardware: "Radeon RX 6800", HardwareType: "GpuAmd", Name: "GPU Core", SensorType: "Temperature", Value: 64},
		{Hardware: "Radeon RX 6800", HardwareType: "GpuAmd", Name: "GPU Fan", SensorType: "Fan", Value: 900},
		{Hardware: "Radeon RX 6800", HardwareType: "GpuAmd", Name: "GPU Core", SensorType: "Load", Value: 22},
	}
	got := FromWMISensors(rows)
	if len(got.GPUs) != 1 || got.GPUs[0].Vendor != "amd" || got.GPUs[0].Temp.Value != 64 {
		t.Fatalf("gpus = %+v", got.GPUs)
	}
	if len(got.Fans) != 1 || got.Fans[0].RPM != 900 {
		t.Fatalf("fans = %+v", got.Fans)
	}
}
