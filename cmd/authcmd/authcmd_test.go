package authcmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/auth"
	detect "github.com/EscapeVelocityOperations/hatch-cli/internal/ci"
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

// h-nd8a: on a terminal, `hatch auth keys create` mints a token and prints it
// EXACTLY ONCE with the CI storage instruction (the plaintext can never be
// retrieved again). The human message and the token go to the same screen.
func TestAuthKeysCreate_Terminal_PrintsTokenOnceWithInstruction(t *testing.T) {
	var gotToken, gotName string
	calls := 0
	restore := setMockDeps(&Deps{
		GetToken:   func() (string, error) { return "test-token", nil },
		IsTerminal: func(io.Writer) bool { return true },
		CreateKey: func(token, name string) (string, error) {
			calls++
			gotToken, gotName = token, name
			return "hatch_ci_secret_xyz", nil
		},
	})
	defer restore()

	cmd := NewCmd()
	cmd.SetArgs([]string{"keys", "create", "--name", "ci-acme"})
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	out := stdout.String()
	if calls != 1 {
		t.Errorf("CreateKey called %d times, want 1", calls)
	}
	if gotName != "ci-acme" {
		t.Errorf("name = %q, want ci-acme", gotName)
	}
	if gotToken != "test-token" {
		t.Errorf("auth token = %q, want test-token", gotToken)
	}
	if n := strings.Count(out+stderr.String(), "hatch_ci_secret_xyz"); n != 1 {
		t.Errorf("token printed %d times, want exactly 1\nstdout:\n%s\nstderr:\n%s", n, out, stderr.String())
	}
	if !strings.Contains(out, "Created deploy token") || !strings.Contains(out, "HATCH_TOKEN") || !strings.Contains(out, "cannot be retrieved later") {
		t.Errorf("missing CI storage instruction on stdout:\n%s", out)
	}
}

// h-db2jl B1: when stdout is NOT a terminal (pipe, redirect, $(…)), stdout
// carries the token ALONE — nothing else — and every human message goes to
// stderr, so `hatch auth keys create | gh secret set HATCH_TOKEN` stores the
// token and not a message line.
func TestAuthKeysCreate_NotTerminal_StdoutIsTheTokenAlone(t *testing.T) {
	for name, isTerm := range map[string]func(io.Writer) bool{
		"explicit non-terminal": func(io.Writer) bool { return false },
		"no detector (default)": nil,
	} {
		t.Run(name, func(t *testing.T) {
			restore := setMockDeps(&Deps{
				GetToken:   func() (string, error) { return "test-token", nil },
				IsTerminal: isTerm,
				CreateKey:  func(string, string) (string, error) { return "hatch_ci_secret_xyz", nil },
			})
			defer restore()

			cmd := NewCmd()
			cmd.SetArgs([]string{"keys", "create", "--name", "ci-acme"})
			stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
			cmd.SetOut(stdout)
			cmd.SetErr(stderr)
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}

			if got := stdout.String(); got != "hatch_ci_secret_xyz\n" {
				t.Errorf("stdout = %q, want the token and a newline only", got)
			}
			msg := stderr.String()
			if strings.Contains(msg, "hatch_ci_secret_xyz") {
				t.Errorf("token must not be repeated on stderr:\n%s", msg)
			}
			for _, want := range []string{"Created deploy token", "ci-acme", "HATCH_TOKEN", "cannot be retrieved later"} {
				if !strings.Contains(msg, want) {
					t.Errorf("stderr missing %q:\n%s", want, msg)
				}
			}
		})
	}
}

// h-db2jl B1, end to end through a REAL pipe: the real command writes to an
// os.Pipe (so the real terminal detection sees a non-terminal), and the
// consumer side that `hatch ci --set-secret` uses reads the minted token back —
// not the "Created deploy token …" line.
func TestAuthKeysCreate_RealPipe_ConsumerGetsTheToken(t *testing.T) {
	const minted = "hatch_0123456789abcdef0123456789abcdef"
	restore := setMockDeps(&Deps{
		GetToken:  func() (string, error) { return "test-token", nil },
		CreateKey: func(string, string) (string, error) { return minted, nil },
	})
	defer restore()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	cmd := NewCmd()
	cmd.SetArgs([]string{"keys", "create", "--name", "ci-acme"})
	cmd.SetOut(w) // an *os.File on a pipe: what the shell hands us in `keys create | …`
	stderr := new(bytes.Buffer)
	cmd.SetErr(stderr)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	w.Close()

	got, err := detect.ReadTokenFrom(r)
	if err != nil {
		t.Fatalf("consumer could not read the piped token: %v", err)
	}
	if got != minted {
		t.Errorf("consumer read %q, want the minted token", got)
	}
	if !strings.Contains(stderr.String(), "Created deploy token") {
		t.Errorf("human message must be on stderr:\n%s", stderr.String())
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
