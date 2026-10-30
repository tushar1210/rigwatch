//go:build windows

package tui

import (
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// Windows has no POSIX signals. SIGTERM is taskkill without /F (close request).
// SIGKILL is taskkill /F (TerminateProcess).
func killOS(pid int32, sig int) error {
	args := []string{"/PID", strconv.Itoa(int(pid))}
	if sig == sigKill {
		args = append([]string{"/F"}, args...)
	}
	cmd := exec.Command("taskkill", args...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(string(out))
	if msg == "" {
		return err
	}
	return errors.New(msg)
}

func errnoDenied(err error) bool {
	var errno syscall.Errno
	return errors.As(err, &errno) && errno == 5
}

func errnoMissing(err error) bool {
	return false
}
