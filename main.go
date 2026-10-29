// Command rigwatch is the module-root entrypoint for
// `go install github.com/tushar1210/rigwatch@latest`.
// Release builds use the same main in cmd/rigwatch.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/tushar1210/rigwatch/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cmd := cli.New()
	if err := cmd.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "rigwatch:", err)
		os.Exit(1)
	}
}
