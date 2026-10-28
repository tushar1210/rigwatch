package collect

import (
	"strings"

	"github.com/shirou/gopsutil/v4/disk"
)

var skipFS = map[string]struct{}{
	"autofs": {}, "binfmt_misc": {}, "bpf": {}, "cgroup": {}, "cgroup2": {},
	"configfs": {}, "debugfs": {}, "devfs": {}, "devpts": {}, "devtmpfs": {},
	"fusectl": {}, "hugetlbfs": {}, "mqueue": {}, "nsfs": {}, "overlay": {},
	"proc": {}, "pstore": {}, "ramfs": {}, "securityfs": {}, "squashfs": {},
	"sysfs": {}, "tmpfs": {}, "tracefs": {},
}

func skipPartition(p disk.PartitionStat) bool {
	if _, ok := skipFS[p.Fstype]; ok {
		return true
	}
	mp := p.Mountpoint
	if mp == "" {
		return true
	}
	for _, prefix := range []string{"/proc", "/sys", "/dev", "/run", "/snap"} {
		if mp == prefix || strings.HasPrefix(mp, prefix+"/") {
			return true
		}
	}
	return false
}

func diskLabel(device, mount string) string {
	device = strings.TrimPrefix(device, "/dev/")
	if mount == "/" {
		if device != "" {
			return device + " /"
		}
		return "/"
	}
	if device != "" {
		return device
	}
	return mount
}

func parentDevice(device string) string {
	name := strings.TrimPrefix(device, "/dev/")
	if name == "" {
		return ""
	}
	if strings.Contains(name, "nvme") {
		if i := strings.LastIndex(name, "p"); i > 0 && digits(name[i+1:]) {
			return name[:i]
		}
	}
	return strings.TrimRight(name, "0123456789")
}

func digits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func skipNIC(name string) bool {
	if name == "lo" || name == "lo0" {
		return true
	}
	for _, prefix := range []string{"veth", "docker", "br-", "virbr", "cali", "flannel", "cni", "lxc", "tunl", "dummy", "kube", "tailscale"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}
