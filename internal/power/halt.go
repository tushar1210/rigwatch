// Package power stops the machine rigwatch is running on.
package power

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// execCommandContext is replaced in tests so Halt never powers off the test runner.
var execCommandContext = exec.CommandContext

// Halt powers the machine off. It asks the OS to shut down and returns when
// that request is accepted or refused.
func Halt() error {
	return HaltContext(context.Background())
}

// HaltContext is Halt with a deadline. A cancelled ctx abandons the request.
func HaltContext(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	args := haltArgs()
	cmd := execCommandContext(ctx, args[0], args[1:]...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(string(out))
	if msg == "" {
		return err
	}
	line := msg
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	return fmt.Errorf("%s", strings.TrimSpace(line))
}
