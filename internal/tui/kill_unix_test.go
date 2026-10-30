//go:build !windows

package tui

import (
	"syscall"
	"testing"
)

func TestKillFailureErrno(t *testing.T) {
	if killFailure(syscall.EPERM) != "access denied" {
		t.Fatalf("EPERM -> %q", killFailure(syscall.EPERM))
	}
	if killFailure(syscall.EACCES) != "access denied" {
		t.Fatalf("EACCES -> %q", killFailure(syscall.EACCES))
	}
	if killFailure(syscall.ESRCH) != "not running" {
		t.Fatalf("ESRCH -> %q", killFailure(syscall.ESRCH))
	}
}
