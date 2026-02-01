package auth

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestCallbackServerValidFlow(t *testing.T) {
	state := "test-state-abc"
	srv := NewCallbackServer(0, state)

	// Use port 0 to get a free port; override listener
	srv.port = 0
	// Re-create with actual free port
	srv2 := &CallbackServer{
		port:          0,
		expectedState: state,
		resultCh:      make(chan CallbackResult, 1),
	}

	// Start on random port
	listener, err := listenFreePort()
	if err != nil {
		t.Fatal(err)
	}
	srv2.listener = listener
	port := listener.Addr().(*net.TCPAddr).Port

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", srv2.handleCallback)
	server := &http.Server{Handler: mux}
	go server.Serve(listener)
	defer server.Close()

	// Make request with valid state and token
	url := fmt.Sprintf("http://localhost:%d/callback?state=%s&token=my-token", port, state)
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	token, err := srv2.WaitForResult(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if token != "my-token" {
		t.Errorf("token = %q, want %q", token, "my-token")
	}
}

func TestCallbackServerStateMismatch(t *testing.T) {
	srv := &CallbackServer{
		expectedState: "correct",
		resultCh:      make(chan CallbackResult, 1),
	}

	listener, err := listenFreePort()
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	srv.listener = listener

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", srv.handleCallback)
	server := &http.Server{Handler: mux}
	go server.Serve(listener)
	defer server.Close()

	url := fmt.Sprintf("http://localhost:%d/callback?state=wrong&token=tok", port)
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err = srv.WaitForResult(ctx)
	if err == nil {
		t.Error("expected error for state mismatch")
	}
}

func TestCallbackServerMissingToken(t *testing.T) {
	state := "valid-state"
	srv := &CallbackServer{
		expectedState: state,
		resultCh:      make(chan CallbackResult, 1),
	}

	listener, err := listenFreePort()
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	srv.listener = listener

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", srv.handleCallback)
	server := &http.Server{Handler: mux}
	go server.Serve(listener)
	defer server.Close()

	url := fmt.Sprintf("http://localhost:%d/callback?state=%s&token=", port, state)
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestCallbackServerTimeout(t *testing.T) {
	srv := &CallbackServer{
		expectedState: "s",
		resultCh:      make(chan CallbackResult, 1),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := srv.WaitForResult(ctx)
	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestCallbackServerStart(t *testing.T) {
	srv := NewCallbackServer(0, "test-state")
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	// Verify we can make a request
	port := srv.listener.Addr().(*net.TCPAddr).Port
	url := fmt.Sprintf("http://localhost:%d/callback?state=test-state&token=tok123", port)
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	token, err := srv.WaitForResult(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if token != "tok123" {
		t.Errorf("token = %q, want tok123", token)
	}
}

func TestCallbackServerStartPortConflict(t *testing.T) {
	// Start one server on a port
	srv1 := NewCallbackServer(0, "s")
	if err := srv1.Start(); err != nil {
		t.Fatal(err)
	}
	port := srv1.listener.Addr().(*net.TCPAddr).Port
	defer srv1.Close()

	// Try to start another on the same port
	srv2 := NewCallbackServer(port, "s")
	err := srv2.Start()
	if err == nil {
		srv2.Close()
		t.Error("expected error starting on occupied port")
	}
}

func TestCallbackServerCloseNilListener(t *testing.T) {
	srv := NewCallbackServer(0, "s")
	// Close with nil listener should not error
	if err := srv.Close(); err != nil {
		t.Errorf("Close() on nil listener: %v", err)
	}
}

func TestCallbackServerCloseActiveListener(t *testing.T) {
	srv := NewCallbackServer(0, "s")
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	if err := srv.Close(); err != nil {
		t.Errorf("Close() error: %v", err)
	}
}

func TestCallbackServerPort(t *testing.T) {
	srv := NewCallbackServer(9999, "s")
	if srv.Port() != 9999 {
		t.Errorf("Port() = %d, want 9999", srv.Port())
	}
}

func listenFreePort() (net.Listener, error) {
	return net.Listen("tcp", "localhost:0")
}
