package deploy

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
)

// --- h-59ej: hatch deploy waits for the rollout to finish -----------------

// statusSequence returns a GetApp replaying statuses (the last one repeats)
// and a pointer to the number of calls.
func statusSequence(reason string, statuses ...string) (func(string) (*api.App, error), *int) {
	calls := 0
	return func(slug string) (*api.App, error) {
		st := statuses[len(statuses)-1]
		if calls < len(statuses) {
			st = statuses[calls]
		}
		calls++
		app := &api.App{Slug: slug, Status: st}
		if st == "failed" {
			app.StatusReason = reason
		}
		return app, nil
	}, &calls
}

// waitDeps wires a mock client and a fake clock that advances on Sleep.
func waitDeps(t *testing.T, mock *mockAPIClient) {
	t.Helper()
	now := time.Unix(0, 0)
	deps = &Deps{
		GetToken:     func() (string, error) { return "tok123", nil },
		GetCwd:       func() (string, error) { return ".", nil },
		Confirm:      func(string) bool { return true },
		NewAPIClient: newMockAPIClient(mock),
		Sleep:        func(d time.Duration) { now = now.Add(d) },
		Now:          func() time.Time { return now },
	}
	t.Cleanup(func() { resetDeployState(); noWait = false; waitTimeout = 0 })
}

// A1: success is only reported once the egg is running.
func TestRunDeploy_WaitsUntilRunning(t *testing.T) {
	tmp := writeParentHatchToml(t, "mysite-x1y2")
	getApp, calls := statusSequence("", "building", "building", "running")
	waitDeps(t, &mockAPIClient{getAppFn: getApp})
	deployTarget, runtime = tmp, "static"

	var err error
	out := captureOutput(func() { err = runDeploy(nil, nil) })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *calls != 3 {
		t.Fatalf("GetApp called %d times, want 3", *calls)
	}
	if n := strings.Count(out, "Deployed successfully!"); n != 1 {
		t.Fatalf("\"Deployed successfully!\" printed %d times, want 1:\n%s", n, out)
	}
}

// A2: a failed rollout is an error carrying status_reason, never a success.
func TestRunDeploy_FailedStatusReturnsError(t *testing.T) {
	tmp := writeParentHatchToml(t, "mysite-x1y2")
	getApp, _ := statusSequence("Deploy failed: app did not start in time", "building", "failed")
	waitDeps(t, &mockAPIClient{getAppFn: getApp})
	deployTarget, runtime = tmp, "static"

	var err error
	out := captureOutput(func() { err = runDeploy(nil, nil) })
	if err == nil || !strings.Contains(err.Error(), "app did not start in time") {
		t.Fatalf("want error with status_reason, got %v", err)
	}
	if !strings.Contains(err.Error(), "hatch logs mysite-x1y2") {
		t.Fatalf("error should suggest hatch logs, got %v", err)
	}
	if strings.Contains(out, "Deployed successfully!") {
		t.Fatalf("failed deploy must not print success:\n%s", out)
	}
}

// A3: no terminal status within --timeout is an error naming the last status.
func TestRunDeploy_TimeoutReturnsError(t *testing.T) {
	tmp := writeParentHatchToml(t, "mysite-x1y2")
	getApp, _ := statusSequence("", "building")
	waitDeps(t, &mockAPIClient{getAppFn: getApp})
	deployTarget, runtime = tmp, "static"
	waitTimeout = 10 * time.Second

	var err error
	out := captureOutput(func() { err = runDeploy(nil, nil) })
	if err == nil || !strings.Contains(err.Error(), "timed out") || !strings.Contains(err.Error(), "last status: building") {
		t.Fatalf("want timeout error naming last status, got %v", err)
	}
	if !strings.Contains(err.Error(), "hatch info mysite-x1y2") {
		t.Fatalf("timeout error should suggest hatch info, got %v", err)
	}
	if strings.Contains(out, "Deployed successfully!") {
		t.Fatalf("timed-out deploy must not print success:\n%s", out)
	}
}

// A4: --no-wait never polls and never claims success.
func TestRunDeploy_NoWaitSkipsPolling(t *testing.T) {
	tmp := writeParentHatchToml(t, "mysite-x1y2")
	waitDeps(t, &mockAPIClient{getAppFn: func(string) (*api.App, error) {
		t.Error("GetApp must not be called with --no-wait")
		return nil, errors.New("unexpected")
	}})
	deployTarget, runtime = tmp, "static"
	noWait = true

	var err error
	out := captureOutput(func() { err = runDeploy(nil, nil) })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(out, "Deployed successfully!") || !strings.Contains(out, "Deploy started") {
		t.Fatalf("--no-wait output should say \"Deploy started\" only:\n%s", out)
	}
}

// parseSingleJSONLine asserts stdout is exactly one JSON object line.
func parseSingleJSONLine(t *testing.T, out string) map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("--json stdout must be exactly one line, got %d:\n%s", len(lines), out)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &payload); err != nil {
		t.Fatalf("--json stdout is not valid JSON: %v\n%s", err, out)
	}
	return payload
}

// A5: --json on the normal path prints one JSON line with the final status.
func TestRunDeploy_JSONOutput_Running(t *testing.T) {
	tmp := writeParentHatchToml(t, "mysite-x1y2")
	getApp, _ := statusSequence("", "building", "running")
	waitDeps(t, &mockAPIClient{getAppFn: getApp})
	deployTarget, runtime, jsonOut = tmp, "static", true

	var err error
	out := captureOutput(func() { err = runDeploy(nil, nil) })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	p := parseSingleJSONLine(t, out)
	if p["slug"] != "mysite-x1y2" || p["status"] != "running" || p["waited"] != true ||
		p["url"] != "https://mysite-x1y2.nest.gethatch.eu" {
		t.Fatalf("unexpected JSON payload: %v", p)
	}
}

// A5: on failure, --json still prints the final status, then exits non-zero.
func TestRunDeploy_JSONOutput_Failed(t *testing.T) {
	tmp := writeParentHatchToml(t, "mysite-x1y2")
	getApp, _ := statusSequence("Deploy failed: app did not start in time", "failed")
	waitDeps(t, &mockAPIClient{getAppFn: getApp})
	deployTarget, runtime, jsonOut = tmp, "static", true

	var err error
	out := captureOutput(func() { err = runDeploy(nil, nil) })
	if err == nil {
		t.Fatal("want non-nil error on failed deploy")
	}
	p := parseSingleJSONLine(t, out)
	if p["status"] != "failed" || p["status_reason"] != "Deploy failed: app did not start in time" || p["error"] == nil {
		t.Fatalf("unexpected JSON payload: %v", p)
	}
}

// A6: previews wait too, and --json keeps the existing keys plus status.
func TestRunDeploy_Preview_WaitsAndReportsStatus(t *testing.T) {
	tmp := writeParentHatchToml(t, "mysite-x1y2")
	var polled []string
	getApp, _ := statusSequence("", "building", "running")
	waitDeps(t, &mockAPIClient{getAppFn: func(slug string) (*api.App, error) {
		polled = append(polled, slug)
		return getApp(slug)
	}})
	deployTarget, runtime, previewRef, jsonOut = tmp, "static", "pr-42", true

	var err error
	out := captureOutput(func() { err = runDeploy(nil, nil) })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(polled) != 2 || polled[0] != "mysite-x1y2-pr-42" {
		t.Fatalf("preview should poll its own slug until running, polled %v", polled)
	}
	p := parseSingleJSONLine(t, out)
	if p["slug"] != "mysite-x1y2-pr-42" || p["pr_number"] != float64(42) || p["status"] != "running" ||
		!strings.Contains(p["url"].(string), "mysite-x1y2-pr-42") {
		t.Fatalf("unexpected preview JSON payload: %v", p)
	}
}

// A6: a failed preview rollout is an error, without "Preview deployed!".
func TestRunDeploy_Preview_FailedReturnsError(t *testing.T) {
	tmp := writeParentHatchToml(t, "mysite-x1y2")
	getApp, _ := statusSequence("boom", "failed")
	waitDeps(t, &mockAPIClient{getAppFn: getApp})
	deployTarget, runtime, previewRef = tmp, "static", "pr-42"

	var err error
	out := captureOutput(func() { err = runDeploy(nil, nil) })
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("want error with reason, got %v", err)
	}
	if strings.Contains(out, "Preview deployed!") {
		t.Fatalf("failed preview must not print success:\n%s", out)
	}
}

func TestNewCmd_WaitFlagsRegistered(t *testing.T) {
	cmd := NewCmd()
	if cmd.Flags().Lookup("no-wait") == nil {
		t.Fatal("--no-wait flag not registered")
	}
	f := cmd.Flags().Lookup("timeout")
	if f == nil || f.DefValue != "6m0s" {
		t.Fatalf("--timeout flag missing or wrong default: %+v", f)
	}
}
