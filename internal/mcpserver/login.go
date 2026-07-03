package mcpserver

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/auth"
	"github.com/mark3labs/mcp-go/mcp"
)

const loginAuthBaseURL = "https://gethatch.eu/cli-auth"

// loginBlockTimeout/loginBackgroundTimeout are vars (not consts) so tests can
// shrink them to make background-goroutine behavior deterministic and fast.
var (
	loginBlockTimeout      = 120 * time.Second
	loginBackgroundTimeout = 300 * time.Second
)

// authCallbackServer is the subset of *auth.CallbackServer's behavior the
// login handler depends on. It extends auth.Server with Port() so the
// handler can build the auth URL after binding an ephemeral port (":0").
type authCallbackServer interface {
	Start() error
	Port() int
	WaitForResult(ctx context.Context) (string, error)
	Close() error
}

// Package-level functions for dependency injection (overridden in tests),
// mirroring the cmd/login.Deps pattern.
var (
	loginGenerateState     = auth.GenerateState
	loginNewCallbackServer = func(state string) authCallbackServer {
		return auth.NewCallbackServer(0, state)
	}
	loginOpenBrowser = auth.OpenBrowser
	loginSaveToken   = auth.SaveToken
)

// loginFlow tracks a single in-flight login so a re-entrant `login` call
// joins the same callback server instead of starting a second one (D1).
type loginFlow struct {
	authURL string
}

var (
	loginFlowMu     sync.Mutex
	activeLoginFlow *loginFlow
)

// --- login ---

func loginTool() mcp.Tool {
	return mcp.NewTool("login",
		mcp.WithDescription("Authenticate with Hatch via browser sign-in or account creation (signup==login). Opens a browser to gethatch.eu; on success returns your plan summary. Call this whenever another tool reports 'not authenticated'."),
	)
}

// loginHandler implements D1: one flow at a time (package-level mutex/state).
// Already-authenticated calls short-circuit to the plan summary. Otherwise a
// callback server is started and handed to a background goroutine with a
// 300s deadline; the handler itself blocks at most 120s (MCP clients time out
// long calls) before falling back to returning the auth URL as text. A
// re-entrant call while a flow is active returns the same URL without
// starting a second server.
func loginHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if token, err := getTokenFunc(); err == nil && token != "" {
		return loginSuccessResult(token), nil
	}

	loginFlowMu.Lock()
	if activeLoginFlow != nil {
		url := activeLoginFlow.authURL
		loginFlowMu.Unlock()
		return mcp.NewToolResultText(loginPendingText(url)), nil
	}
	// Reserve the slot before releasing the lock so a concurrent call can't
	// also pass the nil check and start a second server.
	flow := &loginFlow{}
	activeLoginFlow = flow
	loginFlowMu.Unlock()

	state, err := loginGenerateState()
	if err != nil {
		clearLoginFlow(flow)
		return toolError("failed to start login: %v", err)
	}

	srv := loginNewCallbackServer(state)
	if err := srv.Start(); err != nil {
		clearLoginFlow(flow)
		return toolError("failed to start login: %v", err)
	}

	authURL := fmt.Sprintf("%s?state=%s&port=%d", loginAuthBaseURL, state, srv.Port())
	loginFlowMu.Lock()
	flow.authURL = authURL
	loginFlowMu.Unlock()

	resultCh := make(chan *mcp.CallToolResult, 1)
	go runLoginFlow(srv, flow, resultCh)

	if err := loginOpenBrowser(authURL); err != nil {
		return mcp.NewToolResultText(loginPendingText(authURL)), nil
	}

	select {
	case res := <-resultCh:
		return res, nil
	case <-time.After(loginBlockTimeout):
		return mcp.NewToolResultText(loginPendingText(authURL)), nil
	}
}

// runLoginFlow waits for the OAuth callback (or the background deadline),
// saves the token on success, and always releases the flow slot + closes the
// server — independent of whether the blocking handler above is still
// listening on resultCh (it may have already returned via timeout/headless).
func runLoginFlow(srv authCallbackServer, flow *loginFlow, resultCh chan<- *mcp.CallToolResult) {
	defer srv.Close()
	defer clearLoginFlow(flow)

	ctx, cancel := context.WithTimeout(context.Background(), loginBackgroundTimeout)
	defer cancel()

	token, err := srv.WaitForResult(ctx)
	if err != nil {
		res, _ := toolError("failed to complete login: %v", err)
		resultCh <- res
		return
	}

	if err := loginSaveToken(token); err != nil {
		res, _ := toolError("failed to save login token: %v", err)
		resultCh <- res
		return
	}

	resultCh <- loginSuccessResult(token)
}

func clearLoginFlow(flow *loginFlow) {
	loginFlowMu.Lock()
	if activeLoginFlow == flow {
		activeLoginFlow = nil
	}
	loginFlowMu.Unlock()
}

// loginSuccessResult builds the post-auth summary. It uses the newAPIClient
// seam directly (not newClient()/getTokenFunc()) so a token freshly received
// from the OAuth callback doesn't have to round-trip through disk first.
// Never includes the token itself (telemetry via toolError is a separate,
// error-only path). An energy-fetch failure degrades to a bare
// "Authenticated." — it must never fail the login after the token is saved.
func loginSuccessResult(token string) *mcp.CallToolResult {
	client := newAPIClient(token)
	energy, err := client.GetAccountEnergy()
	if err != nil {
		return mcp.NewToolResultText("Authenticated.")
	}
	return mcp.NewToolResultText(fmt.Sprintf(
		"Authenticated. Plan: %s (daily %d/%d min, eggs %d/%d)",
		energy.Tier, energy.DailyRemaining, energy.DailyLimit,
		energy.EggsActive, energy.EggsLimit))
}

func loginPendingText(url string) string {
	return fmt.Sprintf(
		"Sign in to Hatch: %s\n\nOpen this URL in your browser to sign in or create an account. The callback stays armed for about 5 min — after signing in, call the 'login' tool again to confirm.",
		url)
}
