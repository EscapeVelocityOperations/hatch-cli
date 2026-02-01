package auth

import (
	"context"
	"fmt"
	"net"
	"net/http"
)

// Server is the interface for the OAuth callback server.
type Server interface {
	Start() error
	WaitForResult(ctx context.Context) (string, error)
	Close() error
}

const successHTML = `<!DOCTYPE html>
<html><head><title>Hatch CLI</title>
<style>body{font-family:system-ui,sans-serif;display:flex;justify-content:center;align-items:center;height:100vh;margin:0;background:#f8f9fa}
.card{text-align:center;padding:2rem;border-radius:8px;background:#fff;box-shadow:0 2px 8px rgba(0,0,0,.1)}
h1{color:#22c55e;margin-bottom:.5rem}</style></head>
<body><div class="card"><h1>Authenticated</h1><p>You can close this window and return to the terminal.</p></div></body></html>`

type CallbackResult struct {
	Token string
	Err   error
}

type CallbackServer struct {
	port          int
	expectedState string
	listener      net.Listener
	resultCh      chan CallbackResult
}

func NewCallbackServer(port int, expectedState string) *CallbackServer {
	return &CallbackServer{
		port:          port,
		expectedState: expectedState,
		resultCh:      make(chan CallbackResult, 1),
	}
}

func (s *CallbackServer) Start() error {
	var err error
	s.listener, err = net.Listen("tcp", fmt.Sprintf("localhost:%d", s.port))
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %w", s.port, err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", s.handleCallback)

	srv := &http.Server{Handler: mux}
	go func() {
		if err := srv.Serve(s.listener); err != nil && err != http.ErrServerClosed {
			s.resultCh <- CallbackResult{Err: fmt.Errorf("server error: %w", err)}
		}
	}()

	return nil
}

func (s *CallbackServer) handleCallback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	token := r.URL.Query().Get("token")

	if state != s.expectedState {
		http.Error(w, "Invalid state parameter", http.StatusBadRequest)
		s.resultCh <- CallbackResult{Err: fmt.Errorf("state mismatch: expected %q, got %q", s.expectedState, state)}
		return
	}

	if token == "" {
		http.Error(w, "Missing token", http.StatusBadRequest)
		s.resultCh <- CallbackResult{Err: fmt.Errorf("empty token in callback")}
		return
	}

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, successHTML)

	s.resultCh <- CallbackResult{Token: token}
}

func (s *CallbackServer) WaitForResult(ctx context.Context) (string, error) {
	select {
	case result := <-s.resultCh:
		return result.Token, result.Err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (s *CallbackServer) Close() error {
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

func (s *CallbackServer) Port() int {
	return s.port
}
