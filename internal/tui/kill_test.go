package tui

import (
	"errors"
	"strings"
	"testing"
)

func TestKillSignalLabels(t *testing.T) {
	ask := newKillAsk(4210, "rustc")
	if ask.sig != sigTerm {
		t.Fatalf("default sig = %d", ask.sig)
	}
	if !strings.Contains(ask.prompt(), "TERM") || !strings.Contains(ask.prompt(), "9 kill") {
		t.Fatalf("prompt = %s", ask.prompt())
	}
	ask.sig = sigKill
	if !strings.Contains(ask.prompt(), "KILL") || strings.Contains(ask.prompt(), "9 kill") {
		t.Fatalf("kill prompt = %s", ask.prompt())
	}
}

func TestKillFailureMessages(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{nil, ""},
		{errKillRefused, "refused"},
		{errors.New("operation not permitted"), "access denied"},
		{errors.New("Access is denied."), "access denied"},
		{errors.New("permission denied"), "access denied"},
		{errors.New("no such process"), "not running"},
		{errors.New(`The process "4" not found.`), "not running"},
		{errors.New("boom"), "failed"},
	}
	for _, tc := range cases {
		if got := killFailure(tc.err); got != tc.want {
			t.Fatalf("killFailure(%v) = %q, want %q", tc.err, got, tc.want)
		}
	}
}

func TestKillRejectsUnsafePID(t *testing.T) {
	for _, pid := range []int32{-5, -1, 0, 1} {
		err := killPID(pid, sigTerm)
		if !errors.Is(err, errKillRefused) {
			t.Fatalf("pid %d: %v", pid, err)
		}
		if killFailure(err) != "refused" {
			t.Fatalf("pid %d message %q", pid, killFailure(err))
		}
	}
}
