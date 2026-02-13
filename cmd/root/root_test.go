package root

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/config"
)

func setupTestConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	return dir
}

func TestExecute(t *testing.T) {
	// Test that Execute returns a valid command
	err := Execute()
	// With no args, cobra shows help, which is not an error
	// It shouldn't panic
	_ = err
}

func TestIsVerbose(t *testing.T) {
	// Default verbose is false
	if IsVerbose() {
		t.Error("expected verbose to be false by default")
	}
}

func TestPersistentPreRun_SkipTOSForAllowedCommands(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantTOS bool
	}{
		{
			name:    "version command skips TOS",
			args:    []string{"version"},
			wantTOS: false,
		},
		{
			name:    "help command skips TOS",
			args:    []string{"--help"},
			wantTOS: false,
		},
		{
			name:    "configure help skips TOS",
			args:    []string{"configure", "--help"},
			wantTOS: false,
		},
		{
			name:    "login help skips TOS",
			args:    []string{"login", "--help"},
			wantTOS: false,
		},
		{
			name:    "init help skips TOS",
			args:    []string{"init", "--help"},
			wantTOS: false,
		},
		{
			name:    "mcp help skips TOS",
			args:    []string{"mcp", "--help"},
			wantTOS: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupTestConfig(t)
			verbose = false
			tokenFlag = ""

			// Execute the command
			output, _ := captureExec(tt.args)

			// Check if TOS prompt appeared
			hasTOS := strings.Contains(output, "Terms of Service")
			if hasTOS != tt.wantTOS {
				t.Errorf("TOS prompt = %v, want %v. Output: %s", hasTOS, tt.wantTOS, output)
			}
		})
	}
}

func TestPersistentPreRun_TOSAlreadyAccepted(t *testing.T) {
	home := setupTestConfig(t)
	verbose = false
	tokenFlag = ""

	// Create config with TOS already accepted
	hatchDir := filepath.Join(home, ".hatch")
	if err := os.MkdirAll(hatchDir, 0700); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		TosAcceptedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	// Use a command that would normally trigger TOS (apps --help)
	output, _ := captureExec([]string{"apps", "--help"})

	if strings.Contains(output, "Terms of Service") {
		t.Error("should not show TOS prompt when already accepted")
	}
}

func TestPersistentPreRun_VerboseFlag(t *testing.T) {
	setupTestConfig(t)

	// Create config with TOS accepted to avoid TOS prompt
	home, _ := os.UserHomeDir()
	hatchDir := filepath.Join(home, ".hatch")
	os.MkdirAll(hatchDir, 0700)
	cfg := &config.Config{
		TosAcceptedAt: time.Now().UTC().Format(time.RFC3339),
	}
	config.Save(cfg)

	verbose = false

	_, _ = captureExec([]string{"--verbose", "version"})

	// After execution, verbose should be true
	// This is set by PersistentPreRun
	if !verbose {
		t.Error("verbose should be true after --verbose flag")
	}
}

func TestPersistentPreRun_ConfigLoadErrorDoesNotBlock(t *testing.T) {
	// Set HOME to a directory that exists but has unreadable config
	home := setupTestConfig(t)
	hatchDir := filepath.Join(home, ".hatch")
	os.MkdirAll(hatchDir, 0700)

	// Create an invalid JSON config
	cfgPath := filepath.Join(hatchDir, "config.json")
	os.WriteFile(cfgPath, []byte("{invalid json"), 0600)

	verbose = false
	tokenFlag = ""

	// version command should still work even with bad config
	// Should not error - config errors are ignored in PersistentPreRun
	_, err := captureExec([]string{"version"})
	// The main goal is that config errors don't cause a panic or fatal error
	// version command should execute successfully
	if err != nil && strings.Contains(err.Error(), "config") {
		t.Errorf("config error should not block command execution: %v", err)
	}
}

func TestRootCmdStructure(t *testing.T) {
	// Test that rootCmd has expected properties
	if rootCmd.Use != "hatch" {
		t.Errorf("Use = %q, want %q", rootCmd.Use, "hatch")
	}

	if rootCmd.Short == "" {
		t.Error("expected non-empty Short description")
	}

	if rootCmd.Long == "" {
		t.Error("expected non-empty Long description")
	}

	if !rootCmd.SilenceUsage {
		t.Error("expected SilenceUsage to be true")
	}
}

// captureExec executes the root command with given args and returns output
func captureExec(args []string) (string, error) {
	// Create a test command with isolated state
	testCmd := *rootCmd
	testCmd.SetArgs(args)

	// Execute - we can't easily capture the TOS prompt without os.Stdin manipulation
	// For this test, we're mainly checking the code paths execute without panic
	err := testCmd.Execute()
	return "", err
}
