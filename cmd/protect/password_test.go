package protect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// mockPasswordAPIClient implements PasswordAPIClient (cmd/webhook mock pattern).
type mockPasswordAPIClient struct {
	setFn    func(slug, password string) (*PasswordProtection, error)
	getFn    func(slug string) (*PasswordProtection, error)
	deleteFn func(slug string) error

	lastSetPassword string
	setCalled       bool
	deleteCalled    bool
}

func (m *mockPasswordAPIClient) SetPasswordProtection(slug, password string) (*PasswordProtection, error) {
	m.setCalled = true
	m.lastSetPassword = password
	if m.setFn != nil {
		return m.setFn(slug, password)
	}
	return &PasswordProtection{Protected: true}, nil
}

func (m *mockPasswordAPIClient) GetPasswordProtection(slug string) (*PasswordProtection, error) {
	if m.getFn != nil {
		return m.getFn(slug)
	}
	return &PasswordProtection{}, nil
}

func (m *mockPasswordAPIClient) DeletePasswordProtection(slug string) error {
	m.deleteCalled = true
	if m.deleteFn != nil {
		return m.deleteFn(slug)
	}
	return nil
}

// withTestPasswordDeps wires passwordDeps to a logged-in user inside a tmp app
// dir whose .hatch.toml resolves to slug my-app (cmd/webhook withTestDeps
// pattern, mirrors withTestEmailDeps).
func withTestPasswordDeps(t *testing.T, mock *mockPasswordAPIClient) {
	t.Helper()
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, ".hatch.toml"),
		[]byte("slug = \"my-app\"\nname = \"my-app\"\n"), 0o644); err != nil {
		t.Fatalf("setup .hatch.toml: %v", err)
	}
	passwordDeps = &PasswordDeps{
		GetToken:     func() (string, error) { return "tok123", nil },
		GetCwd:       func() (string, error) { return tmp, nil },
		NewAPIClient: func(token string) PasswordAPIClient { return mock },
	}
	t.Cleanup(func() { passwordDeps = defaultPasswordDeps() })
}

func protectCmdWithFlags() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("password", "", "")
	cmd.Flags().Bool("off", false, "")
	return cmd
}

func TestRunProtectDisable_CallsClear(t *testing.T) {
	mock := &mockPasswordAPIClient{}
	withTestPasswordDeps(t, mock)

	cmd := protectCmdWithFlags()
	_ = cmd.Flags().Set("off", "true")

	out, err := captureStdout(func() error { return runProtect(cmd, nil) })
	if err != nil {
		t.Fatalf("runProtect: %v", err)
	}
	if mock.setCalled {
		t.Error("expected SetPasswordProtection NOT to be called on --off")
	}
	if !mock.deleteCalled {
		t.Fatal("expected DeletePasswordProtection to be called")
	}
	if !strings.Contains(out, "my-app") || !strings.Contains(out, "disabled") {
		t.Errorf("output = %q, want a disabled confirmation mentioning the app slug", out)
	}
}

func TestRunProtectEnable_PostsPassword(t *testing.T) {
	mock := &mockPasswordAPIClient{}
	withTestPasswordDeps(t, mock)

	cmd := protectCmdWithFlags()
	_ = cmd.Flags().Set("password", "hunter2")

	out, err := captureStdout(func() error { return runProtect(cmd, nil) })
	if err != nil {
		t.Fatalf("runProtect: %v", err)
	}
	if !mock.setCalled {
		t.Fatal("expected SetPasswordProtection to be called")
	}
	if mock.lastSetPassword != "hunter2" {
		t.Errorf("password posted = %q, want hunter2", mock.lastSetPassword)
	}
	if !strings.Contains(out, "my-app") {
		t.Errorf("output = %q, want confirmation mentioning the app slug", out)
	}
	if !strings.Contains(out, "auth-gateway") {
		t.Errorf("output = %q, want a mention of the enforcing auth-gateway (honest-output requirement)", out)
	}
	if strings.Contains(out, "hunter2") {
		t.Errorf("output = %q, must never echo the stored password back", out)
	}
}
