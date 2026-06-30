package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type roundTripperFunc func(req *http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

func TestNewClient(t *testing.T) {
	c := NewClient("test-token")
	if c.token != "test-token" {
		t.Fatalf("expected token 'test-token', got %q", c.token)
	}
	if c.host != DefaultHost {
		t.Fatalf("expected host %q, got %q", DefaultHost, c.host)
	}
	if c.httpClient.Timeout != defaultTimeout {
		t.Fatalf("expected default timeout %s, got %s", defaultTimeout, c.httpClient.Timeout)
	}
}

func TestUploadArtifact_UsesExtendedTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(20 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL
	c.httpClient.Timeout = 1 * time.Millisecond

	if err := c.UploadArtifact("myapp", bytes.NewReader([]byte("artifact")), "node", "node server.js"); err != nil {
		t.Fatalf("expected upload to succeed with extended timeout, got error: %v", err)
	}
}

// G-006 (h-ya0fo, review h-l9gzs MEDIUM, AC #1 'hatch cron logs'): the client
// fetches a run's logs as plain text from the API's run-logs endpoint. This is
// the CLI half of the end-to-end logs path (the cmd layer renders what this
// returns).
func TestGetCronRunLogs(t *testing.T) {
	var gotPath, gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("run log line 1\nrun log line 2\n"))
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	logs, err := c.GetCronRunLogs("demo-app", "c1", "run-2")
	if err != nil {
		t.Fatalf("GetCronRunLogs: %v", err)
	}
	if gotPath != "/v1/apps/demo-app/crons/c1/runs/run-2/logs" {
		t.Errorf("GET path = %q, want /v1/apps/demo-app/crons/c1/runs/run-2/logs", gotPath)
	}
	if gotAuth != "Bearer tok123" {
		t.Errorf("Authorization = %q, want Bearer tok123", gotAuth)
	}
	if logs != "run log line 1\nrun log line 2\n" {
		t.Errorf("logs = %q, want the two log lines", logs)
	}
}

// h-urxw: ListKeys must hit the server's real route GET /v1/users/keys (the CLI
// used /v1/keys which 404s) and decode the server shape {id,name,created_at,
// last_used_at} — created_at/last_used_at are RFC3339 strings parsed into time.Time.
func TestListKeys(t *testing.T) {
	var gotPath, gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":"k1","name":"My Key","created_at":"2026-01-15T00:00:00Z","last_used_at":"2026-02-01T00:00:00Z"}]`))
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	keys, err := c.ListKeys()
	if err != nil {
		t.Fatalf("ListKeys: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", gotMethod)
	}
	if gotPath != "/v1/users/keys" {
		t.Errorf("path = %q, want /v1/users/keys (the server only serves /v1/users/keys)", gotPath)
	}
	if len(keys) != 1 || keys[0].ID != "k1" || keys[0].Name != "My Key" {
		t.Fatalf("keys = %+v, want one key k1/My Key", keys)
	}
	if keys[0].CreatedAt.IsZero() {
		t.Errorf("CreatedAt not parsed from the RFC3339 string")
	}
}

func TestUploadArtifact_TimeoutErrorMessage(t *testing.T) {
	c := NewClient("tok123")
	c.httpClient.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return nil, timeoutError{}
	})

	err := c.UploadArtifact("myapp", bytes.NewReader([]byte("artifact")), "node", "node server.js")
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "upload timed out after") {
		t.Fatalf("expected timeout message, got: %v", err)
	}
}

func TestUploadArtifact_RetriesOnTransientError(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("service unavailable"))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	err := c.UploadArtifact("myapp", bytes.NewReader([]byte("artifact")), "node", "node server.js")
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

func TestUploadArtifact_NoRetryOn4xx(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("unauthorized"))
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	err := c.UploadArtifact("myapp", bytes.NewReader([]byte("artifact")), "node", "node server.js")
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if attempts != 1 {
		t.Fatalf("expected exactly 1 attempt (no retry on 4xx), got %d", attempts)
	}
	if !strings.Contains(err.Error(), "upload failed (401)") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestUploadArtifact_ExhaustsRetries(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("bad gateway"))
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	err := c.UploadArtifact("myapp", bytes.NewReader([]byte("artifact")), "node", "node server.js")
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts (maxRetries), got %d", attempts)
	}
}

func TestListApps(t *testing.T) {
	apps := []App{
		{Slug: "myapp", Name: "myapp", Status: "running", URL: "https://myapp.gethatch.eu"},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/apps" {
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
		if r.URL.Path != "/v1/apps/myapp" {
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
		if r.URL.Path != "/v1/apps/myapp/restart" {
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
		if r.URL.Path != "/v1/apps/myapp" {
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
		if r.URL.Path != "/v1/apps/myapp/env" {
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
		if r.Method != "POST" {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/apps/myapp/env" {
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
		if r.URL.Path != "/v1/apps/myapp/env/PORT" {
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
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/apps/myapp/logs" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("lines") != "50" {
			t.Fatalf("expected lines=50, got %s", r.URL.Query().Get("lines"))
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade failed: %v", err)
			return
		}
		defer conn.Close()
		conn.WriteMessage(websocket.TextMessage, []byte("line one"))
		conn.WriteMessage(websocket.TextMessage, []byte("line two"))
		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	var lines []string
	err := c.StreamLogs("myapp", 50, false, "", func(line string) {
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

func TestStreamLogs_BuildLogs(t *testing.T) {
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("type") != "build" {
			t.Fatalf("expected type=build, got %s", r.URL.Query().Get("type"))
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade failed: %v", err)
			return
		}
		defer conn.Close()
		conn.WriteMessage(websocket.TextMessage, []byte("build output"))
		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	var lines []string
	err := c.StreamLogs("myapp", 100, true, "build", func(line string) {
		lines = append(lines, line)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
}

func TestCreateApp(t *testing.T) {
	app := App{
		Slug:   "myapp-j9ou",
		Name:   "myapp",
		Status: "building",
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/apps" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(app)
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	result, err := c.CreateApp("myapp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Slug != "myapp-j9ou" {
		t.Fatalf("expected slug 'myapp-j9ou', got %q", result.Slug)
	}
	if result.Name != "myapp" {
		t.Fatalf("expected name 'myapp', got %q", result.Name)
	}
}

// h-t3ju6 GAP-2: the real *Client cron methods (the cmd/cron APIClient
// interface) issue the HTTP calls the control plane already serves
// (POST/GET/DELETE /apps/{slug}/crons + runs). Until these exist the cron
// command is wired to a stub that returns "cron API client not wired yet".

func TestCreateCron(t *testing.T) {
	var gotMethod, gotPath, gotAuth string
	var body createCronBody
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotAuth = r.Method, r.URL.Path, r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(CronJob{ID: "cron-1", Schedule: body.Schedule, Command: body.Command, Enabled: true})
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	job, err := c.CreateCron("demo-app", "*/5 * * * *", "echo hi")
	if err != nil {
		t.Fatalf("CreateCron: %v", err)
	}
	if gotMethod != "POST" {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/v1/apps/demo-app/crons" {
		t.Errorf("path = %q, want /v1/apps/demo-app/crons", gotPath)
	}
	if gotAuth != "Bearer tok123" {
		t.Errorf("auth = %q, want Bearer tok123", gotAuth)
	}
	if body.Schedule != "*/5 * * * *" || body.Command != "echo hi" {
		t.Errorf("body = %+v, want schedule=*/5 * * * * command=echo hi", body)
	}
	if job == nil || job.ID != "cron-1" || job.Schedule != "*/5 * * * *" || job.Command != "echo hi" {
		t.Errorf("returned job = %+v, want id=cron-1 schedule/command echoed", job)
	}
}

// createCronBody mirrors the POST /apps/{slug}/crons request the API expects,
// so the test can assert the client sends schedule+command.
type createCronBody struct {
	Schedule string `json:"schedule"`
	Command  string `json:"command"`
}

func TestListCrons(t *testing.T) {
	var gotMethod, gotPath string
	lastRun := time.Date(2026, 6, 10, 3, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		json.NewEncoder(w).Encode([]CronJob{
			{ID: "cron-1", Schedule: "*/5 * * * *", Command: "echo hi", Enabled: true, LastRunStatus: "success", LastRunAt: &lastRun},
			{ID: "cron-2", Schedule: "0 4 * * *", Command: "npm run cleanup", Enabled: false},
		})
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	crons, err := c.ListCrons("demo-app")
	if err != nil {
		t.Fatalf("ListCrons: %v", err)
	}
	if gotMethod != "GET" {
		t.Errorf("method = %q, want GET", gotMethod)
	}
	if gotPath != "/v1/apps/demo-app/crons" {
		t.Errorf("path = %q, want /v1/apps/demo-app/crons", gotPath)
	}
	if len(crons) != 2 {
		t.Fatalf("got %d crons, want 2", len(crons))
	}
	if crons[0].ID != "cron-1" || crons[0].LastRunStatus != "success" {
		t.Errorf("cron[0] = %+v, want id=cron-1 last_run_status=success", crons[0])
	}
	if crons[1].Enabled {
		t.Errorf("cron[1].Enabled = true, want false")
	}
}

func TestDeleteCron(t *testing.T) {
	var gotMethod, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	if err := c.DeleteCron("demo-app", "cron-1"); err != nil {
		t.Fatalf("DeleteCron: %v", err)
	}
	if gotMethod != "DELETE" {
		t.Errorf("method = %q, want DELETE", gotMethod)
	}
	if gotPath != "/v1/apps/demo-app/crons/cron-1" {
		t.Errorf("path = %q, want /v1/apps/demo-app/crons/cron-1", gotPath)
	}
}

func TestListCronRuns(t *testing.T) {
	var gotMethod, gotPath string
	started := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		json.NewEncoder(w).Encode([]CronRun{
			{ID: "run-2", Status: "success", StartedAt: started},
			{ID: "run-1", Status: "failed", StartedAt: started.Add(-time.Hour)},
		})
	}))
	defer server.Close()

	c := NewClient("tok123")
	c.host = server.URL

	runs, err := c.ListCronRuns("demo-app", "cron-1")
	if err != nil {
		t.Fatalf("ListCronRuns: %v", err)
	}
	if gotMethod != "GET" {
		t.Errorf("method = %q, want GET", gotMethod)
	}
	if gotPath != "/v1/apps/demo-app/crons/cron-1/runs" {
		t.Errorf("path = %q, want /v1/apps/demo-app/crons/cron-1/runs", gotPath)
	}
	if len(runs) != 2 || runs[0].ID != "run-2" {
		t.Fatalf("runs = %+v, want 2 with run-2 first", runs)
	}
}
