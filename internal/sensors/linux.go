//go:build linux

package sensors

import "context"

func readPlatform(_ context.Context, _ string) Reading {
	temps, fans := ReadSysfs("/sys")
	hint := ""
	if len(temps) == 0 && len(fans) == 0 {
		hint = "no hwmon sensors"
	}
	return Reading{Temps: temps, Fans: fans, Hint: hint}
}
