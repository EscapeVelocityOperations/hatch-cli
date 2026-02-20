package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/EscapeVelocityOperations/hatch-cli/cmd/root"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/telemetry"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/update"
	"golang.org/x/term"
)

func main() {
	// Initialize telemetry version
	telemetry.Version = root.Version()

	// Start update check in background (non-blocking)
	type updateResult struct {
		result *update.CheckResult
	}
	updateCh := make(chan updateResult, 1)
	go func() {
		r := update.Check(root.Version())
		updateCh <- updateResult{result: r}
	}()

	err := root.Execute()

	if err != nil {
		// Send telemetry for CLI errors
		telemetry.Send(
			root.LastCommand(),
			strings.Join(root.LastArgs(), " "),
			err.Error(),
			root.LastMode(),
		)
		// Brief wait for telemetry goroutine to fire
		time.Sleep(50 * time.Millisecond)

		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Show update notification (suppress in MCP mode, non-interactive, or dev builds)
	if root.LastMode() != "mcp" && root.Version() != "dev" && isInteractive() {
		select {
		case u := <-updateCh:
			if msg := update.FormatNotification(u.result); msg != "" {
				fmt.Fprint(os.Stderr, msg)
			}
		case <-time.After(500 * time.Millisecond):
			// Don't delay CLI exit waiting for update check
		}
	}
}

// isInteractive returns true if stderr is a terminal (not piped).
func isInteractive() bool {
	return term.IsTerminal(int(os.Stderr.Fd()))
}
