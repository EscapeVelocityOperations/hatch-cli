package db

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestRunConnect_NotLoggedIn(t *testing.T) {
	deps = &Deps{
		GetToken: func() (string, error) { return "", nil },
	}
	defer func() { deps = defaultDeps() }()

	err := runConnect(nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "not logged in. Run 'hatch login', set HATCH_TOKEN, or use --token" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunConnect_NoArg(t *testing.T) {
	deps = &Deps{
		GetToken: func() (string, error) { return "tok123", nil },
	}
	defer func() { deps = defaultDeps() }()

	err := runConnect(nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "no egg specified") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunConnect_ListenError(t *testing.T) {
	deps = &Deps{
		GetToken: func() (string, error) { return "tok123", nil },
		Listen: func(network, address string) (net.Listener, error) {
			return nil, fmt.Errorf("address already in use")
		},
	}
	defer func() { deps = defaultDeps() }()

	err := runConnect(nil, []string{"myapp"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "address already in use") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveSlug_WithArg(t *testing.T) {
	slug, err := resolveSlug([]string{"myapp"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if slug != "myapp" {
		t.Fatalf("expected slug 'myapp', got %q", slug)
	}
}

func TestResolveSlug_NoArg(t *testing.T) {
	_, err := resolveSlug(nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "no egg specified") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWSURLForSlug(t *testing.T) {
	url := wsURLForSlug("myapp")
	expected := "wss://api.gethatch.eu/v1/apps/myapp/db/tunnel"
	if url != expected {
		t.Fatalf("expected URL %q, got %q", expected, url)
	}
}

func TestWSURLForSlug_EdgeCases(t *testing.T) {
	tests := []struct {
		slug     string
		expected string
	}{
		{"test", "wss://api.gethatch.eu/v1/apps/test/db/tunnel"},
		{"my-app-123", "wss://api.gethatch.eu/v1/apps/my-app-123/db/tunnel"},
		{"app_with_underscore", "wss://api.gethatch.eu/v1/apps/app_with_underscore/db/tunnel"},
	}
	for _, tt := range tests {
		t.Run(tt.slug, func(t *testing.T) {
			url := wsURLForSlug(tt.slug)
			if url != tt.expected {
				t.Fatalf("expected URL %q, got %q", tt.expected, url)
			}
		})
	}
}

func TestNewConnectCmd(t *testing.T) {
	cmd := newConnectCmd()
	if cmd.Use != "connect [slug] [-- psql-args...]" {
		t.Fatalf("unexpected use: %s", cmd.Use)
	}
	if cmd.Short != "Open a local TCP proxy to your egg's database" {
		t.Fatalf("unexpected short: %s", cmd.Short)
	}
	portFlag := cmd.Flags().Lookup("port")
	if portFlag == nil || portFlag.DefValue != "15432" {
		t.Fatalf("unexpected default port")
	}
	hostFlag := cmd.Flags().Lookup("host")
	if hostFlag == nil || hostFlag.DefValue != "localhost" {
		t.Fatalf("unexpected default host")
	}
	noPsqlFlag := cmd.Flags().Lookup("no-psql")
	if noPsqlFlag == nil {
		t.Fatal("no-psql flag not found")
	}
}

func TestNewCmd(t *testing.T) {
	cmd := NewCmd()
	if cmd.Use != "db" {
		t.Fatalf("unexpected use: %s", cmd.Use)
	}
	if len(cmd.Commands()) != 3 {
		t.Fatalf("expected 3 subcommands, got %d", len(cmd.Commands()))
	}
	names := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		names[sub.Name()] = true
	}
	for _, expected := range []string{"connect", "add", "info"} {
		if !names[expected] {
			t.Fatalf("expected %s subcommand", expected)
		}
	}
}

// TestFormatWSDialError is the h-f70o regression test: gorilla/websocket
// collapses every non-101 dial response into the bare string "websocket: bad
// handshake", discarding the *http.Response that actually says why (401 bad
// token, 404 unknown slug, 5xx server-side, or a Caddy-level rejection).
// formatWSDialError must recover that signal instead of letting the opaque
// string reach the user.
func TestFormatWSDialError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		resp       *http.Response
		wantSubstr []string
	}{
		{
			name:       "nil response is a network-layer failure, not a handshake rejection",
			err:        errors.New("dial tcp: connection refused"),
			resp:       nil,
			wantSubstr: []string{"connection refused"},
		},
		{
			name: "401 hints at the token",
			err:  errors.New("websocket: bad handshake"),
			resp: &http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(strings.NewReader("invalid token")),
			},
			wantSubstr: []string{"HTTP 401", "invalid token", "hatch login"},
		},
		{
			name: "404 hints at the slug",
			err:  errors.New("websocket: bad handshake"),
			resp: &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(strings.NewReader("app not found")),
			},
			wantSubstr: []string{"HTTP 404", "app not found", "unknown app slug"},
		},
		{
			name: "502 with body hints at the server side",
			err:  errors.New("websocket: bad handshake"),
			resp: &http.Response{
				StatusCode: http.StatusBadGateway,
				Body:       io.NopCloser(strings.NewReader("control plane unreachable")),
			},
			wantSubstr: []string{"HTTP 502", "control plane unreachable", "hatch logs"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatWSDialError(tt.err, tt.resp)
			if strings.Contains(got, "bad handshake") {
				t.Errorf("formatWSDialError() = %q, must never let the bare gorilla error through", got)
			}
			for _, want := range tt.wantSubstr {
				if !strings.Contains(got, want) {
					t.Errorf("formatWSDialError() = %q, want substring %q", got, want)
				}
			}
		})
	}
}

// TestHandleConn_SurfacesHTTPStatus confirms handleConn is wired to
// formatWSDialError instead of printing the raw dial error — capture
// stdout since ui.Error writes straight to it (fmt.Println, no seam).
func TestHandleConn_SurfacesHTTPStatus(t *testing.T) {
	deps = &Deps{
		DialWS: func(u string, h http.Header) (*websocket.Conn, *http.Response, error) {
			return nil, &http.Response{
				StatusCode: http.StatusBadGateway,
				Body:       io.NopCloser(strings.NewReader("control plane unreachable")),
			}, errors.New("websocket: bad handshake")
		},
	}
	defer func() { deps = defaultDeps() }()

	out := captureStdout(t, func() {
		clientConn, serverConn := net.Pipe()
		defer clientConn.Close()
		handleConn(serverConn, "wss://example.invalid/tunnel", nil)
	})

	if !strings.Contains(out, "HTTP 502") {
		t.Errorf("output = %q, want it to contain HTTP 502", out)
	}
	if strings.Contains(out, "bad handshake") {
		t.Errorf("output = %q, must not contain the bare gorilla error", out)
	}
}

// captureStdout redirects os.Stdout for the duration of fn and returns
// whatever was written. fn runs synchronously; the pipe is drained
// concurrently so a blocking Println inside fn can't deadlock against an
// unread pipe buffer.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	old := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = old }()

	outCh := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(r)
		outCh <- string(data)
	}()

	fn()
	w.Close()

	select {
	case out := <-outCh:
		return out
	case <-time.After(2 * time.Second):
		t.Fatal("timed out reading captured stdout")
		return ""
	}
}

// fakeListener answers exactly one Accept() call with an error, simulating
// an already-closed/shutdown listener — enough to let runConnect's accept
// loop return immediately (existing behavior: any Accept error -> `return
// nil`, "Listener closed (shutdown)") without a real client connection.
type fakeListener struct{}

func (f *fakeListener) Accept() (net.Conn, error) { return nil, fmt.Errorf("closed") }
func (f *fakeListener) Close() error              { return nil }
func (f *fakeListener) Addr() net.Addr            { return &net.TCPAddr{} }

// TestConnect_StartupProbeFailsFast is the h-f70o regression test for D2:
// previously --no-psql sat silently until the first client connected, then
// failed per-connection with an opaque error. The probe must fail runConnect
// itself, with the real status surfaced, before ever reaching the accept
// loop.
func TestConnect_StartupProbeFailsFast(t *testing.T) {
	deps = &Deps{
		GetToken: func() (string, error) { return "tok123", nil },
		DialWS: func(u string, h http.Header) (*websocket.Conn, *http.Response, error) {
			return nil, &http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(strings.NewReader("invalid token")),
			}, errors.New("websocket: bad handshake")
		},
		Listen: func(network, address string) (net.Listener, error) {
			return &fakeListener{}, nil
		},
	}
	defer func() { deps = defaultDeps() }()

	err := runConnect(nil, []string{"myapp"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "HTTP 401") {
		t.Errorf("error = %v, want it to contain HTTP 401", err)
	}
	if strings.Contains(err.Error(), "bad handshake") {
		t.Errorf("error = %v, must not contain the bare gorilla error", err)
	}
}

// TestProbeTunnel_ClosesCleanly is the D2 happy-path counterpart, exercised
// directly against probeTunnel rather than the full runConnect: a successful
// probe must close its connection (not leak it), verified against a real
// gorilla websocket server so the close is observed on the wire, not just
// assumed. (Going through runConnect here would also exercise its DB-
// credentials fetch, which isn't behind the Deps seam and reaches the real
// api.gethatch.eu over the network — not hermetic, and not what this test
// is about.)
func TestProbeTunnel_ClosesCleanly(t *testing.T) {
	closed := make(chan struct{})
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer c.Close()
		// Blocks until the client closes; a normal close unblocks ReadMessage
		// with a close error instead of hanging.
		_, _, _ = c.ReadMessage()
		close(closed)
	}))
	defer srv.Close()

	testWSURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	dialer := &websocket.Dialer{}
	deps = &Deps{
		DialWS: func(u string, h http.Header) (*websocket.Conn, *http.Response, error) {
			return dialer.Dial(testWSURL, h)
		},
	}
	defer func() { deps = defaultDeps() }()

	if err := probeTunnel("wss://example.invalid/tunnel", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case <-closed:
		// probe connection was closed cleanly; the server observed it.
	case <-time.After(2 * time.Second):
		t.Fatal("probe connection was never closed")
	}
}

// TestProbeTunnel_FailsFast is the probeTunnel-level counterpart to
// TestConnect_StartupProbeFailsFast: confirms the real status is surfaced
// (not the bare gorilla error), independent of runConnect's surrounding
// setup.
func TestProbeTunnel_FailsFast(t *testing.T) {
	deps = &Deps{
		DialWS: func(u string, h http.Header) (*websocket.Conn, *http.Response, error) {
			return nil, &http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(strings.NewReader("invalid token")),
			}, errors.New("websocket: bad handshake")
		},
	}
	defer func() { deps = defaultDeps() }()

	err := probeTunnel("wss://example.invalid/tunnel", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "HTTP 401") {
		t.Errorf("error = %v, want it to contain HTTP 401", err)
	}
	if strings.Contains(err.Error(), "bad handshake") {
		t.Errorf("error = %v, must not contain the bare gorilla error", err)
	}
}

func TestDefaultDeps(t *testing.T) {
	d := defaultDeps()
	if d.GetToken == nil {
		t.Fatal("GetToken not set")
	}
	if d.DialWS == nil {
		t.Fatal("DialWS not set")
	}
	if d.Listen == nil {
		t.Fatal("Listen not set")
	}
	if d.RunPsql == nil {
		t.Fatal("RunPsql not set")
	}
}
