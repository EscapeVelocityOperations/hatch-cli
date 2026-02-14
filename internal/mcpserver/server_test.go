package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/mark3labs/mcp-go/mcp"
)

// --- Test helpers ---

// saveAndRestore saves the current DI variables and returns a cleanup function.
func saveAndRestore(t *testing.T) {
	t.Helper()
	origGetToken := getTokenFunc
	origNewAPI := newAPIClient
	t.Cleanup(func() {
		getTokenFunc = origGetToken
		newAPIClient = origNewAPI
	})
}

// setAuthToken configures getTokenFunc to return a fixed token.
func setAuthToken(token string) {
	getTokenFunc = func() (string, error) { return token, nil }
}

// setAuthError configures getTokenFunc to return an error.
func setAuthError(err error) {
	getTokenFunc = func() (string, error) { return "", err }
}

// setNoAuth configures getTokenFunc to return an empty token (not logged in).
func setNoAuth() {
	getTokenFunc = func() (string, error) { return "", nil }
}

// newMockServer creates an httptest.Server and configures newAPIClient to point to it.
// The handler map keys are "METHOD /path" strings.
func newMockServer(t *testing.T, routes map[string]http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		if h, ok := routes[key]; ok {
			h(w, r)
			return
		}
		// Also try with query string stripped
		pathOnly := r.URL.Path
		keyNoQuery := r.Method + " " + pathOnly
		if h, ok := routes[keyNoQuery]; ok {
			h(w, r)
			return
		}
		t.Logf("unhandled request: %s %s", r.Method, r.URL.String())
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	newAPIClient = func(token string) *api.Client {
		return api.NewTestClient(token, srv.URL)
	}
	return srv
}

// jsonHandler returns an http.HandlerFunc that writes status 200 and JSON body.
func jsonHandler(v interface{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(v)
	}
}

// errorHandler returns an http.HandlerFunc that writes the given HTTP status and message.
func errorHandler(status int, msg string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		w.Write([]byte(msg))
	}
}

// makeReq creates a CallToolRequest with the given arguments.
func makeReq(args map[string]interface{}) mcp.CallToolRequest {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = args
	return req
}

// resultText extracts the text content from a CallToolResult.
func resultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) == 0 {
		return ""
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	return tc.Text
}

// assertError checks that the result is an error and contains the expected substring.
func assertError(t *testing.T, result *mcp.CallToolResult, err error, substr string) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error result, got success: %s", resultText(t, result))
	}
	text := resultText(t, result)
	if !strings.Contains(text, substr) {
		t.Fatalf("expected error containing %q, got: %s", substr, text)
	}
}

// assertSuccess checks that the result is not an error.
func assertSuccess(t *testing.T, result *mcp.CallToolResult, err error) string {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", resultText(t, result))
	}
	return resultText(t, result)
}

// --- NewServer ---

func TestNewServer(t *testing.T) {
	s := NewServer()
	if s == nil {
		t.Fatal("NewServer returned nil")
	}
}

// --- get_platform_info ---

func TestGetPlatformInfoHandler(t *testing.T) {
	req := mcp.CallToolRequest{}
	result, err := getPlatformInfoHandler(context.Background(), req)
	text := assertSuccess(t, result, err)

	// Should contain framework info
	if !strings.Contains(text, "static") {
		t.Error("expected 'static' framework in output")
	}
	if !strings.Contains(text, "node") {
		t.Error("expected 'node' framework in output")
	}
	if !strings.Contains(text, "deploy") {
		t.Error("expected deploy info in output")
	}
}

// --- check_auth ---

func TestCheckAuthHandler_Success(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("hatch_test_token123")

	result, err := checkAuthHandler(context.Background(), makeReq(nil))
	text := assertSuccess(t, result, err)

	if !strings.Contains(text, "Authenticated") {
		t.Errorf("expected 'Authenticated' in response, got: %s", text)
	}
}

func TestCheckAuthHandler_NoToken(t *testing.T) {
	saveAndRestore(t)
	setNoAuth()

	result, err := checkAuthHandler(context.Background(), makeReq(nil))
	assertError(t, result, err, "not authenticated")
}

func TestCheckAuthHandler_TokenError(t *testing.T) {
	saveAndRestore(t)
	setAuthError(fmt.Errorf("disk read error"))

	result, err := checkAuthHandler(context.Background(), makeReq(nil))
	assertError(t, result, err, "disk read error")
}

func TestCheckAuthHandler_TokenRedaction(t *testing.T) {
	saveAndRestore(t)
	setAuthError(fmt.Errorf("bad token: hatch_secretABC123"))

	result, err := checkAuthHandler(context.Background(), makeReq(nil))
	assertError(t, result, err, "hatch_****")
	text := resultText(t, result)
	if strings.Contains(text, "hatch_secretABC123") {
		t.Error("token was NOT redacted in error message")
	}
}

// --- list_apps ---

func TestListAppsHandler_Success(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("tok")
	newMockServer(t, map[string]http.HandlerFunc{
		"GET /v1/apps": jsonHandler([]api.App{
			{Slug: "myapp-a1b2", Name: "myapp", Status: "running", URL: "https://myapp-a1b2.nest.gethatch.eu"},
		}),
	})

	result, err := listAppsHandler(context.Background(), makeReq(nil))
	text := assertSuccess(t, result, err)

	if !strings.Contains(text, "myapp-a1b2") {
		t.Errorf("expected slug in output, got: %s", text)
	}
}

func TestListAppsHandler_Empty(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("tok")
	newMockServer(t, map[string]http.HandlerFunc{
		"GET /v1/apps": jsonHandler([]api.App{}),
	})

	result, err := listAppsHandler(context.Background(), makeReq(nil))
	text := assertSuccess(t, result, err)

	if !strings.Contains(text, "No apps found") {
		t.Errorf("expected 'No apps found' message, got: %s", text)
	}
}

func TestListAppsHandler_AuthFailure(t *testing.T) {
	saveAndRestore(t)
	setNoAuth()

	result, err := listAppsHandler(context.Background(), makeReq(nil))
	assertError(t, result, err, "not authenticated")
}

func TestListAppsHandler_APIError(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("tok")
	newMockServer(t, map[string]http.HandlerFunc{
		"GET /v1/apps": errorHandler(500, "internal server error"),
	})

	result, err := listAppsHandler(context.Background(), makeReq(nil))
	assertError(t, result, err, "failed to list apps")
}

// --- get_status ---

func TestGetStatusHandler_MissingApp(t *testing.T) {
	result, err := getStatusHandler(context.Background(), makeReq(map[string]interface{}{}))
	assertError(t, result, err, "missing required parameter")
}

func TestGetStatusHandler_Success(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("tok")
	now := time.Now()
	newMockServer(t, map[string]http.HandlerFunc{
		"GET /v1/apps/myapp-a1b2": jsonHandler(api.App{
			Slug: "myapp-a1b2", Name: "myapp", Status: "running",
			URL: "https://myapp-a1b2.nest.gethatch.eu", Region: "eu",
			CreatedAt: now, UpdatedAt: now,
		}),
	})

	result, err := getStatusHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
	}))
	text := assertSuccess(t, result, err)

	if !strings.Contains(text, "running") {
		t.Errorf("expected 'running' status, got: %s", text)
	}
	if !strings.Contains(text, "myapp") {
		t.Errorf("expected app name, got: %s", text)
	}
}

func TestGetStatusHandler_AuthFailure(t *testing.T) {
	saveAndRestore(t)
	setNoAuth()

	result, err := getStatusHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
	}))
	assertError(t, result, err, "not authenticated")
}

func TestGetStatusHandler_APIError(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("tok")
	newMockServer(t, map[string]http.HandlerFunc{
		"GET /v1/apps/myapp-a1b2": errorHandler(404, "app not found"),
	})

	result, err := getStatusHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
	}))
	assertError(t, result, err, "failed to get app status")
}

// --- create_app ---

func TestCreateAppHandler_MissingName(t *testing.T) {
	result, err := createAppHandler(context.Background(), makeReq(map[string]interface{}{}))
	assertError(t, result, err, "missing required parameter")
}

func TestCreateAppHandler_Success(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("tok")
	newMockServer(t, map[string]http.HandlerFunc{
		"POST /v1/apps": jsonHandler(api.App{Slug: "testapp-x1y2", Name: "testapp"}),
	})

	result, err := createAppHandler(context.Background(), makeReq(map[string]interface{}{
		"name": "testapp",
	}))
	text := assertSuccess(t, result, err)

	if !strings.Contains(text, "testapp-x1y2") {
		t.Errorf("expected slug in output, got: %s", text)
	}
}

func TestCreateAppHandler_AuthFailure(t *testing.T) {
	saveAndRestore(t)
	setNoAuth()

	result, err := createAppHandler(context.Background(), makeReq(map[string]interface{}{
		"name": "testapp",
	}))
	assertError(t, result, err, "not authenticated")
}

func TestCreateAppHandler_APIError(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("tok")
	newMockServer(t, map[string]http.HandlerFunc{
		"POST /v1/apps": errorHandler(422, "name already taken"),
	})

	result, err := createAppHandler(context.Background(), makeReq(map[string]interface{}{
		"name": "testapp",
	}))
	assertError(t, result, err, "failed to create app")
}

// --- delete_app ---

func TestDeleteAppHandler_MissingApp(t *testing.T) {
	result, err := deleteAppHandler(context.Background(), makeReq(map[string]interface{}{}))
	assertError(t, result, err, "missing required parameter")
}

func TestDeleteAppHandler_NoConfirm(t *testing.T) {
	result, err := deleteAppHandler(context.Background(), makeReq(map[string]interface{}{
		"app":     "myapp-a1b2",
		"confirm": false,
	}))
	assertError(t, result, err, "confirm must be set to true")
}

func TestDeleteAppHandler_Success(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("tok")
	newMockServer(t, map[string]http.HandlerFunc{
		"DELETE /v1/apps/myapp-a1b2": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	})

	result, err := deleteAppHandler(context.Background(), makeReq(map[string]interface{}{
		"app":     "myapp-a1b2",
		"confirm": true,
	}))
	text := assertSuccess(t, result, err)

	if !strings.Contains(text, "permanently deleted") {
		t.Errorf("expected deletion confirmation, got: %s", text)
	}
}

func TestDeleteAppHandler_AuthFailure(t *testing.T) {
	saveAndRestore(t)
	setNoAuth()

	result, err := deleteAppHandler(context.Background(), makeReq(map[string]interface{}{
		"app":     "myapp-a1b2",
		"confirm": true,
	}))
	assertError(t, result, err, "not authenticated")
}

func TestDeleteAppHandler_APIError(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("tok")
	newMockServer(t, map[string]http.HandlerFunc{
		"DELETE /v1/apps/myapp-a1b2": errorHandler(404, "app not found"),
	})

	result, err := deleteAppHandler(context.Background(), makeReq(map[string]interface{}{
		"app":     "myapp-a1b2",
		"confirm": true,
	}))
	assertError(t, result, err, "failed to delete app")
}

// --- restart_app ---

func TestRestartAppHandler_MissingApp(t *testing.T) {
	result, err := restartAppHandler(context.Background(), makeReq(map[string]interface{}{}))
	assertError(t, result, err, "missing required parameter")
}

func TestRestartAppHandler_Success(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("tok")
	newMockServer(t, map[string]http.HandlerFunc{
		"POST /v1/apps/myapp-a1b2/restart": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	})

	result, err := restartAppHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
	}))
	text := assertSuccess(t, result, err)

	if !strings.Contains(text, "restarted successfully") {
		t.Errorf("expected restart confirmation, got: %s", text)
	}
}

func TestRestartAppHandler_AuthFailure(t *testing.T) {
	saveAndRestore(t)
	setNoAuth()

	result, err := restartAppHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
	}))
	assertError(t, result, err, "not authenticated")
}

func TestRestartAppHandler_APIError(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("tok")
	newMockServer(t, map[string]http.HandlerFunc{
		"POST /v1/apps/myapp-a1b2/restart": errorHandler(500, "restart failed"),
	})

	result, err := restartAppHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
	}))
	assertError(t, result, err, "failed to restart app")
}

// --- set_env ---

func TestSetEnvHandler_MissingParams(t *testing.T) {
	result, err := setEnvHandler(context.Background(), makeReq(map[string]interface{}{}))
	assertError(t, result, err, "missing required parameter")
}

func TestSetEnvHandler_MissingKey(t *testing.T) {
	result, err := setEnvHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
	}))
	assertError(t, result, err, "missing required parameter")
}

func TestSetEnvHandler_MissingValue(t *testing.T) {
	result, err := setEnvHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
		"key": "FOO",
	}))
	assertError(t, result, err, "missing required parameter")
}

func TestSetEnvHandler_Success(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("tok")
	newMockServer(t, map[string]http.HandlerFunc{
		"POST /v1/apps/myapp-a1b2/env": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	})

	result, err := setEnvHandler(context.Background(), makeReq(map[string]interface{}{
		"app":   "myapp-a1b2",
		"key":   "DATABASE_URL",
		"value": "postgres://localhost/db",
	}))
	text := assertSuccess(t, result, err)

	if !strings.Contains(text, "DATABASE_URL") {
		t.Errorf("expected key in output, got: %s", text)
	}
}

func TestSetEnvHandler_AuthFailure(t *testing.T) {
	saveAndRestore(t)
	setNoAuth()

	result, err := setEnvHandler(context.Background(), makeReq(map[string]interface{}{
		"app":   "myapp-a1b2",
		"key":   "FOO",
		"value": "bar",
	}))
	assertError(t, result, err, "not authenticated")
}

// --- get_env ---

func TestGetEnvHandler_MissingApp(t *testing.T) {
	result, err := getEnvHandler(context.Background(), makeReq(map[string]interface{}{}))
	assertError(t, result, err, "missing required parameter")
}

func TestGetEnvHandler_Success(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("tok")
	newMockServer(t, map[string]http.HandlerFunc{
		"GET /v1/apps/myapp-a1b2/env": jsonHandler([]api.EnvVar{
			{Key: "DATABASE_URL", Value: "postgres://localhost/db"},
			{Key: "NODE_ENV", Value: "production"},
		}),
	})

	result, err := getEnvHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
	}))
	text := assertSuccess(t, result, err)

	if !strings.Contains(text, "DATABASE_URL") {
		t.Errorf("expected DATABASE_URL in output, got: %s", text)
	}
}

func TestGetEnvHandler_Empty(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("tok")
	newMockServer(t, map[string]http.HandlerFunc{
		"GET /v1/apps/myapp-a1b2/env": jsonHandler([]api.EnvVar{}),
	})

	result, err := getEnvHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
	}))
	text := assertSuccess(t, result, err)

	if !strings.Contains(text, "No environment variables") {
		t.Errorf("expected empty message, got: %s", text)
	}
}

func TestGetEnvHandler_AuthFailure(t *testing.T) {
	saveAndRestore(t)
	setNoAuth()

	result, err := getEnvHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
	}))
	assertError(t, result, err, "not authenticated")
}

// --- delete_env ---

func TestDeleteEnvHandler_MissingParams(t *testing.T) {
	result, err := deleteEnvHandler(context.Background(), makeReq(map[string]interface{}{}))
	assertError(t, result, err, "missing required parameter")
}

func TestDeleteEnvHandler_MissingKey(t *testing.T) {
	result, err := deleteEnvHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
	}))
	assertError(t, result, err, "missing required parameter")
}

func TestDeleteEnvHandler_InvalidKey(t *testing.T) {
	result, err := deleteEnvHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
		"key": "../../../etc/passwd",
	}))
	assertError(t, result, err, "invalid env key")
}

func TestDeleteEnvHandler_Success(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("tok")
	newMockServer(t, map[string]http.HandlerFunc{
		"DELETE /v1/apps/myapp-a1b2/env/MY_VAR": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	})

	result, err := deleteEnvHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
		"key": "MY_VAR",
	}))
	text := assertSuccess(t, result, err)

	if !strings.Contains(text, "MY_VAR") {
		t.Errorf("expected key in output, got: %s", text)
	}
}

func TestDeleteEnvHandler_AuthFailure(t *testing.T) {
	saveAndRestore(t)
	setNoAuth()

	result, err := deleteEnvHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
		"key": "MY_VAR",
	}))
	assertError(t, result, err, "not authenticated")
}

func TestDeleteEnvHandler_APIError(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("tok")
	newMockServer(t, map[string]http.HandlerFunc{
		"DELETE /v1/apps/myapp-a1b2/env/MY_VAR": errorHandler(404, "not found"),
	})

	result, err := deleteEnvHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
		"key": "MY_VAR",
	}))
	assertError(t, result, err, "failed to delete env var")
}

// --- add_domain ---

func TestAddDomainHandler_MissingParams(t *testing.T) {
	result, err := addDomainHandler(context.Background(), makeReq(map[string]interface{}{}))
	assertError(t, result, err, "missing required parameter")
}

func TestAddDomainHandler_MissingDomain(t *testing.T) {
	result, err := addDomainHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
	}))
	assertError(t, result, err, "missing required parameter")
}

func TestAddDomainHandler_InvalidDomain(t *testing.T) {
	result, err := addDomainHandler(context.Background(), makeReq(map[string]interface{}{
		"app":    "myapp-a1b2",
		"domain": "example.com/../../etc",
	}))
	assertError(t, result, err, "invalid domain")
}

func TestAddDomainHandler_Success(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("tok")
	newMockServer(t, map[string]http.HandlerFunc{
		"POST /v1/apps/myapp-a1b2/domains": jsonHandler(api.Domain{
			Domain: "example.com",
			Status: "pending",
			CNAME:  "myapp-a1b2.nest.gethatch.eu",
		}),
	})

	result, err := addDomainHandler(context.Background(), makeReq(map[string]interface{}{
		"app":    "myapp-a1b2",
		"domain": "example.com",
	}))
	text := assertSuccess(t, result, err)

	if !strings.Contains(text, "example.com") {
		t.Errorf("expected domain in output, got: %s", text)
	}
	if !strings.Contains(text, "CNAME") {
		t.Errorf("expected DNS instructions, got: %s", text)
	}
}

func TestAddDomainHandler_AuthFailure(t *testing.T) {
	saveAndRestore(t)
	setNoAuth()

	result, err := addDomainHandler(context.Background(), makeReq(map[string]interface{}{
		"app":    "myapp-a1b2",
		"domain": "example.com",
	}))
	assertError(t, result, err, "not authenticated")
}

// --- list_domains ---

func TestListDomainsHandler_MissingApp(t *testing.T) {
	result, err := listDomainsHandler(context.Background(), makeReq(map[string]interface{}{}))
	assertError(t, result, err, "missing required parameter")
}

func TestListDomainsHandler_Success(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("tok")
	newMockServer(t, map[string]http.HandlerFunc{
		"GET /v1/apps/myapp-a1b2/domains": jsonHandler([]api.Domain{
			{Domain: "example.com", Status: "active", CNAME: "myapp-a1b2.nest.gethatch.eu"},
		}),
	})

	result, err := listDomainsHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
	}))
	text := assertSuccess(t, result, err)

	if !strings.Contains(text, "example.com") {
		t.Errorf("expected domain in output, got: %s", text)
	}
}

func TestListDomainsHandler_Empty(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("tok")
	newMockServer(t, map[string]http.HandlerFunc{
		"GET /v1/apps/myapp-a1b2/domains": jsonHandler([]api.Domain{}),
	})

	result, err := listDomainsHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
	}))
	text := assertSuccess(t, result, err)

	if !strings.Contains(text, "No custom domains") {
		t.Errorf("expected empty message, got: %s", text)
	}
}

func TestListDomainsHandler_AuthFailure(t *testing.T) {
	saveAndRestore(t)
	setNoAuth()

	result, err := listDomainsHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
	}))
	assertError(t, result, err, "not authenticated")
}

// --- remove_domain ---

func TestRemoveDomainHandler_MissingParams(t *testing.T) {
	result, err := removeDomainHandler(context.Background(), makeReq(map[string]interface{}{}))
	assertError(t, result, err, "missing required parameter")
}

func TestRemoveDomainHandler_InvalidDomain(t *testing.T) {
	result, err := removeDomainHandler(context.Background(), makeReq(map[string]interface{}{
		"app":    "myapp-a1b2",
		"domain": "evil.com?inject=true",
	}))
	assertError(t, result, err, "invalid domain")
}

func TestRemoveDomainHandler_Success(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("tok")
	newMockServer(t, map[string]http.HandlerFunc{
		"DELETE /v1/apps/myapp-a1b2/domains/example.com": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	})

	result, err := removeDomainHandler(context.Background(), makeReq(map[string]interface{}{
		"app":    "myapp-a1b2",
		"domain": "example.com",
	}))
	text := assertSuccess(t, result, err)

	if !strings.Contains(text, "removed") {
		t.Errorf("expected removal confirmation, got: %s", text)
	}
}

func TestRemoveDomainHandler_AuthFailure(t *testing.T) {
	saveAndRestore(t)
	setNoAuth()

	result, err := removeDomainHandler(context.Background(), makeReq(map[string]interface{}{
		"app":    "myapp-a1b2",
		"domain": "example.com",
	}))
	assertError(t, result, err, "not authenticated")
}

// --- get_logs ---

func TestGetLogsHandler_MissingApp(t *testing.T) {
	result, err := getLogsHandler(context.Background(), makeReq(map[string]interface{}{}))
	assertError(t, result, err, "missing required parameter")
}

func TestGetLogsHandler_Success(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("tok")
	newMockServer(t, map[string]http.HandlerFunc{
		"GET /v1/apps/myapp-a1b2/logs": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"lines":["[2024-01-01] Starting app","[2024-01-01] Listening on :8080"]}`))
		},
	})

	result, err := getLogsHandler(context.Background(), makeReq(map[string]interface{}{
		"app":   "myapp-a1b2",
		"lines": float64(50),
	}))
	text := assertSuccess(t, result, err)

	if !strings.Contains(text, "Starting app") {
		t.Errorf("expected log content, got: %s", text)
	}
}

func TestGetLogsHandler_Empty(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("tok")
	newMockServer(t, map[string]http.HandlerFunc{
		"GET /v1/apps/myapp-a1b2/logs": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"lines":[]}`))
		},
	})

	result, err := getLogsHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
	}))
	text := assertSuccess(t, result, err)

	if !strings.Contains(text, "No recent logs") {
		t.Errorf("expected empty message, got: %s", text)
	}
}

func TestGetLogsHandler_AuthFailure(t *testing.T) {
	saveAndRestore(t)
	setNoAuth()

	result, err := getLogsHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
	}))
	assertError(t, result, err, "not authenticated")
}

// --- get_build_logs ---

func TestGetBuildLogsHandler_MissingApp(t *testing.T) {
	result, err := getBuildLogsHandler(context.Background(), makeReq(map[string]interface{}{}))
	assertError(t, result, err, "missing required parameter")
}

func TestGetBuildLogsHandler_Success(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("tok")
	newMockServer(t, map[string]http.HandlerFunc{
		"GET /v1/apps/myapp-a1b2/logs": func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("type") != "build" {
				t.Error("expected type=build query parameter")
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"lines":["Building image...","Build complete"]}`))
		},
	})

	result, err := getBuildLogsHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
	}))
	text := assertSuccess(t, result, err)

	if !strings.Contains(text, "Building image") {
		t.Errorf("expected build log content, got: %s", text)
	}
}

func TestGetBuildLogsHandler_AuthFailure(t *testing.T) {
	saveAndRestore(t)
	setNoAuth()

	result, err := getBuildLogsHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
	}))
	assertError(t, result, err, "not authenticated")
}

// --- add_database ---

func TestAddDatabaseHandler_MissingApp(t *testing.T) {
	result, err := addDatabaseHandler(context.Background(), makeReq(map[string]interface{}{}))
	assertError(t, result, err, "missing required parameter")
}

func TestAddDatabaseHandler_Success(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("tok")
	newMockServer(t, map[string]http.HandlerFunc{
		"POST /v1/apps/myapp-a1b2/addons": jsonHandler(api.Addon{
			Type:   "postgresql",
			Status: "provisioned",
			URL:    "postgres://user:pass@localhost/db",
		}),
	})

	result, err := addDatabaseHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
	}))
	text := assertSuccess(t, result, err)

	if !strings.Contains(text, "provisioned") {
		t.Errorf("expected provisioned status, got: %s", text)
	}
	if !strings.Contains(text, "DATABASE_URL") {
		t.Errorf("expected DATABASE_URL in output, got: %s", text)
	}
}

func TestAddDatabaseHandler_AuthFailure(t *testing.T) {
	saveAndRestore(t)
	setNoAuth()

	result, err := addDatabaseHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
	}))
	assertError(t, result, err, "not authenticated")
}

func TestAddDatabaseHandler_APIError(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("tok")
	newMockServer(t, map[string]http.HandlerFunc{
		"POST /v1/apps/myapp-a1b2/addons": errorHandler(500, "provisioning failed"),
	})

	result, err := addDatabaseHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
	}))
	assertError(t, result, err, "failed to add database")
}

// --- add_storage ---

func TestAddStorageHandler_MissingApp(t *testing.T) {
	result, err := addStorageHandler(context.Background(), makeReq(map[string]interface{}{}))
	assertError(t, result, err, "missing required parameter")
}

func TestAddStorageHandler_Success(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("tok")
	newMockServer(t, map[string]http.HandlerFunc{
		"POST /v1/apps/myapp-a1b2/addons": jsonHandler(api.Addon{
			Type:   "s3",
			Status: "provisioned",
			URL:    "https://s3.example.com/mybucket",
		}),
	})

	result, err := addStorageHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
	}))
	text := assertSuccess(t, result, err)

	if !strings.Contains(text, "provisioned") {
		t.Errorf("expected provisioned status, got: %s", text)
	}
}

func TestAddStorageHandler_AuthFailure(t *testing.T) {
	saveAndRestore(t)
	setNoAuth()

	result, err := addStorageHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
	}))
	assertError(t, result, err, "not authenticated")
}

// --- get_database_url ---

func TestGetDatabaseURLHandler_MissingApp(t *testing.T) {
	result, err := getDatabaseURLHandler(context.Background(), makeReq(map[string]interface{}{}))
	assertError(t, result, err, "missing required parameter")
}

func TestGetDatabaseURLHandler_Success(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("tok")
	newMockServer(t, map[string]http.HandlerFunc{
		"GET /v1/apps/myapp-a1b2/env": jsonHandler([]api.EnvVar{
			{Key: "DATABASE_URL", Value: "postgres://user:pass@host/db"},
			{Key: "NODE_ENV", Value: "production"},
		}),
	})

	result, err := getDatabaseURLHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
	}))
	text := assertSuccess(t, result, err)

	if text != "postgres://user:pass@host/db" {
		t.Errorf("expected DATABASE_URL value, got: %s", text)
	}
}

func TestGetDatabaseURLHandler_NoDatabaseURL(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("tok")
	newMockServer(t, map[string]http.HandlerFunc{
		"GET /v1/apps/myapp-a1b2/env": jsonHandler([]api.EnvVar{
			{Key: "NODE_ENV", Value: "production"},
		}),
	})

	result, err := getDatabaseURLHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
	}))
	assertError(t, result, err, "no DATABASE_URL found")
}

func TestGetDatabaseURLHandler_AuthFailure(t *testing.T) {
	saveAndRestore(t)
	setNoAuth()

	result, err := getDatabaseURLHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
	}))
	assertError(t, result, err, "not authenticated")
}

// --- get_app_details ---

func TestGetAppDetailsHandler_MissingApp(t *testing.T) {
	result, err := getAppDetailsHandler(context.Background(), makeReq(map[string]interface{}{}))
	assertError(t, result, err, "missing required parameter")
}

func TestGetAppDetailsHandler_Success(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("tok")
	now := time.Now()
	newMockServer(t, map[string]http.HandlerFunc{
		"GET /v1/apps/myapp-a1b2": jsonHandler(api.App{
			Slug: "myapp-a1b2", Name: "myapp", Status: "running",
			URL: "https://myapp-a1b2.nest.gethatch.eu",
			CreatedAt: now, UpdatedAt: now,
		}),
	})

	result, err := getAppDetailsHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
	}))
	text := assertSuccess(t, result, err)

	if !strings.Contains(text, "myapp-a1b2") {
		t.Errorf("expected slug in output, got: %s", text)
	}
}

func TestGetAppDetailsHandler_AuthFailure(t *testing.T) {
	saveAndRestore(t)
	setNoAuth()

	result, err := getAppDetailsHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
	}))
	assertError(t, result, err, "not authenticated")
}

// --- health_check ---

func TestHealthCheckHandler_MissingApp(t *testing.T) {
	result, err := healthCheckHandler(context.Background(), makeReq(map[string]interface{}{}))
	assertError(t, result, err, "missing required parameter")
}

func TestHealthCheckHandler_InvalidSlug(t *testing.T) {
	result, err := healthCheckHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "../../../etc/passwd",
	}))
	assertError(t, result, err, "invalid slug")
}

// Note: healthCheckHandler makes a real HTTP request to the app URL,
// so we only test parameter validation and slug validation here.
// Integration tests would be needed for the actual health check.

// --- bulk_set_env ---

func TestBulkSetEnvHandler_MissingApp(t *testing.T) {
	result, err := bulkSetEnvHandler(context.Background(), makeReq(map[string]interface{}{}))
	assertError(t, result, err, "missing required parameter")
}

func TestBulkSetEnvHandler_MissingVars(t *testing.T) {
	result, err := bulkSetEnvHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
	}))
	assertError(t, result, err, "missing required parameter 'vars'")
}

func TestBulkSetEnvHandler_EmptyVars(t *testing.T) {
	result, err := bulkSetEnvHandler(context.Background(), makeReq(map[string]interface{}{
		"app":  "myapp-a1b2",
		"vars": map[string]interface{}{},
	}))
	assertError(t, result, err, "cannot be empty")
}

func TestBulkSetEnvHandler_Success(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("tok")
	newMockServer(t, map[string]http.HandlerFunc{
		"POST /v1/apps/myapp-a1b2/env": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	})

	result, err := bulkSetEnvHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
		"vars": map[string]interface{}{
			"FOO": "bar",
			"BAZ": "qux",
		},
	}))
	text := assertSuccess(t, result, err)

	if !strings.Contains(text, "Set 2 environment variables") {
		t.Errorf("expected bulk set confirmation, got: %s", text)
	}
}

func TestBulkSetEnvHandler_AuthFailure(t *testing.T) {
	saveAndRestore(t)
	setNoAuth()

	result, err := bulkSetEnvHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
		"vars": map[string]interface{}{
			"FOO": "bar",
		},
	}))
	assertError(t, result, err, "not authenticated")
}

// --- deploy_app ---

func TestDeployAppHandler_MissingDeployTarget(t *testing.T) {
	result, err := deployAppHandler(context.Background(), makeReq(map[string]interface{}{}))
	assertError(t, result, err, "missing required parameter")
}

func TestDeployAppHandler_MissingRuntime(t *testing.T) {
	result, err := deployAppHandler(context.Background(), makeReq(map[string]interface{}{
		"deploy_target": "/tmp",
	}))
	assertError(t, result, err, "missing required parameter")
}

func TestDeployAppHandler_InvalidRuntime(t *testing.T) {
	result, err := deployAppHandler(context.Background(), makeReq(map[string]interface{}{
		"deploy_target": "/tmp",
		"runtime":       "invalid-fw",
	}))
	assertError(t, result, err, "unknown runtime")
}

func TestDeployAppHandler_NonStaticMissingStartCmd(t *testing.T) {
	result, err := deployAppHandler(context.Background(), makeReq(map[string]interface{}{
		"deploy_target": "/tmp",
		"runtime":       "node",
	}))
	assertError(t, result, err, "start_command is required")
}

// --- skill resource ---

func TestSkillResourceHandler(t *testing.T) {
	req := mcp.ReadResourceRequest{}
	contents, err := skillResourceHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(contents) != 1 {
		t.Fatalf("expected 1 resource content, got %d", len(contents))
	}
	text, ok := contents[0].(mcp.TextResourceContents)
	if !ok {
		t.Fatal("expected TextResourceContents")
	}
	if text.URI != "hatch://skill" {
		t.Fatalf("expected URI hatch://skill, got %s", text.URI)
	}
	if text.Text == "" {
		t.Fatal("expected non-empty skill content")
	}
}

// --- Error format consistency ---

func TestErrorFormatConsistency(t *testing.T) {
	// Verify that all error paths use the "failed to" prefix format
	saveAndRestore(t)
	setNoAuth()

	handlers := []struct {
		name    string
		handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)
		args    map[string]interface{}
	}{
		{"listApps", listAppsHandler, nil},
		{"getStatus", getStatusHandler, map[string]interface{}{"app": "test-a1b2"}},
		{"createApp", createAppHandler, map[string]interface{}{"name": "test"}},
		{"deleteApp", deleteAppHandler, map[string]interface{}{"app": "test-a1b2", "confirm": true}},
		{"restartApp", restartAppHandler, map[string]interface{}{"app": "test-a1b2"}},
		{"setEnv", setEnvHandler, map[string]interface{}{"app": "test-a1b2", "key": "K", "value": "V"}},
		{"getEnv", getEnvHandler, map[string]interface{}{"app": "test-a1b2"}},
		{"deleteEnv", deleteEnvHandler, map[string]interface{}{"app": "test-a1b2", "key": "K"}},
		{"getLogs", getLogsHandler, map[string]interface{}{"app": "test-a1b2"}},
		{"getBuildLogs", getBuildLogsHandler, map[string]interface{}{"app": "test-a1b2"}},
		{"addDatabase", addDatabaseHandler, map[string]interface{}{"app": "test-a1b2"}},
		{"addStorage", addStorageHandler, map[string]interface{}{"app": "test-a1b2"}},
		{"getDatabaseURL", getDatabaseURLHandler, map[string]interface{}{"app": "test-a1b2"}},
		{"addDomain", addDomainHandler, map[string]interface{}{"app": "test-a1b2", "domain": "example.com"}},
		{"listDomains", listDomainsHandler, map[string]interface{}{"app": "test-a1b2"}},
		{"removeDomain", removeDomainHandler, map[string]interface{}{"app": "test-a1b2", "domain": "example.com"}},
		{"getAppDetails", getAppDetailsHandler, map[string]interface{}{"app": "test-a1b2"}},
		{"bulkSetEnv", bulkSetEnvHandler, map[string]interface{}{"app": "test-a1b2", "vars": map[string]interface{}{"K": "V"}}},
	}

	for _, tc := range handlers {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.handler(context.Background(), makeReq(tc.args))
			if err != nil {
				t.Fatalf("unexpected Go error: %v", err)
			}
			if !result.IsError {
				t.Skip("handler did not produce error (may have different code path)")
				return
			}
			text := resultText(t, result)
			if !strings.HasPrefix(text, "failed to") {
				t.Errorf("error does not use 'failed to' prefix: %s", text)
			}
		})
	}
}

// --- Token redaction ---

func TestTokenRedactionInAPIErrors(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("tok")

	// Return an error that contains a token
	newMockServer(t, map[string]http.HandlerFunc{
		"GET /v1/apps": errorHandler(401, "invalid token: hatch_secretToken123"),
	})

	result, err := listAppsHandler(context.Background(), makeReq(nil))
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	text := resultText(t, result)

	if strings.Contains(text, "hatch_secretToken123") {
		t.Error("token was NOT redacted in API error response")
	}
	if !strings.Contains(text, "hatch_****") {
		t.Error("expected redacted token placeholder")
	}
}
