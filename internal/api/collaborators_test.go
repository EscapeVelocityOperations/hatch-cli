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
