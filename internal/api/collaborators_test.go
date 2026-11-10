package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInviteCollaborator(t *testing.T) {
	var gotMethod, gotPath, gotAuth string
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotAuth = r.Method, r.URL.Path, r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"c1","app_id":"a1","email":"friend@example.com","role":"deploy","status":"pending","created_at":"2026-07-02T00:00:00Z"}`))
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	collab, err := c.InviteCollaborator("my-app", "friend@example.com")
	if err != nil {
		t.Fatalf("InviteCollaborator: %v", err)
	}
	if gotMethod != "POST" || gotPath != "/v1/apps/my-app/collaborators" {
		t.Errorf("request = %s %s, want POST /v1/apps/my-app/collaborators", gotMethod, gotPath)
	}
	if gotAuth != "Bearer tok123" {
		t.Errorf("auth = %q", gotAuth)
	}
	if body["email"] != "friend@example.com" {
		t.Errorf("body email = %v, want friend@example.com", body["email"])
	}
	if collab.Email != "friend@example.com" || collab.Status != "pending" {
		t.Errorf("collab = %+v, want email=friend@example.com status=pending", collab)
	}
}

func TestInviteCollaborator_MaxReachedError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"maximum collaborators per egg reached"}`, http.StatusConflict)
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	_, err := c.InviteCollaborator("my-app", "friend@example.com")
	if err == nil || !strings.Contains(err.Error(), "409") {
		t.Fatalf("expected 409 error, got %v", err)
	}
}

func TestListCollaborators(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"c1","app_id":"a1","email":"friend@example.com","role":"deploy","status":"accepted","created_at":"2026-07-02T00:00:00Z"}]`))
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	collabs, err := c.ListCollaborators("my-app")
	if err != nil {
		t.Fatalf("ListCollaborators: %v", err)
	}
	if gotPath != "/v1/apps/my-app/collaborators" {
		t.Errorf("path = %q", gotPath)
	}
	if len(collabs) != 1 || collabs[0].Email != "friend@example.com" {
		t.Errorf("collabs = %+v", collabs)
	}
}

func TestRemoveCollaborator(t *testing.T) {
	var gotMethod, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	if err := c.RemoveCollaborator("my-app", "c1"); err != nil {
		t.Fatalf("RemoveCollaborator: %v", err)
	}
	if gotMethod != "DELETE" || gotPath != "/v1/apps/my-app/collaborators/c1" {
		t.Errorf("request = %s %s, want DELETE /v1/apps/my-app/collaborators/c1", gotMethod, gotPath)
	}
}

func TestListPendingInvites(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"c1","email":"me@example.com","role":"deploy","status":"pending","created_at":"2026-07-02T00:00:00Z","app_slug":"their-app","app_name":"Their App","invited_by_email":"owner@example.com"}]`))
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	invites, err := c.ListPendingInvites()
	if err != nil {
		t.Fatalf("ListPendingInvites: %v", err)
	}
	if gotPath != "/v1/invitations/pending" {
		t.Errorf("path = %q", gotPath)
	}
	if len(invites) != 1 || invites[0].AppSlug != "their-app" || invites[0].InvitedByEmail != "owner@example.com" {
		t.Errorf("invites = %+v", invites)
	}
}

func TestAcceptInvite(t *testing.T) {
	var gotMethod, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"c1","app_id":"a1","email":"me@example.com","role":"deploy","status":"accepted","created_at":"2026-07-02T00:00:00Z"}`))
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	collab, err := c.AcceptInvite("tok-abc")
	if err != nil {
		t.Fatalf("AcceptInvite: %v", err)
	}
	if gotMethod != "POST" || gotPath != "/v1/invitations/tok-abc/accept" {
		t.Errorf("request = %s %s, want POST /v1/invitations/tok-abc/accept", gotMethod, gotPath)
	}
	if collab.Status != "accepted" {
		t.Errorf("status = %q, want accepted", collab.Status)
	}
}

func TestDeclineInvite(t *testing.T) {
	var gotMethod, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	if err := c.DeclineInvite("tok-abc"); err != nil {
		t.Fatalf("DeclineInvite: %v", err)
	}
	if gotMethod != "POST" || gotPath != "/v1/invitations/tok-abc/decline" {
		t.Errorf("request = %s %s, want POST /v1/invitations/tok-abc/decline", gotMethod, gotPath)
	}
}

// TestInviteCollaborator_EmailSpecialCharsJSONEncoded: an email with '+' and
// '/' travels intact in the JSON body and never touches the URL path.
func TestInviteCollaborator_EmailSpecialCharsJSONEncoded(t *testing.T) {
	const email = `a+b/c"d@example.com`
	var gotURI string
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURI = r.RequestURI
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("body is not valid JSON: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"c1","email":"x"}`))
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	if _, err := c.InviteCollaborator("my-app", email); err != nil {
		t.Fatalf("InviteCollaborator: %v", err)
	}
	if gotURI != "/v1/apps/my-app/collaborators" {
		t.Errorf("request URI = %q, want /v1/apps/my-app/collaborators", gotURI)
	}
	if body["email"] != email {
		t.Errorf("body email = %v, want %q", body["email"], email)
	}
}

// TestCollaboratorPathSegmentsEscaped: collaborator IDs and invite tokens are
// untrusted (CLI args, LLM-supplied MCP args) and are path-escaped, so '/',
// '?', '#' cannot add segments or a query; '+' and '@' are path-legal and
// pass through.
func TestCollaboratorPathSegmentsEscaped(t *testing.T) {
	var gotURI string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURI = r.RequestURI
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"c1"}`))
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	tests := []struct {
		name string
		call func() error
		want string
	}{
		{"remove id with + and /", func() error { return c.RemoveCollaborator("my-app", "a+b/c@example.com") },
			"/v1/apps/my-app/collaborators/a+b%2Fc@example.com"},
		{"remove id with traversal-ish path", func() error { return c.RemoveCollaborator("my-app", "../../apps/other") },
			"/v1/apps/my-app/collaborators/..%2F..%2Fapps%2Fother"},
		{"accept token with ? and #", func() error { _, err := c.AcceptInvite("t?x=1#f"); return err },
			"/v1/invitations/t%3Fx=1%23f/accept"},
		{"decline token with /", func() error { return c.DeclineInvite("a/b") },
			"/v1/invitations/a%2Fb/decline"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotURI = ""
			if err := tt.call(); err != nil {
				t.Fatalf("call: %v", err)
			}
			if gotURI != tt.want {
				t.Errorf("request URI = %q, want %q", gotURI, tt.want)
			}
		})
	}
}

// TestCollaboratorPathSegmentsRejectDotSegments: "." and ".." survive
// url.PathEscape unchanged and stay traversal-meaningful, so they (and "")
// are rejected before any request is made.
func TestCollaboratorPathSegmentsRejectDotSegments(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	for _, v := range []string{"", ".", ".."} {
		if err := c.RemoveCollaborator("my-app", v); err == nil {
			t.Errorf("RemoveCollaborator(%q) error = nil, want rejection", v)
		}
		if _, err := c.AcceptInvite(v); err == nil {
			t.Errorf("AcceptInvite(%q) error = nil, want rejection", v)
		}
		if err := c.DeclineInvite(v); err == nil {
			t.Errorf("DeclineInvite(%q) error = nil, want rejection", v)
		}
	}
	if called {
		t.Error("a rejected path segment must not reach the server")
	}
}
