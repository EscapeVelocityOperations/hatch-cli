package mcpserver

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
)

// --- fakes ---

// fakeCallbackServer is a test double for authCallbackServer.
type fakeCallbackServer struct {
	startErr   error
	port       int
	token      string
	waitErr    error
	blockOnCtx bool // if true, WaitForResult blocks until ctx is done (never resolves on its own)
	waitDelay  time.Duration

	mu     sync.Mutex
	closed bool
}

func (f *fakeCallbackServer) Start() error { return f.startErr }
func (f *fakeCallbackServer) Port() int    { return f.port }

func (f *fakeCallbackServer) WaitForResult(ctx context.Context) (string, error) {
	if f.blockOnCtx {
		<-ctx.Done()
		return "", ctx.Err()
	}
	if f.waitDelay > 0 {
		select {
		case <-time.After(f.waitDelay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if f.waitErr != nil {
		return "", f.waitErr
	}
	return f.token, nil
}

func (f *fakeCallbackServer) Close() error {
	f.mu.Lock()
	f.closed = true
	f.mu.Unlock()
	return nil
}

func (f *fakeCallbackServer) isClosed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

// saveAndRestoreLoginDeps saves the login DI seams and timeouts, restoring on cleanup.
func saveAndRestoreLoginDeps(t *testing.T) {
	t.Helper()
	origState := loginGenerateState
	origNewServer := loginNewCallbackServer
	origBrowser := loginOpenBrowser
	origSave := loginSaveToken
	origBlock := loginBlockTimeout
	origBackground := loginBackgroundTimeout
	t.Cleanup(func() {
		loginGenerateState = origState
		loginNewCallbackServer = origNewServer
		loginOpenBrowser = origBrowser
		loginSaveToken = origSave
		loginBlockTimeout = origBlock
		loginBackgroundTimeout = origBackground
		loginFlowMu.Lock()
		activeLoginFlow = nil
		loginFlowMu.Unlock()
	})
}

// waitUntil polls cond every 5ms until it's true or the timeout elapses.
func waitUntil(t *testing.T, timeout time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return cond()
}

func fakeEnergyRoute() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"GET /v1/account/energy": jsonHandler(api.EnergyStatus{
			Tier: "free", DailyRemaining: 200, DailyLimit: 240,
			EggsActive: 1, EggsLimit: 3,
		}),
	}
}

// --- T-004: already-authenticated short-circuit ---

func TestLoginHandler_AlreadyAuthenticated(t *testing.T) {
	saveAndRestore(t)
	saveAndRestoreLoginDeps(t)
	setAuthToken("hatch_existing_tok")
	newMockServer(t, fakeEnergyRoute())

	serverStarted := false
	loginNewCallbackServer = func(state string) authCallbackServer {
		serverStarted = true
		return &fakeCallbackServer{}
	}

	result, err := loginHandler(context.Background(), makeReq(nil))
	text := assertSuccess(t, result, err)

	if !strings.Contains(text, "Authenticated") {
		t.Errorf("expected 'Authenticated' in result, got: %s", text)
	}
	if !strings.Contains(text, "free") {
		t.Errorf("expected plan tier in result, got: %s", text)
	}
	if serverStarted {
		t.Error("expected no callback server to be started when already authenticated")
	}
}

func TestLoginHandler_EnergyFetchFailureDegradesGracefully(t *testing.T) {
	saveAndRestore(t)
	saveAndRestoreLoginDeps(t)
	setAuthToken("hatch_tok")
	newMockServer(t, map[string]http.HandlerFunc{
		"GET /v1/account/energy": errorHandler(500, "energy service down"),
	})

	result, err := loginHandler(context.Background(), makeReq(nil))
	text := assertSuccess(t, result, err)

	if text != "Authenticated." {
		t.Errorf("expected bare 'Authenticated.' when the energy fetch fails (login must never fail after auth succeeds), got: %q", text)
	}
}

// --- T-005: happy path ---

func TestLoginHandler_HappyPath(t *testing.T) {
	saveAndRestore(t)
	saveAndRestoreLoginDeps(t)
	setNoAuth()
	newMockServer(t, fakeEnergyRoute())

	loginGenerateState = func() (string, error) { return "test-state-abc", nil }

	fake := &fakeCallbackServer{port: 54321, token: "oauth-tok-secret-123"}
	loginNewCallbackServer = func(state string) authCallbackServer {
		if state != "test-state-abc" {
			t.Errorf("expected callback server built with the generated state, got %q", state)
		}
		return fake
	}

	var openedURL string
	loginOpenBrowser = func(url string) error { openedURL = url; return nil }

	var savedToken string
	loginSaveToken = func(token string) error { savedToken = token; return nil }

	result, err := loginHandler(context.Background(), makeReq(nil))
	text := assertSuccess(t, result, err)

	if !strings.Contains(openedURL, "state=test-state-abc") {
		t.Errorf("expected auth URL to contain the generated state, got: %s", openedURL)
	}
	if !strings.Contains(openedURL, "port=54321") {
		t.Errorf("expected auth URL to contain the callback port, got: %s", openedURL)
	}
	if savedToken != "oauth-tok-secret-123" {
		t.Errorf("expected token to be saved, got: %q", savedToken)
	}
	if !strings.Contains(text, "free") {
		t.Errorf("expected plan tier in result, got: %s", text)
	}
	if strings.Contains(text, "oauth-tok-secret-123") {
		t.Error("login result must NEVER contain the token")
	}
}

// --- T-006: headless, background save, re-entrancy ---

func TestLoginHandler_Headless_ReturnsAuthURL(t *testing.T) {
	saveAndRestore(t)
	saveAndRestoreLoginDeps(t)
	setNoAuth()

	loginGenerateState = func() (string, error) { return "headless-state", nil }
	fake := &fakeCallbackServer{port: 9999, blockOnCtx: true}
	loginNewCallbackServer = func(state string) authCallbackServer { return fake }
	loginOpenBrowser = func(url string) error { return fmt.Errorf("no display") }
	loginBackgroundTimeout = 50 * time.Millisecond

	result, err := loginHandler(context.Background(), makeReq(nil))
	text := assertSuccess(t, result, err)

	if !strings.Contains(text, "headless-state") {
		t.Errorf("expected auth URL (with state) in headless result, got: %s", text)
	}
	if !strings.Contains(strings.ToLower(text), "5 min") {
		t.Errorf("expected a ~5 min callback-armed note in headless result, got: %s", text)
	}

	waitUntil(t, time.Second, fake.isClosed)
}

func TestLoginHandler_BackgroundSaveAfterBlockingTimeout(t *testing.T) {
	saveAndRestore(t)
	saveAndRestoreLoginDeps(t)
	setNoAuth()
	newMockServer(t, fakeEnergyRoute())

	loginGenerateState = func() (string, error) { return "bg-state", nil }
	fake := &fakeCallbackServer{port: 1234, token: "late-tok", waitDelay: 80 * time.Millisecond}
	loginNewCallbackServer = func(state string) authCallbackServer { return fake }
	loginOpenBrowser = func(url string) error { return nil }
	loginBlockTimeout = 20 * time.Millisecond // shorter than waitDelay: handler returns before the callback lands
	loginBackgroundTimeout = 2 * time.Second

	var mu sync.Mutex
	var savedToken string
	loginSaveToken = func(token string) error {
		mu.Lock()
		savedToken = token
		mu.Unlock()
		return nil
	}

	result, err := loginHandler(context.Background(), makeReq(nil))
	text := assertSuccess(t, result, err)
	if !strings.Contains(text, "bg-state") {
		t.Fatalf("expected the pending auth URL from the blocking timeout, got: %s", text)
	}

	ok := waitUntil(t, time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return savedToken == "late-tok"
	})
	if !ok {
		t.Fatal("expected the background goroutine to save the token that arrived after the blocking window")
	}
	if !waitUntil(t, time.Second, fake.isClosed) {
		t.Fatal("expected the callback server to be closed after the background flow completed")
	}
}

func TestLoginHandler_Reentrant_SameURLNoSecondServer(t *testing.T) {
	saveAndRestore(t)
	saveAndRestoreLoginDeps(t)
	setNoAuth()

	loginGenerateState = func() (string, error) { return "reentrant-state", nil }
	fake := &fakeCallbackServer{port: 4242, blockOnCtx: true}
	var callMu sync.Mutex
	serverCalls := 0
	loginNewCallbackServer = func(state string) authCallbackServer {
		callMu.Lock()
		serverCalls++
		callMu.Unlock()
		return fake
	}
	loginOpenBrowser = func(url string) error { return nil }
	loginBlockTimeout = 30 * time.Millisecond
	loginBackgroundTimeout = 200 * time.Millisecond

	result1, err1 := loginHandler(context.Background(), makeReq(nil))
	text1 := assertSuccess(t, result1, err1)

	result2, err2 := loginHandler(context.Background(), makeReq(nil))
	text2 := assertSuccess(t, result2, err2)

	callMu.Lock()
	calls := serverCalls
	callMu.Unlock()
	if calls != 1 {
		t.Fatalf("expected exactly 1 callback server across concurrent logins, got %d", calls)
	}
	if text1 != text2 {
		t.Fatalf("expected re-entrant login to return the same URL text:\n1: %s\n2: %s", text1, text2)
	}

	waitUntil(t, time.Second, fake.isClosed)
}

// --- adjacent error paths (mirrors cmd/login.Deps error coverage) ---

func TestLoginHandler_GenerateStateError(t *testing.T) {
	saveAndRestore(t)
	saveAndRestoreLoginDeps(t)
	setNoAuth()
	loginGenerateState = func() (string, error) { return "", fmt.Errorf("entropy fail") }

	result, err := loginHandler(context.Background(), makeReq(nil))
	assertError(t, result, err, "failed to start login")
}

func TestLoginHandler_ServerStartError(t *testing.T) {
	saveAndRestore(t)
	saveAndRestoreLoginDeps(t)
	setNoAuth()
	loginGenerateState = func() (string, error) { return "s", nil }
	loginNewCallbackServer = func(state string) authCallbackServer {
		return &fakeCallbackServer{startErr: fmt.Errorf("port in use")}
	}

	result, err := loginHandler(context.Background(), makeReq(nil))
	assertError(t, result, err, "failed to start login")
}

func TestLoginHandler_RetriesAfterStartError(t *testing.T) {
	saveAndRestore(t)
	saveAndRestoreLoginDeps(t)
	setNoAuth()
	newMockServer(t, fakeEnergyRoute())

	attempt := 0
	loginGenerateState = func() (string, error) { return "retry-state", nil }
	loginNewCallbackServer = func(state string) authCallbackServer {
		attempt++
		if attempt == 1 {
			return &fakeCallbackServer{startErr: fmt.Errorf("port in use")}
		}
		return &fakeCallbackServer{port: 111, token: "retry-tok"}
	}
	loginOpenBrowser = func(url string) error { return nil }
	loginSaveToken = func(token string) error { return nil }

	result1, err1 := loginHandler(context.Background(), makeReq(nil))
	assertError(t, result1, err1, "failed to start login")

	result2, err2 := loginHandler(context.Background(), makeReq(nil))
	text2 := assertSuccess(t, result2, err2)
	if !strings.Contains(text2, "free") {
		t.Errorf("expected a successful retry after a start error, got: %s", text2)
	}
	if attempt != 2 {
		t.Fatalf("expected a fresh callback server attempt on retry, got %d attempts", attempt)
	}
}
