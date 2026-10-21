package protect

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func enableCmdWith(t *testing.T, ttl string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.Flags().StringSlice("email", nil, "")
	cmd.Flags().StringSlice("domain", nil, "")
	cmd.Flags().String("session-ttl", "", "")
	_ = cmd.Flags().Set("email", "a@b.com")
	if ttl != "" {
		_ = cmd.Flags().Set("session-ttl", ttl)
	}
	return cmd
}

// TestRunEmailEnable_SessionTTL (h-pbhn D3.2): --session-ttl is parsed and
// sent; omitted it stays nil (server keeps the stored value); bad values
// never reach the API.
func TestRunEmailEnable_SessionTTL(t *testing.T) {
	for _, tc := range []struct {
		flag    string
		want    *int
		wantErr bool
	}{
		{"", nil, false},
		{"30d", intp(2592000), false},
		{"default", intp(0), false},
		{"31d", nil, true},
		{"soon", nil, true},
	} {
		mock := &mockEmailAPIClient{}
		withTestEmailDeps(t, mock)
		_, err := captureStdout(func() error { return runEmailEnable(enableCmdWith(t, tc.flag), nil) })
		if (err != nil) != tc.wantErr {
			t.Errorf("--session-ttl %q: err = %v, wantErr %v", tc.flag, err, tc.wantErr)
			continue
		}
		if tc.wantErr {
			if mock.setCalled {
				t.Errorf("--session-ttl %q: invalid value reached the API", tc.flag)
			}
			continue
		}
		if !eqIntPtr(mock.lastSetTTL, tc.want) {
			t.Errorf("--session-ttl %q: sent %v, want %v", tc.flag, derefOr(mock.lastSetTTL), derefOr(tc.want))
		}
	}
}

func TestRunEmailSessionTTL_KeepsListsAndSendsTTL(t *testing.T) {
	mock := &mockEmailAPIClient{getFn: func(string) (*EmailProtection, error) {
		return &EmailProtection{Enabled: true, Emails: []string{"a@b.com"}, Domains: []string{"jbi.example"}, SessionTTL: 43200}, nil
	}, setFn: func(_ string, emails, domains []string) (*EmailProtection, error) {
		return &EmailProtection{Enabled: true, Emails: emails, Domains: domains, SessionTTL: 2592000}, nil
	}}
	withTestEmailDeps(t, mock)

	out, err := captureStdout(func() error { return runEmailSessionTTL(&cobra.Command{}, []string{"30d"}) })
	if err != nil {
		t.Fatalf("runEmailSessionTTL: %v", err)
	}
	if strings.Join(mock.lastSetEmails, ",") != "a@b.com" || strings.Join(mock.lastSetDomains, ",") != "jbi.example" {
		t.Errorf("lists sent = %v / %v, want the current ones unchanged", mock.lastSetEmails, mock.lastSetDomains)
	}
	if !eqIntPtr(mock.lastSetTTL, intp(2592000)) {
		t.Errorf("ttl sent = %v, want 2592000", derefOr(mock.lastSetTTL))
	}
	if !strings.Contains(out, "30d") {
		t.Errorf("output = %q, want the new TTL", out)
	}
}

func TestRunEmailSessionTTL_DisabledOrInvalid(t *testing.T) {
	mock := &mockEmailAPIClient{getFn: func(string) (*EmailProtection, error) { return &EmailProtection{Enabled: false}, nil }}
	withTestEmailDeps(t, mock)
	if _, err := captureStdout(func() error { return runEmailSessionTTL(&cobra.Command{}, []string{"7d"}) }); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Errorf("disabled egg: err = %v, want a 'disabled' error", err)
	}
	if mock.setCalled {
		t.Error("disabled egg: must not call Set")
	}
	if _, err := captureStdout(func() error { return runEmailSessionTTL(&cobra.Command{}, []string{"45m"}) }); err == nil {
		t.Error("45m: want an out-of-range error")
	}
}

// TestRunEmailAddRemove_LeaveTTLAlone: add/remove re-send the lists only,
// so they must never reset a TTL set earlier.
func TestRunEmailAddRemove_LeaveTTLAlone(t *testing.T) {
	mock := &mockEmailAPIClient{getFn: func(string) (*EmailProtection, error) {
		return &EmailProtection{Enabled: true, Emails: []string{"a@b.com"}, SessionTTL: 2592000}, nil
	}}
	withTestEmailDeps(t, mock)
	if _, err := captureStdout(func() error { return runEmailAdd(&cobra.Command{}, []string{"c@d.com"}) }); err != nil {
		t.Fatal(err)
	}
	if mock.lastSetTTL != nil {
		t.Errorf("add sent ttl %v, want nil", *mock.lastSetTTL)
	}
	if _, err := captureStdout(func() error { return runEmailRemove(&cobra.Command{}, []string{"a@b.com"}) }); err != nil {
		t.Fatal(err)
	}
	if mock.lastSetTTL != nil {
		t.Errorf("remove sent ttl %v, want nil", *mock.lastSetTTL)
	}
}

func TestRunEmailList_ShowsSessionTTL(t *testing.T) {
	mock := &mockEmailAPIClient{getFn: func(string) (*EmailProtection, error) {
		return &EmailProtection{Enabled: true, Emails: []string{"a@b.com"}, SessionTTL: 2592000, MailerConfigured: true}, nil
	}}
	withTestEmailDeps(t, mock)
	out, err := captureStdout(func() error { return runEmailList(&cobra.Command{}, nil) })
	if err != nil || !strings.Contains(out, "Session: 30d") {
		t.Errorf("list output = %q (err %v), want 'Session: 30d'", out, err)
	}
}

// TestEmailSubcommands_RejectStrayPositionals (h-abmr F1 pattern): a slug
// typed after a cwd-resolved subcommand must fail instead of mutating the
// cwd egg.
func TestEmailSubcommands_RejectStrayPositionals(t *testing.T) {
	for _, c := range []*cobra.Command{emailEnableCmd, emailDisableCmd, emailListCmd} {
		if err := c.Args(c, []string{"other-app"}); err == nil {
			t.Errorf("%s accepted a stray positional", c.Name())
		}
	}
	if err := emailSessionTTLCmd.Args(emailSessionTTLCmd, []string{"30d", "other-app"}); err == nil {
		t.Error("session-ttl accepted two positionals")
	}
}

// --- sessions revoke ---

type mockSessionsAPIClient struct {
	revoked []string
	err     error
}

func (m *mockSessionsAPIClient) RevokeSessions(slug string) error {
	m.revoked = append(m.revoked, slug)
	return m.err
}

func withTestSessionsDeps(t *testing.T, mock *mockSessionsAPIClient, tty bool) {
	t.Helper()
	sessionsDeps = &SessionsDeps{
		GetToken:        func() (string, error) { return "tok123", nil },
		GetCwd:          func() (string, error) { return t.TempDir(), nil },
		StdinIsTerminal: func() bool { return tty },
		NewAPIClient:    func(string) SessionsAPIClient { return mock },
	}
	t.Cleanup(func() { sessionsDeps = defaultSessionsDeps() })
}

func runRevoke(t *testing.T, stdin string, args ...string) (string, string, error) {
	t.Helper()
	cmd := newSessionsCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs(append([]string{"revoke"}, args...))
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func TestSessionsRevoke(t *testing.T) {
	t.Run("--yes revokes the named egg", func(t *testing.T) {
		mock := &mockSessionsAPIClient{}
		withTestSessionsDeps(t, mock, false)
		out, _, err := runRevoke(t, "", "portal", "--yes")
		if err != nil || len(mock.revoked) != 1 || mock.revoked[0] != "portal" || !strings.Contains(out, "revoked") {
			t.Fatalf("err=%v revoked=%v out=%q", err, mock.revoked, out)
		}
	})
	t.Run("non-TTY without --yes refuses", func(t *testing.T) {
		mock := &mockSessionsAPIClient{}
		withTestSessionsDeps(t, mock, false)
		if _, _, err := runRevoke(t, "", "portal"); err == nil || !strings.Contains(err.Error(), "--yes") {
			t.Fatalf("err = %v, want a --yes refusal", err)
		}
		if len(mock.revoked) != 0 {
			t.Fatal("must not revoke without confirmation")
		}
	})
	t.Run("TTY confirmation by typing the slug", func(t *testing.T) {
		mock := &mockSessionsAPIClient{}
		withTestSessionsDeps(t, mock, true)
		if _, _, err := runRevoke(t, "portal\n", "portal"); err != nil || len(mock.revoked) != 1 {
			t.Fatalf("err=%v revoked=%v", err, mock.revoked)
		}
	})
	t.Run("TTY wrong answer cancels", func(t *testing.T) {
		mock := &mockSessionsAPIClient{}
		withTestSessionsDeps(t, mock, true)
		_, errOut, err := runRevoke(t, "nope\n", "portal")
		if err != nil || len(mock.revoked) != 0 || !strings.Contains(errOut, "Cancelled") {
			t.Fatalf("err=%v revoked=%v stderr=%q", err, mock.revoked, errOut)
		}
	})
	t.Run("no slug and no app dir errors", func(t *testing.T) {
		withTestSessionsDeps(t, &mockSessionsAPIClient{}, false)
		if _, _, err := runRevoke(t, "", "--yes"); err == nil || !strings.Contains(err.Error(), "no app specified") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("API error surfaces", func(t *testing.T) {
		withTestSessionsDeps(t, &mockSessionsAPIClient{err: errors.New("egg is not protected")}, false)
		if _, _, err := runRevoke(t, "", "portal", "--yes"); err == nil || !strings.Contains(err.Error(), "not protected") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("two positionals rejected", func(t *testing.T) {
		withTestSessionsDeps(t, &mockSessionsAPIClient{}, false)
		if _, _, err := runRevoke(t, "", "a", "b", "--yes"); err == nil {
			t.Fatal("want an args error")
		}
	})
}

func intp(v int) *int { return &v }

func eqIntPtr(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func derefOr(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}
