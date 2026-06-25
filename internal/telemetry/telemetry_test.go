package telemetry

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRedact(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"token", "error: hatch_abc123def", "error: hatch_****"},
		{"key=value", "token=secret123 other", "token=**** other"},
		{"password", "password: mysecret", "password=****"},
		{"no sensitive data", "file not found", "file not found"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := redact(tt.input)
			if got != tt.want {
				t.Errorf("redact(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSendFiresHTTPRequest(t *testing.T) {
	var received Event
	// done carries the happens-before edge: the handler closes it after writing
	// `received`, and the test reads `received` only after <-done. A time.Sleep
	// establishes no such edge, so -race flagged both the `received` write/read
	// and the deferred APIHost restore vs Send's goroutine read (h-wy0a7).
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/telemetry" {
			t.Errorf("expected /telemetry, got %s", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusCreated)
		close(done) // received is fully written; release the test
	}))
	defer server.Close()

	oldHost := APIHost
	APIHost = server.URL
	defer func() { APIHost = oldHost }()

	Send("hatch deploy", "--runtime node", "deploy failed: timeout", "cli")

	select {
	case <-done: // deterministic synchronization; replaces time.Sleep
	case <-time.After(2 * time.Second):
		t.Fatal("telemetry request not received within 2s")
	}

	if received.Command != "hatch deploy" {
		t.Errorf("expected command 'hatch deploy', got %q", received.Command)
	}
	if received.Error != "deploy failed: timeout" {
		t.Errorf("expected error 'deploy failed: timeout', got %q", received.Error)
	}
	if received.Mode != "cli" {
		t.Errorf("expected mode 'cli', got %q", received.Mode)
	}
}

func TestSendEmptyErrorIsNoop(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	oldHost := APIHost
	APIHost = server.URL
	defer func() { APIHost = oldHost }()

	Send("hatch deploy", "", "", "cli")
	// No goroutine is spawned for an empty error, so `called` is touched only by
	// this goroutine — no wait needed (the old time.Sleep was dead weight).

	if called {
		t.Error("expected no HTTP request for empty error")
	}
}
