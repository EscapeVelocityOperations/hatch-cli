package authcmd

import (
	"bytes"
	"context"
	"fmt"
	"strings"
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

// h-nd8a: `hatch auth keys create` mints a token and prints it EXACTLY ONCE with
// the CI storage instruction (the plaintext can never be retrieved again).
func TestAuthKeysCreate_PrintsTokenOnceWithInstruction(t *testing.T) {
	var gotToken, gotName string
	calls := 0
	restore := setMockDeps(&Deps{
		GetToken: func() (string, error) { return "test-token", nil },
		CreateKey: func(token, name string) (string, error) {
			calls++
			gotToken, gotName = token, name
			return "hatch_ci_secret_xyz", nil
		},
	})
	defer restore()

	cmd := NewCmd()
	cmd.SetArgs([]string{"keys", "create", "--name", "ci-acme"})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if calls != 1 {
		t.Errorf("CreateKey called %d times, want 1", calls)
	}
	if gotName != "ci-acme" {
		t.Errorf("name = %q, want ci-acme", gotName)
	}
	if gotToken != "test-token" {
		t.Errorf("auth token = %q, want test-token", gotToken)
	}
	if n := strings.Count(out, "hatch_ci_secret_xyz"); n != 1 {
		t.Errorf("token printed %d times, want exactly 1\noutput:\n%s", n, out)
	}
	if !strings.Contains(out, "HATCH_TOKEN") || !strings.Contains(out, "cannot be retrieved later") {
		t.Errorf("missing CI storage instruction in output:\n%s", out)
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
