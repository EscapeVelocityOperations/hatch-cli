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

func TestRunProtectStatus_Protected(t *testing.T) {
	mock := &mockPasswordAPIClient{
		getFn: func(slug string) (*PasswordProtection, error) {
			return &PasswordProtection{Protected: true}, nil
		},
	}
	withTestPasswordDeps(t, mock)

	cmd := protectCmdWithFlags()

	out, err := captureStdout(func() error { return runProtect(cmd, nil) })
	if err != nil {
		t.Fatalf("runProtect: %v", err)
	}
	if mock.setCalled || mock.deleteCalled {
		t.Error("bare status call must not mutate protection state")
	}
	if !strings.Contains(out, "my-app") || !strings.Contains(strings.ToLower(out), "protected") {
		t.Errorf("output = %q, want a status line mentioning the app slug + protected", out)
	}
}

func TestRunProtectStatus_Unprotected(t *testing.T) {
	mock := &mockPasswordAPIClient{
		getFn: func(slug string) (*PasswordProtection, error) {
			return &PasswordProtection{Protected: false}, nil
		},
	}
	withTestPasswordDeps(t, mock)

	cmd := protectCmdWithFlags()

	out, err := captureStdout(func() error { return runProtect(cmd, nil) })
	if err != nil {
		t.Fatalf("runProtect: %v", err)
	}
	if !strings.Contains(out, "my-app") || !strings.Contains(strings.ToLower(out), "not") {
		t.Errorf("output = %q, want a status line indicating NOT protected", out)
	}
}

func TestRunProtect_MutuallyExclusiveFlags(t *testing.T) {
	mock := &mockPasswordAPIClient{}
	withTestPasswordDeps(t, mock)

	cmd := protectCmdWithFlags()
	_ = cmd.Flags().Set("password", "hunter2")
	_ = cmd.Flags().Set("off", "true")

	err := runProtect(cmd, nil)
	if err == nil {
		t.Fatal("expected an error when --password and --off are both set")
	}
	if mock.setCalled || mock.deleteCalled {
		t.Error("expected no API call when flags conflict")
	}
}

func TestRunProtect_EmptyPassword(t *testing.T) {
	mock := &mockPasswordAPIClient{}
	withTestPasswordDeps(t, mock)

	cmd := protectCmdWithFlags()
	_ = cmd.Flags().Set("password", "")

	err := runProtect(cmd, nil)
	if err == nil {
		t.Fatal("expected an error for an explicitly empty --password")
	}
	if mock.setCalled {
		t.Error("expected no API call for an empty password")
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
	if !strings.Contains(out, "Password protection enabled") {
		t.Errorf("output = %q, want a factual enablement confirmation", out)
	}
	// h-macc rework: the enforcement layer has open P0 bypass fixes
	// (h-7lbm PR#79, h-wvzu PR#81) — the CLI must not assert reliable
	// enforcement it cannot guarantee. "auth-gateway" may still appear
	// as a description of intent (who prompts for the password), but
	// never as an enforcement guarantee.
	if strings.Contains(out, "enforced by") {
		t.Errorf("output = %q, must not claim enforcement while known bypasses are open (h-abmr rework)", out)
	}
	if strings.Contains(out, "hunter2") {
		t.Errorf("output = %q, must never echo the stored password back", out)
	}
}
