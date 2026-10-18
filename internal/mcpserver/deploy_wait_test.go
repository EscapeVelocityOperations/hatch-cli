package mcpserver

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/deploywait"
)

// --- h-59ej: deploy_app waits for the rollout to finish ---

// deployWaitFixture serves an upload plus a GetApp replaying statuses (the
// last one repeats), with a fake clock so the wait never sleeps.
func deployWaitFixture(t *testing.T, reason string, statuses ...string) (dir string, polls *int) {
	t.Helper()
	saveAndRestore(t)
	setAuthToken("tok")

	dir, err := os.MkdirTemp("/tmp", "mcp-deploy-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}

	origOpts := deployWaitOptions
	now := time.Unix(0, 0)
	deployWaitOptions = deploywait.Options{
		Sleep: func(d time.Duration) { now = now.Add(d) },
		Now:   func() time.Time { return now },
	}
	t.Cleanup(func() { deployWaitOptions = origOpts })

	calls := 0
	newMockServer(t, map[string]http.HandlerFunc{
		"POST /v1/apps/site-a1b2/artifact": jsonHandler(map[string]string{"status": "deploying"}),
		"GET /v1/apps/site-a1b2": func(w http.ResponseWriter, r *http.Request) {
			st := statuses[len(statuses)-1]
			if calls < len(statuses) {
				st = statuses[calls]
			}
			calls++
			jsonHandler(api.App{Slug: "site-a1b2", Status: st, StatusReason: reason})(w, r)
		},
	})
	return dir, &calls
}

func deployReq(dir string, extra map[string]interface{}) map[string]interface{} {
	args := map[string]interface{}{"deploy_target": dir, "runtime": "static", "app": "site-a1b2"}
	for k, v := range extra {
		args[k] = v
	}
	return args
}

// A8: running → "Deployed successfully!" only after polling.
func TestDeployAppHandler_WaitsUntilRunning(t *testing.T) {
	dir, polls := deployWaitFixture(t, "", "building", "running")
	result, err := deployAppHandler(context.Background(), makeReq(deployReq(dir, nil)))
	text := assertSuccess(t, result, err)
	if !strings.Contains(text, "Deployed successfully!") || !strings.Contains(text, "Status: running") {
		t.Fatalf("unexpected success text: %s", text)
	}
	if *polls != 2 {
		t.Fatalf("GetApp polled %d times, want 2", *polls)
	}
}

// A8: failed → tool error carrying status_reason.
func TestDeployAppHandler_FailedIsToolError(t *testing.T) {
	dir, _ := deployWaitFixture(t, "Deploy failed: app did not start in time", "building", "failed")
	result, err := deployAppHandler(context.Background(), makeReq(deployReq(dir, nil)))
	assertError(t, result, err, "app did not start in time")
}

func TestDeployAppHandler_WaitFalseSkipsPolling(t *testing.T) {
	dir, polls := deployWaitFixture(t, "", "building")
	result, err := deployAppHandler(context.Background(), makeReq(deployReq(dir, map[string]interface{}{"wait": false})))
	text := assertSuccess(t, result, err)
	if *polls != 0 || strings.Contains(text, "Deployed successfully!") || !strings.Contains(text, "Deploy started") {
		t.Fatalf("wait=false polled %d times, text: %s", *polls, text)
	}
}
