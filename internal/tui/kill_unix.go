//go:build !windows

package tui

import (
	"errors"
	"syscall"
)

func killOS(pid int32, sig int) error {
	s := syscall.SIGTERM
	if sig == sigKill {
		s = syscall.SIGKILL
	}
	return syscall.Kill(int(pid), s)
}

func errnoDenied(err error) bool {
	return errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES)
}

func errnoMissing(err error) bool {
	return errors.Is(err, syscall.ESRCH)
}
