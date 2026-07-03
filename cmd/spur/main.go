// Command spur is the Spur command-line client. main is intentionally thin:
// it wires an OS-signal-aware context and hands off to internal/app.Run.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spurintel/cli/internal/app"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := app.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "spur: %v\n", err)
		os.Exit(1)
	}
}
