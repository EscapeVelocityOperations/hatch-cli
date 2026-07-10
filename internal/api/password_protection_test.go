package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSetPasswordProtection(t *testing.T) {
	var gotMethod, gotPath, gotAuth string
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotAuth = r.Method, r.URL.Path, r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"protected":true}`))
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	pp, err := c.SetPasswordProtection("my-app", "hunter2")
	if err != nil {
		t.Fatalf("SetPasswordProtection: %v", err)
	}
	if gotMethod != "POST" || gotPath != "/v1/apps/my-app/protect" {
		t.Errorf("request = %s %s, want POST /v1/apps/my-app/protect", gotMethod, gotPath)
	}
	if gotAuth != "Bearer tok123" {
		t.Errorf("auth = %q, want Bearer tok123", gotAuth)
	}
	if body["password"] != "hunter2" {
		t.Errorf("body password = %v, want hunter2", body["password"])
	}
	if !pp.Protected {
		t.Errorf("password protection = %+v, want Protected:true", pp)
	}
}

func TestGetPasswordProtection(t *testing.T) {
	var gotMethod, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"protected":true}`))
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	pp, err := c.GetPasswordProtection("my-app")
	if err != nil {
		t.Fatalf("GetPasswordProtection: %v", err)
	}
	if gotMethod != "GET" || gotPath != "/v1/apps/my-app/protect" {
		t.Errorf("request = %s %s, want GET /v1/apps/my-app/protect", gotMethod, gotPath)
	}
	if !pp.Protected {
		t.Errorf("password protection = %+v, want Protected:true", pp)
	}
}

func TestDeletePasswordProtection(t *testing.T) {
	var gotMethod, gotPath, gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotAuth = r.Method, r.URL.Path, r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"protected":false}`))
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	if err := c.DeletePasswordProtection("my-app"); err != nil {
		t.Fatalf("DeletePasswordProtection: %v", err)
	}
	if gotMethod != "DELETE" || gotPath != "/v1/apps/my-app/protect" {
		t.Errorf("request = %s %s, want DELETE /v1/apps/my-app/protect", gotMethod, gotPath)
	}
	if gotAuth != "Bearer tok123" {
		t.Errorf("auth = %q, want Bearer tok123", gotAuth)
	}
}

// TestSetPasswordProtection_APIError: a 4xx is surfaced as an error, not swallowed.
func TestSetPasswordProtection_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "password is required", http.StatusBadRequest)
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	_, err := c.SetPasswordProtection("my-app", "")
	if err == nil || !strings.Contains(err.Error(), "400") {
		t.Errorf("SetPasswordProtection error = %v, want a 400 API error", err)
	}
}
