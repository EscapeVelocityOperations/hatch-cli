package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSetEmailProtection(t *testing.T) {
	var gotMethod, gotPath, gotAuth string
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotAuth = r.Method, r.URL.Path, r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"email_protected":true,"emails":["a@b.com"],"domains":["corp.com"]}`))
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	ep, err := c.SetEmailProtection("my-app", []string{"a@b.com"}, []string{"corp.com"})
	if err != nil {
		t.Fatalf("SetEmailProtection: %v", err)
	}
	if gotMethod != "POST" || gotPath != "/v1/apps/my-app/email-protect" {
		t.Errorf("request = %s %s, want POST /v1/apps/my-app/email-protect", gotMethod, gotPath)
	}
	if gotAuth != "Bearer tok123" {
		t.Errorf("auth = %q, want Bearer tok123", gotAuth)
	}
	emails, _ := body["emails"].([]any)
	if len(emails) != 1 || emails[0] != "a@b.com" {
		t.Errorf("body emails = %v", body["emails"])
	}
	if !ep.EmailProtected || len(ep.Emails) != 1 || ep.Emails[0] != "a@b.com" || len(ep.Domains) != 1 || ep.Domains[0] != "corp.com" {
		t.Errorf("email protection = %+v, want protected + emails/domains passthrough", ep)
	}
}

func TestGetEmailProtection(t *testing.T) {
	var gotMethod, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"email_protected":true,"emails":["a@b.com"],"domains":[]}`))
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	ep, err := c.GetEmailProtection("my-app")
	if err != nil {
		t.Fatalf("GetEmailProtection: %v", err)
	}
	if gotMethod != "GET" || gotPath != "/v1/apps/my-app/email-protect" {
		t.Errorf("request = %s %s, want GET /v1/apps/my-app/email-protect", gotMethod, gotPath)
	}
	if !ep.EmailProtected || len(ep.Emails) != 1 {
		t.Errorf("email protection = %+v", ep)
	}
}

func TestDeleteEmailProtection(t *testing.T) {
	var gotMethod, gotPath, gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotAuth = r.Method, r.URL.Path, r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"email_protected":false}`))
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	if err := c.DeleteEmailProtection("my-app"); err != nil {
		t.Fatalf("DeleteEmailProtection: %v", err)
	}
	if gotMethod != "DELETE" || gotPath != "/v1/apps/my-app/email-protect" {
		t.Errorf("request = %s %s, want DELETE /v1/apps/my-app/email-protect", gotMethod, gotPath)
	}
	if gotAuth != "Bearer tok123" {
		t.Errorf("auth = %q, want Bearer tok123", gotAuth)
	}
}

// TestSetEmailProtection_APIError: a 4xx is surfaced as an error, not swallowed.
func TestSetEmailProtection_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid email address", http.StatusBadRequest)
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	_, err := c.SetEmailProtection("my-app", []string{"not-an-email"}, nil)
	if err == nil || !strings.Contains(err.Error(), "400") {
		t.Errorf("SetEmailProtection error = %v, want a 400 API error", err)
	}
}
