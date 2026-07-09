package protect

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

// mockEmailAPIClient implements EmailAPIClient (cmd/webhook mock pattern).
type mockEmailAPIClient struct {
	setFn    func(slug string, emails, domains []string) (*EmailProtection, error)
	getFn    func(slug string) (*EmailProtection, error)
	deleteFn func(slug string) error

	lastSetEmails  []string
	lastSetDomains []string
	deleteCalled   bool
}

func (m *mockEmailAPIClient) SetEmailProtection(slug string, emails, domains []string) (*EmailProtection, error) {
	m.lastSetEmails, m.lastSetDomains = emails, domains
	if m.setFn != nil {
		return m.setFn(slug, emails, domains)
	}
	return &EmailProtection{Enabled: true, Emails: emails, Domains: domains}, nil
}

func (m *mockEmailAPIClient) GetEmailProtection(slug string) (*EmailProtection, error) {
	if m.getFn != nil {
		return m.getFn(slug)
	}
	return &EmailProtection{}, nil
}

func (m *mockEmailAPIClient) DeleteEmailProtection(slug string) error {
	m.deleteCalled = true
	if m.deleteFn != nil {
		return m.deleteFn(slug)
	}
	return nil
}

// withTestEmailDeps wires emailDeps to a logged-in user inside a tmp app dir
// whose .hatch.toml resolves to slug my-app (cmd/webhook withTestDeps pattern).
func withTestEmailDeps(t *testing.T, mock *mockEmailAPIClient) {
	t.Helper()
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, ".hatch.toml"),
		[]byte("slug = \"my-app\"\nname = \"my-app\"\n"), 0o644); err != nil {
		t.Fatalf("setup .hatch.toml: %v", err)
	}
	emailDeps = &EmailDeps{
		GetToken:     func() (string, error) { return "tok123", nil },
		GetCwd:       func() (string, error) { return tmp, nil },
		NewAPIClient: func(token string) EmailAPIClient { return mock },
	}
	t.Cleanup(func() { emailDeps = defaultEmailDeps() })
}

func captureStdout(fn func() error) (string, error) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	runErr := fn()

	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String(), runErr
}

// captureStdoutAndStderr mirrors captureStdout but also captures stderr —
// the mailer_configured warning (T-005) is deliberately kept off the
// parseable stdout payload, so tests need to assert on both streams.
func captureStdoutAndStderr(fn func() error) (stdout, stderr string, err error) {
	oldOut, oldErr := os.Stdout, os.Stderr
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout, os.Stderr = wOut, wErr

	runErr := fn()

	wOut.Close()
	wErr.Close()
	os.Stdout, os.Stderr = oldOut, oldErr

	var bufOut, bufErr bytes.Buffer
	_, _ = io.Copy(&bufOut, rOut)
	_, _ = io.Copy(&bufErr, rErr)
	return bufOut.String(), bufErr.String(), runErr
}

func TestRunEmailEnable_PostsNormalizedLists(t *testing.T) {
	mock := &mockEmailAPIClient{}
	withTestEmailDeps(t, mock)

	cmd := &cobra.Command{}
	cmd.Flags().StringSlice("email", nil, "")
	cmd.Flags().StringSlice("domain", nil, "")
	_ = cmd.Flags().Set("email", "a@b.com")
	_ = cmd.Flags().Set("domain", "corp.com")

	out, err := captureStdout(func() error { return runEmailEnable(cmd, nil) })
	if err != nil {
		t.Fatalf("runEmailEnable: %v", err)
	}
	if len(mock.lastSetEmails) != 1 || mock.lastSetEmails[0] != "a@b.com" {
		t.Errorf("emails posted = %v, want [a@b.com]", mock.lastSetEmails)
	}
	if len(mock.lastSetDomains) != 1 || mock.lastSetDomains[0] != "corp.com" {
		t.Errorf("domains posted = %v, want [corp.com]", mock.lastSetDomains)
	}
	if out == "" {
		t.Error("expected confirmation output")
	}
}

// TestRunEmailEnable_NormalizesCaseAndWhitespace (h-vo8d rework, MEDIUM):
// enable's --email/--domain flags get the same trim+lowercase treatment as
// add/remove before being sent, so what the CLI echoes back matches what the
// server will actually store.
func TestRunEmailEnable_NormalizesCaseAndWhitespace(t *testing.T) {
	mock := &mockEmailAPIClient{}
	withTestEmailDeps(t, mock)

	cmd := &cobra.Command{}
	cmd.Flags().StringSlice("email", nil, "")
	cmd.Flags().StringSlice("domain", nil, "")
	_ = cmd.Flags().Set("email", " Admin@Corp.com ")
	_ = cmd.Flags().Set("domain", "@Corp.com")

	if _, err := captureStdout(func() error { return runEmailEnable(cmd, nil) }); err != nil {
		t.Fatalf("runEmailEnable: %v", err)
	}
	if len(mock.lastSetEmails) != 1 || mock.lastSetEmails[0] != "admin@corp.com" {
		t.Errorf("emails posted = %v, want [admin@corp.com] (trimmed + lowercased)", mock.lastSetEmails)
	}
	if len(mock.lastSetDomains) != 1 || mock.lastSetDomains[0] != "corp.com" {
		t.Errorf("domains posted = %v, want [corp.com] (lowercased, @ stripped)", mock.lastSetDomains)
	}
}

func TestRunEmailEnable_NoListsErrors(t *testing.T) {
	mock := &mockEmailAPIClient{}
	withTestEmailDeps(t, mock)

	cmd := &cobra.Command{}
	cmd.Flags().StringSlice("email", nil, "")
	cmd.Flags().StringSlice("domain", nil, "")

	if err := runEmailEnable(cmd, nil); err == nil {
		t.Error("want an error when neither --email nor --domain is given")
	}
}

func TestRunEmailDisable_CallsDelete(t *testing.T) {
	mock := &mockEmailAPIClient{}
	withTestEmailDeps(t, mock)

	if _, err := captureStdout(func() error { return runEmailDisable(&cobra.Command{}, nil) }); err != nil {
		t.Fatalf("runEmailDisable: %v", err)
	}
	if !mock.deleteCalled {
		t.Error("runEmailDisable did not call DeleteEmailProtection")
	}
}

func TestRunEmailList_RendersEmailsAndDomains(t *testing.T) {
	mock := &mockEmailAPIClient{
		getFn: func(slug string) (*EmailProtection, error) {
			return &EmailProtection{Enabled: true, Emails: []string{"a@b.com"}, Domains: []string{"corp.com"}}, nil
		},
	}
	withTestEmailDeps(t, mock)

	out, err := captureStdout(func() error { return runEmailList(&cobra.Command{}, nil) })
	if err != nil {
		t.Fatalf("runEmailList: %v", err)
	}
	if !containsAll(out, "a@b.com", "corp.com") {
		t.Errorf("list output = %q, want it to mention both a@b.com and corp.com", out)
	}
}

func TestRunEmailList_DisabledSaysSo(t *testing.T) {
	mock := &mockEmailAPIClient{getFn: func(slug string) (*EmailProtection, error) {
		return &EmailProtection{Enabled: false}, nil
	}}
	withTestEmailDeps(t, mock)

	out, err := captureStdout(func() error { return runEmailList(&cobra.Command{}, nil) })
	if err != nil {
		t.Fatalf("runEmailList: %v", err)
	}
	if !containsAll(out, "disabled") {
		t.Errorf("list output = %q, want it to say protection is disabled", out)
	}
}

// TestRunEmailList_WarnsWhenMailerNotConfigured / TestRunEmailEnable_...
// (h-7b9l T-004): an enabled-but-unconfigured mailer silently locks out
// every visitor (no sign-in link can ever be sent) — list/enable must warn
// on stderr, and the warning must never land in stdout's parseable payload.
func TestRunEmailList_WarnsWhenMailerNotConfigured(t *testing.T) {
	mock := &mockEmailAPIClient{
		getFn: func(slug string) (*EmailProtection, error) {
			return &EmailProtection{Enabled: true, Emails: []string{"a@b.com"}, MailerConfigured: false}, nil
		},
	}
	withTestEmailDeps(t, mock)

	stdout, stderr, err := captureStdoutAndStderr(func() error { return runEmailList(&cobra.Command{}, nil) })
	if err != nil {
		t.Fatalf("runEmailList: %v", err)
	}
	if !containsAll(stderr, "warning", "not configured") {
		t.Errorf("stderr = %q, want a mailer-not-configured warning", stderr)
	}
	if bytes.Contains([]byte(stdout), []byte("warning")) {
		t.Errorf("stdout = %q, warning must not appear in the parseable payload", stdout)
	}
}

func TestRunEmailList_NoWarningWhenMailerConfigured(t *testing.T) {
	mock := &mockEmailAPIClient{
		getFn: func(slug string) (*EmailProtection, error) {
			return &EmailProtection{Enabled: true, Emails: []string{"a@b.com"}, MailerConfigured: true}, nil
		},
	}
	withTestEmailDeps(t, mock)

	_, stderr, err := captureStdoutAndStderr(func() error { return runEmailList(&cobra.Command{}, nil) })
	if err != nil {
		t.Fatalf("runEmailList: %v", err)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want no warning when mailer is configured", stderr)
	}
}

func TestRunEmailEnable_WarnsWhenMailerNotConfigured(t *testing.T) {
	mock := &mockEmailAPIClient{
		setFn: func(slug string, emails, domains []string) (*EmailProtection, error) {
			return &EmailProtection{Enabled: true, Emails: emails, Domains: domains, MailerConfigured: false}, nil
		},
	}
	withTestEmailDeps(t, mock)

	cmd := &cobra.Command{}
	cmd.Flags().StringSlice("email", nil, "")
	cmd.Flags().StringSlice("domain", nil, "")
	_ = cmd.Flags().Set("email", "a@b.com")

	_, stderr, err := captureStdoutAndStderr(func() error { return runEmailEnable(cmd, nil) })
	if err != nil {
		t.Fatalf("runEmailEnable: %v", err)
	}
	if !containsAll(stderr, "warning", "not configured") {
		t.Errorf("stderr = %q, want a mailer-not-configured warning", stderr)
	}
}

func TestRunEmailEnable_NoWarningWhenMailerConfigured(t *testing.T) {
	mock := &mockEmailAPIClient{
		setFn: func(slug string, emails, domains []string) (*EmailProtection, error) {
			return &EmailProtection{Enabled: true, Emails: emails, Domains: domains, MailerConfigured: true}, nil
		},
	}
	withTestEmailDeps(t, mock)

	cmd := &cobra.Command{}
	cmd.Flags().StringSlice("email", nil, "")
	cmd.Flags().StringSlice("domain", nil, "")
	_ = cmd.Flags().Set("email", "a@b.com")

	_, stderr, err := captureStdoutAndStderr(func() error { return runEmailEnable(cmd, nil) })
	if err != nil {
		t.Fatalf("runEmailEnable: %v", err)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want no warning when mailer is configured", stderr)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !bytes.Contains([]byte(s), []byte(sub)) {
			return false
		}
	}
	return true
}
