package authcmd

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/auth"
)

type mockServer struct {
	startErr error
	token    string
	waitErr  error
}

func (m *mockServer) Start() error                                       { return m.startErr }
func (m *mockServer) WaitForResult(ctx context.Context) (string, error)  { return m.token, m.waitErr }
func (m *mockServer) Close() error                                       { return nil }

func setMockDeps(d *Deps) func() {
	old := deps
	deps = d
	return func() { deps = old }
}

func TestNewCmdReturnsAuthCommand(t *testing.T) {
	cmd := NewCmd()
	if cmd.Use != "auth" {
		t.Errorf("Use = %q, want %q", cmd.Use, "auth")
	}
	if cmd.Short == "" {
		t.Error("expected non-empty Short description")
	}

	// Check subcommands exist
	subCmds := cmd.Commands()
	names := make(map[string]bool)
	for _, c := range subCmds {
		names[c.Use] = true
	}
	for _, name := range []string{"login", "logout", "status", "keys"} {
		if !names[name] {
			t.Errorf("missing subcommand %q", name)
		}
	}
}

func TestAuthLoginAlreadyLoggedIn(t *testing.T) {
	restore := setMockDeps(&Deps{
		IsLoggedIn: func() (bool, error) { return true, nil },
	})
	defer restore()

	cmd := NewCmd()
	cmd.SetArgs([]string{"login"})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}

func TestAuthLoginSuccessfulFlow(t *testing.T) {
	var savedToken string
	restore := setMockDeps(&Deps{
		IsLoggedIn:    func() (bool, error) { return false, nil },
		GenerateState: func() (string, error) { return "test-state", nil },
		NewServer: func(port int, state string) auth.Server {
			return &mockServer{token: "oauth-tok-123"}
		},
		OpenBrowser: func(url string) error { return nil },
		SaveToken:   func(token string) error { savedToken = token; return nil },
	})
	defer restore()

	cmd := NewCmd()
	cmd.SetArgs([]string{"login"})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if savedToken != "oauth-tok-123" {
		t.Errorf("saved token = %q, want %q", savedToken, "oauth-tok-123")
	}
}

func TestAuthLoginIsLoggedInError(t *testing.T) {
	restore := setMockDeps(&Deps{
		IsLoggedIn: func() (bool, error) { return false, fmt.Errorf("disk error") },
	})
	defer restore()

	cmd := NewCmd()
	cmd.SetArgs([]string{"login"})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAuthLogout(t *testing.T) {
	cleared := false
	restore := setMockDeps(&Deps{
		ClearToken: func() error { cleared = true; return nil },
	})
	defer restore()

	cmd := NewCmd()
	cmd.SetArgs([]string{"logout"})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !cleared {
		t.Error("token was not cleared")
	}
}

func TestAuthLogoutError(t *testing.T) {
	restore := setMockDeps(&Deps{
		ClearToken: func() error { return fmt.Errorf("permission denied") },
	})
	defer restore()

	cmd := NewCmd()
	cmd.SetArgs([]string{"logout"})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAuthStatusLoggedIn(t *testing.T) {
	restore := setMockDeps(&Deps{
		IsLoggedIn:     func() (bool, error) { return true, nil },
		GetTokenSource: func() string { return "config file" },
	})
	defer restore()

	cmd := NewCmd()
	cmd.SetArgs([]string{"status"})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}

func TestAuthStatusNotLoggedIn(t *testing.T) {
	restore := setMockDeps(&Deps{
		IsLoggedIn: func() (bool, error) { return false, nil },
	})
	defer restore()

	cmd := NewCmd()
	cmd.SetArgs([]string{"status"})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}

func TestAuthStatusError(t *testing.T) {
	restore := setMockDeps(&Deps{
		IsLoggedIn: func() (bool, error) { return false, fmt.Errorf("read error") },
	})
	defer restore()

	cmd := NewCmd()
	cmd.SetArgs([]string{"status"})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAuthKeysNotLoggedIn(t *testing.T) {
	restore := setMockDeps(&Deps{
		GetToken: func() (string, error) { return "", nil },
	})
	defer restore()

	cmd := NewCmd()
	cmd.SetArgs([]string{"keys"})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for not logged in")
	}
}

func TestAuthKeysSuccess(t *testing.T) {
	restore := setMockDeps(&Deps{
		GetToken: func() (string, error) { return "test-token", nil },
		ListKeys: func(token string) ([]api.APIKey, error) {
			return []api.APIKey{
				{
					ID:        "key-1",
					Name:      "My Key",
					Prefix:    "hk_abc",
					CreatedAt: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
				},
			}, nil
		},
	})
	defer restore()

	cmd := NewCmd()
	cmd.SetArgs([]string{"keys"})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}

func TestAuthKeysEmpty(t *testing.T) {
	restore := setMockDeps(&Deps{
		GetToken: func() (string, error) { return "test-token", nil },
		ListKeys: func(token string) ([]api.APIKey, error) {
			return []api.APIKey{}, nil
		},
	})
	defer restore()

	cmd := NewCmd()
	cmd.SetArgs([]string{"keys"})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}

func TestAuthKeysAPIError(t *testing.T) {
	restore := setMockDeps(&Deps{
		GetToken: func() (string, error) { return "test-token", nil },
		ListKeys: func(token string) ([]api.APIKey, error) {
			return nil, fmt.Errorf("API error")
		},
	})
	defer restore()

	cmd := NewCmd()
	cmd.SetArgs([]string{"keys"})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAuthKeysGetTokenError(t *testing.T) {
	restore := setMockDeps(&Deps{
		GetToken: func() (string, error) { return "", fmt.Errorf("config error") },
	})
	defer restore()

	cmd := NewCmd()
	cmd.SetArgs([]string{"keys"})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
}
