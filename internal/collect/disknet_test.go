package collect

import (
	"testing"
	"time"
)

func TestParentDevice(t *testing.T) {
	if got := parentDevice("/dev/nvme0n1p2"); got != "nvme0n1" {
		t.Fatalf("nvme = %q", got)
	}
	if got := parentDevice("/dev/sda1"); got != "sda" {
		t.Fatalf("sda = %q", got)
	}
}

func TestPerSec(t *testing.T) {
	if got := perSec(100, 1100, time.Second); got != 1000 {
		t.Fatalf("rate = %v", got)
	}
	if got := perSec(500, 100, time.Second); got != 0 {
		t.Fatalf("reset = %v", got)
	}
}

func TestSkipNIC(t *testing.T) {
	if !skipNIC("lo") || !skipNIC("veth123") || skipNIC("eth0") {
		t.Fatal("nic filter")
	}
}
