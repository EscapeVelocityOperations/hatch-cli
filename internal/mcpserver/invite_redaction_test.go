package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/telemetry"
)

// fictiveInviteToken is a made-up invite token used only by these tests.
const fictiveInviteToken = "9e8d7c6b5a4f30211203f4a5b6c7d8e99e8d7c6b5a4f30211203f4a5b6c7d8e9"

// captureTelemetry points telemetry at a local server and returns the
// channel of received event bodies.
func captureTelemetry(t *testing.T) <-chan string {
	t.Helper()
	bodies := make(chan string, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ev telemetry.Event
		_ = json.NewDecoder(r.Body).Decode(&ev)
		data, _ := json.Marshal(ev)
		bodies <- string(data)
		w.WriteHeader(http.StatusCreated)
	}))
	old := telemetry.APIHost
	telemetry.APIHost = srv.URL
	t.Cleanup(func() {
		telemetry.APIHost = old
		srv.Close()
	})
	return bodies
}

// TestAcceptInviteRedactsTransportError (h-e3s1k.1): the connection drops on
// the accept request, before the invitation is consumed. The real token is on
// the request path, but neither the MCP result nor the telemetry body carry it.
func TestAcceptInviteRedactsTransportError(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("hatch_fictive")
	bodies := captureTelemetry(t)

	paths := make(chan string, 4)
	newMockServer(t, map[string]http.HandlerFunc{
		"GET /v1/invitations/pending": func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`[{"id":"row-1","invite_token":"` + fictiveInviteToken + `"}]`))
		},
		"POST /v1/invitations/" + fictiveInviteToken + "/accept": func(w http.ResponseWriter, r *http.Request) {
			paths <- r.URL.Path
			conn, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				_ = conn.Close() // transport failure: no response at all
			}
		},
	})

	for _, ref := range []string{"row-1", fictiveInviteToken} {
		result, err := acceptInviteHandler(context.Background(), makeReq(map[string]interface{}{"token": ref}))
		if err != nil || result == nil || !result.IsError {
			t.Fatalf("ref %q: expected tool error, got result=%+v err=%v", ref, result, err)
		}
		select {
		case gotPath := <-paths:
			if want := "/v1/invitations/" + fictiveInviteToken + "/accept"; gotPath != want {
				t.Errorf("ref %q: request path = %q, want %q", ref, gotPath, want)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("ref %q: accept request not received", ref)
		}
		text := resultText(t, result)
		if strings.Contains(text, fictiveInviteToken) {
			t.Errorf("ref %q: MCP result leaks token: %s", ref, text)
		}
		if !strings.Contains(text, "/invitations/****/accept") {
			t.Errorf("ref %q: MCP result lost redacted URL context: %s", ref, text)
		}
		select {
		case body := <-bodies:
			if strings.Contains(body, fictiveInviteToken) {
				t.Errorf("ref %q: telemetry body leaks token: %s", ref, body)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("ref %q: telemetry not received", ref)
		}
	}
}

// TestListPendingInvitesHidesToken (h-e3s1k.2): the listing exposes the
// invitation id usable with accept_invite, never the invite token.
func TestListPendingInvitesHidesToken(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("hatch_fictive")
	newMockServer(t, map[string]http.HandlerFunc{
		"GET /v1/invitations/pending": func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`[{"id":"row-1","invite_token":"` + fictiveInviteToken + `","app_slug":"their-app"}]`))
		},
	})
	result, err := listPendingInvitesHandler(context.Background(), makeReq(map[string]interface{}{}))
	text := assertSuccess(t, result, err)
	if strings.Contains(text, fictiveInviteToken) || strings.Contains(text, "invite_token") {
		t.Errorf("listing leaks the token: %s", text)
	}
	if !strings.Contains(text, "row-1") {
		t.Errorf("listing missing invitation id: %s", text)
	}
}
