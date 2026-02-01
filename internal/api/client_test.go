package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	c := NewClient("test-token")
	if c.token != "test-token" {
		t.Fatalf("expected token 'test-token', got %q", c.token)
	}
	if c.host != DefaultHost {
		t.Fatalf("expected host %q, got %q", DefaultHost, c.host)
	}
}

func TestListApps(t *testing.T) {
	apps := []App{
		{Slug: "myapp", Name: "myapp", Status: "running", URL: "https://myapp.gethatch.eu"},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/apps" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer tok123" {
			t.Fatalf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}
		json.NewEncoder(w).Encode(apps)
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	result, err := c.ListApps()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 app, got %d", len(result))
	}
	if result[0].Slug != "myapp" {
		t.Fatalf("expected slug 'myapp', got %q", result[0].Slug)
	}
}

func TestGetApp(t *testing.T) {
	app := App{
		Slug:      "myapp",
		Name:      "myapp",
		Status:    "running",
		URL:       "https://myapp.gethatch.eu",
		Region:    "eu-west",
		CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/apps/myapp" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(app)
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	result, err := c.GetApp("myapp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Slug != "myapp" {
		t.Fatalf("expected slug 'myapp', got %q", result.Slug)
	}
	if result.Region != "eu-west" {
		t.Fatalf("expected region 'eu-west', got %q", result.Region)
	}
}

func TestAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("app not found"))
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	_, err := c.GetApp("nonexistent")
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if err.Error() != "API error 404: app not found" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRestartApp(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/apps/myapp/restart" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	err := c.RestartApp("myapp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected restart endpoint to be called")
	}
}

func TestDeleteApp(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Fatalf("expected DELETE, got %s", r.Method)
		}
		if r.URL.Path != "/apps/myapp" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	err := c.DeleteApp("myapp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetEnvVars(t *testing.T) {
	vars := []EnvVar{{Key: "PORT", Value: "8080"}, {Key: "DB_URL", Value: "postgres://..."}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/apps/myapp/env" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(vars)
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	result, err := c.GetEnvVars("myapp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 vars, got %d", len(result))
	}
	if result[0].Key != "PORT" {
		t.Fatalf("expected key 'PORT', got %q", result[0].Key)
	}
}

func TestSetEnvVar(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PATCH" {
			t.Fatalf("expected PATCH, got %s", r.Method)
		}
		if r.URL.Path != "/apps/myapp/env" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	err := c.SetEnvVar("myapp", "PORT", "8080")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnsetEnvVar(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Fatalf("expected DELETE, got %s", r.Method)
		}
		if r.URL.Path != "/apps/myapp/env/PORT" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	err := c.UnsetEnvVar("myapp", "PORT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStreamLogs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/apps/myapp/logs" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("tail") != "50" {
			t.Fatalf("expected tail=50, got %s", r.URL.Query().Get("tail"))
		}
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Fatalf("expected SSE accept header")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: line one\ndata: line two\n"))
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	var lines []string
	err := c.StreamLogs("myapp", 50, false, func(line string) {
		lines = append(lines, line)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if lines[0] != "line one" {
		t.Fatalf("expected 'line one', got %q", lines[0])
	}
}
