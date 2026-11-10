package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/exitcode"
)

// A11/A12 (h-59ej): explicit exit codes survive non-TTY; other errors keep
// the exit-0 policy for agents (ec970bb).
func TestExitCode(t *testing.T) {
	plain := errors.New("boom")
	deployFailed := exitcode.Wrap(2, errors.New("deploy failed"))
	tests := []struct {
		name        string
		err         error
		mode        string
		interactive bool
		want        int
	}{
		{"plain error, non-TTY", plain, "cli", false, 0},
		{"plain error, TTY", plain, "cli", true, 1},
		{"exit error, non-TTY", deployFailed, "cli", false, 2},
		{"exit error, TTY", deployFailed, "cli", true, 2},
		{"wrapped exit error, non-TTY", fmt.Errorf("ctx: %w", exitcode.Wrap(3, plain)), "cli", false, 3},
		{"exit error, MCP mode", deployFailed, "mcp", false, 0},
		{"plain error, MCP mode", plain, "mcp", true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := exitCode(tt.err, tt.mode, tt.interactive); got != tt.want {
				t.Fatalf("exitCode = %d, want %d", got, tt.want)
			}
		})
	}
}
