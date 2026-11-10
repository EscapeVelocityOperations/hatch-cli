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
	lastSlug        string
	setCalled       bool
	deleteCalled    bool
}

func (m *mockPasswordAPIClient) SetPasswordProtection(slug, password string) (*PasswordProtection, error) {
	m.setCalled = true
	m.lastSetPassword = password
	m.lastSlug = slug
	if m.setFn != nil {
		return m.setFn(slug, password)
	}
	return &PasswordProtection{Protected: true}, nil
}

func (m *mockPasswordAPIClient) GetPasswordProtection(slug string) (*PasswordProtection, error) {
	m.lastSlug = slug
	if m.getFn != nil {
		return m.getFn(slug)
	}
	return &PasswordProtection{}, nil
}

func (m *mockPasswordAPIClient) DeletePasswordProtection(slug string) error {
	m.deleteCalled = true
	m.lastSlug = slug
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

func TestRunProtect_WhitespacePassword(t *testing.T) {
	mock := &mockPasswordAPIClient{}
	withTestPasswordDeps(t, mock)

	cmd := protectCmdWithFlags()
	_ = cmd.Flags().Set("password", "   ")

	if err := runProtect(cmd, nil); err == nil {
		t.Fatal("expected an error for a whitespace-only --password")
	}
	if mock.setCalled {
		t.Error("expected no API call for a whitespace-only password")
	}
}

// executeHatchProtect runs `hatch <args...>` through a real parent root with
// the protect command mounted, the way main wires it. Calling runProtect
// directly (or executing NewCmd() standalone, which makes it the root) cannot
// catch positionals being silently dropped: cobra only rejects unknown args
// on the root command (h-abmr F1). newProtectCmd is the same command NewCmd
// returns minus the email subtree, which can only be mounted once per process.
func executeHatchProtect(args ...string) (string, error) {
	root := &cobra.Command{Use: "hatch", SilenceUsage: true, SilenceErrors: true}
	root.AddCommand(newProtectCmd())
	root.SetArgs(args)
	return captureStdout(root.Execute)
}

// withCwdOutsideApp points GetCwd at a directory with no .hatch.toml.
func withCwdOutsideApp(t *testing.T) {
	t.Helper()
	empty := t.TempDir()
	passwordDeps.GetCwd = func() (string, error) { return empty, nil }
}

func TestProtectViaRoot_PositionalSlugTargetsThatEgg(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantSet    bool
		wantDelete bool
	}{
		{"enable", []string{"protect", "other-app", "--password", "hunter2"}, true, false},
		{"off", []string{"protect", "other-app", "--off"}, false, true},
		{"status", []string{"protect", "other-app"}, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mock := &mockPasswordAPIClient{}
			// cwd is my-app: the positional must win over .hatch.toml.
			withTestPasswordDeps(t, mock)

			out, err := executeHatchProtect(tc.args...)
			if err != nil {
				t.Fatalf("hatch %v: %v", tc.args, err)
			}
			if mock.lastSlug != "other-app" {
				t.Errorf("API targeted slug %q, want other-app (must not fall back to cwd my-app)", mock.lastSlug)
			}
			if mock.setCalled != tc.wantSet || mock.deleteCalled != tc.wantDelete {
				t.Errorf("set=%v delete=%v, want set=%v delete=%v",
					mock.setCalled, mock.deleteCalled, tc.wantSet, tc.wantDelete)
			}
			if !strings.Contains(out, "other-app") || strings.Contains(out, "my-app") {
				t.Errorf("output = %q, want it to name other-app only", out)
			}
		})
	}
}

func TestProtectViaRoot_NoPositionalUsesCwdApp(t *testing.T) {
	for _, args := range [][]string{
		{"protect", "--password", "hunter2"},
		{"protect", "--off"},
	} {
		mock := &mockPasswordAPIClient{}
		withTestPasswordDeps(t, mock)

		if _, err := executeHatchProtect(args...); err != nil {
			t.Fatalf("hatch %v: %v", args, err)
		}
		if mock.lastSlug != "my-app" {
			t.Errorf("hatch %v targeted slug %q, want cwd app my-app", args, mock.lastSlug)
		}
	}
}

func TestProtectViaRoot_NoSlugNoAppErrors(t *testing.T) {
	for _, args := range [][]string{
		{"protect", "--password", "hunter2"},
		{"protect", "--off"},
		{"protect"},
	} {
		mock := &mockPasswordAPIClient{}
		withTestPasswordDeps(t, mock)
		withCwdOutsideApp(t)

		_, err := executeHatchProtect(args...)
		if err == nil {
			t.Fatalf("hatch %v: expected an error with no slug and no .hatch.toml", args)
		}
		if !strings.Contains(err.Error(), "no app specified") {
			t.Errorf("hatch %v: error = %q, want a clear no-app error", args, err)
		}
		if mock.setCalled || mock.deleteCalled {
			t.Errorf("hatch %v: expected no mutation without a target", args)
		}
	}
}

func TestProtectViaRoot_TooManyArgsIsUsageError(t *testing.T) {
	mock := &mockPasswordAPIClient{}
	withTestPasswordDeps(t, mock)

	_, err := executeHatchProtect("protect", "a", "b", "--off")
	if err == nil {
		t.Fatal("expected a usage error for more than one positional")
	}
	if mock.setCalled || mock.deleteCalled {
		t.Error("expected no API call on a usage error")
	}
}

func TestProtectViaRoot_EmailSubcommandStillRoutes(t *testing.T) {
	// MaximumNArgs(1) must not turn the `email` verb into a slug. The only
	// NewCmd() call in this package's tests (email subtree mounts once).
	cmd := NewCmd()
	found, rest, err := cmd.Find([]string{"email"})
	if err != nil {
		t.Fatalf("Find(email): %v", err)
	}
	if found.Name() != "email" || len(rest) != 0 {
		t.Errorf("protect email resolved to %q with args %v, want the email subcommand", found.Name(), rest)
	}
}
