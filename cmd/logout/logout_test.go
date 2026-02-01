package logout

import (
	"bytes"
	"testing"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/auth"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/config"
)

func setupHome(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
}

func TestLogoutClearsToken(t *testing.T) {
	setupHome(t)
	config.Save(&config.Config{Token: "existing-token"})

	cmd := NewCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	ok, err := auth.IsLoggedIn()
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("expected logged out after logout command")
	}
}

func TestLogoutWhenNotLoggedIn(t *testing.T) {
	setupHome(t)

	cmd := NewCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}

func TestNewCmdReturnsCommand(t *testing.T) {
	cmd := NewCmd()
	if cmd.Use != "logout" {
		t.Errorf("Use = %q, want %q", cmd.Use, "logout")
	}
	if cmd.Short == "" {
		t.Error("expected non-empty Short description")
	}
}
