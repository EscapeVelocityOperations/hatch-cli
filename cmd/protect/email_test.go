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

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !bytes.Contains([]byte(s), []byte(sub)) {
			return false
		}
	}
	return true
}
