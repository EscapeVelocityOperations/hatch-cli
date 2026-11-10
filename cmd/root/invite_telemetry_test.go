package root

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/config"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/telemetry"
)

// TestInviteTokenExcludedFromTelemetry (h-e3s1k.1): 'hatch invite
// accept|decline <token>' failing before the invitation is consumed (here: not
// logged in) must not ship the token to /telemetry, while other commands keep
// their args.
func TestInviteTokenExcludedFromTelemetry(t *testing.T) {
	const fictive = "0f1e2d3c4b5a69788796a5b4c3d2e1f00f1e2d3c4b5a69788796a5b4c3d2e1f0"

	home := setupTestConfig(t)
	t.Setenv("HATCH_TOKEN", "")
	verbose = false
	tokenFlag = ""
	if err := os.MkdirAll(filepath.Join(home, ".hatch"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := config.Save(&config.Config{TosAcceptedAt: time.Now().UTC().Format(time.RFC3339)}); err != nil {
		t.Fatal(err)
	}

	bodies := make(chan string, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ev telemetry.Event
		_ = json.NewDecoder(r.Body).Decode(&ev)
		data, _ := json.Marshal(ev)
		bodies <- string(data)
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	oldHost := telemetry.APIHost
	telemetry.APIHost = server.URL
	defer func() { telemetry.APIHost = oldHost }()

	for _, action := range []string{"accept", "decline"} {
		_, err := captureExec([]string{"invite", action, fictive})
		if err == nil || !strings.Contains(err.Error(), "not logged in") {
			t.Fatalf("invite %s: err = %v, want not logged in", action, err)
		}
		args := strings.Join(LastArgs(), " ")
		if strings.Contains(args, fictive) || args != "****" {
			t.Errorf("invite %s: LastArgs = %q, want ****", action, args)
		}
		// Same call main makes on error.
		telemetry.Send(LastCommand(), args, err.Error(), LastMode())
		select {
		case body := <-bodies:
			if strings.Contains(body, fictive) {
				t.Errorf("invite %s: telemetry body leaks token: %s", action, body)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("invite %s: telemetry not received", action)
		}
	}

	// Non-sensitive commands still report their args.
	_, _ = captureExec([]string{"collab", "ls", "some-egg"})
	if got := strings.Join(LastArgs(), " "); got != "some-egg" {
		t.Errorf("collab ls LastArgs = %q, want some-egg", got)
	}
}
