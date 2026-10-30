package power

import (
	"context"
	"os"
	"os/exec"
	"reflect"
	"testing"
)

func TestHaltInvokesShutdown(t *testing.T) {
	var got []string
	execCommandContext = func(_ context.Context, name string, args ...string) *exec.Cmd {
		got = append([]string{name}, args...)
		return exec.Command(os.Args[0], "-test.run=^$")
	}
	t.Cleanup(func() { execCommandContext = exec.CommandContext })

	if err := Halt(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, haltArgs()) {
		t.Fatalf("command = %v, want %v", got, haltArgs())
	}
}
