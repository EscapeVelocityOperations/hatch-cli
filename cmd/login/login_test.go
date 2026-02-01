package login

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/auth"
)

type mockServer struct {
	startErr error
	token    string
	waitErr  error
}

func (m *mockServer) Start() error                                 { return m.startErr }
func (m *mockServer) WaitForResult(ctx context.Context) (string, error) { return m.token, m.waitErr }
func (m *mockServer) Close() error                                 { return nil }

func setMockDeps(d *Deps) func() {
	old := deps
	deps = d
	return func() { deps = old }
}

func TestLoginAlreadyLoggedIn(t *testing.T) {
	restore := setMockDeps(&Deps{
		IsLoggedIn: func() (bool, error) { return true, nil },
	})
	defer restore()

	cmd := NewCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}

func TestLoginSuccessfulFlow(t *testing.T) {
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

func TestLoginIsLoggedInError(t *testing.T) {
	restore := setMockDeps(&Deps{
		IsLoggedIn: func() (bool, error) { return false, fmt.Errorf("disk error") },
	})
	defer restore()

	cmd := NewCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoginGenerateStateError(t *testing.T) {
	restore := setMockDeps(&Deps{
		IsLoggedIn:    func() (bool, error) { return false, nil },
		GenerateState: func() (string, error) { return "", fmt.Errorf("entropy fail") },
	})
	defer restore()

	cmd := NewCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoginServerStartError(t *testing.T) {
	restore := setMockDeps(&Deps{
		IsLoggedIn:    func() (bool, error) { return false, nil },
		GenerateState: func() (string, error) { return "s", nil },
		NewServer: func(port int, state string) auth.Server {
			return &mockServer{startErr: fmt.Errorf("port in use")}
		},
	})
	defer restore()

	cmd := NewCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoginBrowserOpenFails(t *testing.T) {
	var savedToken string
	restore := setMockDeps(&Deps{
		IsLoggedIn:    func() (bool, error) { return false, nil },
		GenerateState: func() (string, error) { return "s", nil },
		NewServer: func(port int, state string) auth.Server {
			return &mockServer{token: "tok"}
		},
		OpenBrowser: func(url string) error { return fmt.Errorf("no display") },
		SaveToken:   func(token string) error { savedToken = token; return nil },
	})
	defer restore()

	cmd := NewCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if savedToken != "tok" {
		t.Errorf("saved token = %q, want %q", savedToken, "tok")
	}
}

func TestLoginWaitError(t *testing.T) {
	restore := setMockDeps(&Deps{
		IsLoggedIn:    func() (bool, error) { return false, nil },
		GenerateState: func() (string, error) { return "s", nil },
		NewServer: func(port int, state string) auth.Server {
			return &mockServer{waitErr: fmt.Errorf("timeout")}
		},
		OpenBrowser: func(url string) error { return nil },
	})
	defer restore()

	cmd := NewCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoginSaveTokenError(t *testing.T) {
	restore := setMockDeps(&Deps{
		IsLoggedIn:    func() (bool, error) { return false, nil },
		GenerateState: func() (string, error) { return "s", nil },
		NewServer: func(port int, state string) auth.Server {
			return &mockServer{token: "tok"}
		},
		OpenBrowser: func(url string) error { return nil },
		SaveToken:   func(token string) error { return fmt.Errorf("write fail") },
	})
	defer restore()

	cmd := NewCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNewCmdReturnsCommand(t *testing.T) {
	cmd := NewCmd()
	if cmd.Use != "login" {
		t.Errorf("Use = %q, want %q", cmd.Use, "login")
	}
	if cmd.Short == "" {
		t.Error("expected non-empty Short description")
	}
}
