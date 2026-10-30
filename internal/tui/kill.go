package tui

import (
	"errors"
	"fmt"
	"strings"
)

const (
	sigTerm = 15
	sigKill = 9
)

var errKillRefused = errors.New("refused")

// killPID signals pid. sigKill is SIGKILL; any other value is SIGTERM.
// pid <= 1 is refused so a bad selection cannot signal init or the process group.
func killPID(pid int32, sig int) error {
	if pid <= 1 {
		return errKillRefused
	}
	return killOS(pid, sig)
}

func sigLabel(sig int) string {
	if sig == sigKill {
		return "KILL"
	}
	return "TERM"
}

// killFailure is the short status shown when a kill does not land.
func killFailure(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, errKillRefused) {
		return "refused"
	}
	if errnoDenied(err) {
		return "access denied"
	}
	if errnoMissing(err) {
		return "not running"
	}
	low := strings.ToLower(err.Error())
	switch {
	case strings.Contains(low, "access is denied") || strings.Contains(low, "operation not permitted") || strings.Contains(low, "not permitted") || strings.Contains(low, "permission denied") || strings.Contains(low, "must be root"):
		return "access denied"
	case strings.Contains(low, "no such process") || strings.Contains(low, "not found"):
		return "not running"
	default:
		return "failed"
	}
}

// killAsk is the confirm step. Nothing is signaled until confirm.
type killAsk struct {
	pid  int32
	name string
	sig  int
}

func newKillAsk(pid int32, name string) *killAsk {
	return &killAsk{pid: pid, name: name, sig: sigTerm}
}

func (k killAsk) prompt() string {
	name := k.name
	if name == "" {
		name = "?"
	}
	name = trunc(name, 24)
	if k.sig == sigKill {
		return fmt.Sprintf("KILL %d %s?  y  n", k.pid, name)
	}
	return fmt.Sprintf("TERM %d %s?  y  n  9 kill", k.pid, name)
}
