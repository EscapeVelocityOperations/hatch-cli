package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// fictiveInviteToken is a made-up hex invite token used only by these tests.
const fictiveInviteToken = "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90"

func TestRedactToken_InvitePaths(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Post \"https://api.gethatch.eu/v1/invitations/" + fictiveInviteToken + "/accept\": dial tcp: refused",
			"Post \"https://api.gethatch.eu/v1/invitations/****/accept\": dial tcp: refused"},
		{"> POST https://h/v1/invitations/" + fictiveInviteToken + "/decline",
			"> POST https://h/v1/invitations/****/decline"},
		{"/invitations/a%2Fb/accept", "/invitations/****/accept"},
		{"> GET https://h/v1/invitations/pending", "> GET https://h/v1/invitations/pending"},
		{"Bearer hatch_abc123", "Bearer hatch_****"},
	}
	for _, tt := range tests {
		if got := RedactToken(tt.in); got != tt.want {
			t.Errorf("RedactToken(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// captureStderr runs f with os.Stderr redirected and returns what it wrote.
func captureStderr(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	defer func() { os.Stderr = old }()
	f()
	_ = w.Close()
	os.Stderr = old
	return <-done
}

// failingTransport simulates a network failure before the request reaches
// the API, after recording the real URL that would have been called.
type failingTransport struct{ gotPath string }

func (f *failingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	f.gotPath = r.URL.Path
	return nil, errors.New("simulated connection refused")
}

// TestInviteTokenRedactedFromDiagnostics (h-e3s1k.1): accept/decline send the
// real token to the API, but neither verbose stderr nor transport errors
// carry it, with verbose on and off.
func TestInviteTokenRedactedFromDiagnostics(t *testing.T) {
	calls := map[string]func(c *Client) error{
		"accept":  func(c *Client) error { _, err := c.AcceptInvite(fictiveInviteToken); return err },
		"decline": func(c *Client) error { return c.DeclineInvite(fictiveInviteToken) },
	}
	for action, call := range calls {
		for _, verbose := range []bool{false, true} {
			name := action
			if verbose {
				name += "/verbose"
			}
			t.Run(name+"/transport-error", func(t *testing.T) {
				SetVerbose(verbose)
				defer SetVerbose(false)
				ft := &failingTransport{}
				c := NewTestClient("hatch_fictive", "https://api.invalid")
				c.httpClient = &http.Client{Transport: ft}
				var err error
				stderr := captureStderr(t, func() { err = call(c) })
				if err == nil {
					t.Fatal("expected transport error")
				}
				if want := "/v1/invitations/" + fictiveInviteToken + "/" + action; ft.gotPath != want {
					t.Errorf("request path = %q, want %q", ft.gotPath, want)
				}
				if strings.Contains(err.Error(), fictiveInviteToken) {
					t.Errorf("error leaks token: %v", err)
				}
				if !strings.Contains(err.Error(), "/invitations/****/"+action) {
					t.Errorf("error lost its redacted URL context: %v", err)
				}
				if strings.Contains(stderr, fictiveInviteToken) {
					t.Errorf("stderr leaks token: %q", stderr)
				}
				if verbose && !strings.Contains(stderr, "/invitations/****/"+action) {
					t.Errorf("verbose stderr missing redacted request line: %q", stderr)
				}
			})
			t.Run(name+"/api-error", func(t *testing.T) {
				SetVerbose(verbose)
				defer SetVerbose(false)
				var gotPath string
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					gotPath = r.URL.Path
					http.Error(w, `{"error":"bad gateway"}`, http.StatusBadGateway)
				}))
				defer server.Close()
				c := NewTestClient("hatch_fictive", server.URL)
				var err error
				stderr := captureStderr(t, func() { err = call(c) })
				if err == nil {
					t.Fatal("expected API error")
				}
				if want := "/v1/invitations/" + fictiveInviteToken + "/" + action; gotPath != want {
					t.Errorf("request path = %q, want %q", gotPath, want)
				}
				if strings.Contains(err.Error()+stderr, fictiveInviteToken) {
					t.Errorf("token leaked: err=%v stderr=%q", err, stderr)
				}
			})
		}
	}
}

// TestPendingInviteTokenDecodedButNotMarshaled (h-e3s1k.2): the invite token
// is decoded from GET /invitations/pending for accept/decline resolution but
// never re-emitted when a PendingInvite is marshaled (CLI/MCP output).
func TestPendingInviteTokenDecodedButNotMarshaled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"id":"row-1","invite_token":"` + fictiveInviteToken + `","app_slug":"their-app"}]`))
	}))
	defer server.Close()

	invites, err := NewTestClient("hatch_fictive", server.URL).ListPendingInvites()
	if err != nil {
		t.Fatal(err)
	}
	if len(invites) != 1 || invites[0].ID != "row-1" || invites[0].InviteToken != fictiveInviteToken || invites[0].AppSlug != "their-app" {
		t.Fatalf("invites = %+v", invites)
	}
	data, _ := json.Marshal(invites)
	if strings.Contains(string(data), fictiveInviteToken) || strings.Contains(string(data), "invite_token") {
		t.Errorf("marshaled invites leak the token: %s", data)
	}
}

func TestFindPendingInvite(t *testing.T) {
	invites := []PendingInvite{
		{ID: "row-1", InviteToken: "tok-1"},
		{ID: "row-2"}, // API returned no token
	}
	for _, ref := range []string{"row-1", "tok-1"} {
		inv, err := FindPendingInvite(invites, ref)
		if err != nil || inv.InviteToken != "tok-1" {
			t.Errorf("FindPendingInvite(%q) = %+v, %v", ref, inv, err)
		}
	}
	if _, err := FindPendingInvite(invites, "nope"); !errors.Is(err, ErrNoMatchingInvite) {
		t.Errorf("unknown ref err = %v, want ErrNoMatchingInvite", err)
	}
	if _, err := FindPendingInvite(invites, ""); !errors.Is(err, ErrNoMatchingInvite) {
		t.Errorf("empty ref err = %v, want ErrNoMatchingInvite", err)
	}
	if _, err := FindPendingInvite(invites, "row-2"); err == nil || errors.Is(err, ErrNoMatchingInvite) {
		t.Errorf("tokenless row err = %v, want explicit missing-token error", err)
	}
}
