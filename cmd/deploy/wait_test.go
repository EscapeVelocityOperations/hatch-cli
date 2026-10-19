package deploy

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/exitcode"
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

// --- h-59ej rework: exit codes survive non-TTY, JSON on every outcome ------

// assertExitCode checks the exit status main will use for err.
func assertExitCode(t *testing.T, err error, want int) {
	t.Helper()
	code, ok := exitcode.From(err)
	if !ok || code != want {
		t.Fatalf("exit code = %d (carried: %v), want %d; err = %v", code, ok, want, err)
	}
}

// A12: a failed rollout exits 2, a timeout 3, on normal and preview paths.
func TestRunDeploy_ExitCodes(t *testing.T) {
	tests := []struct {
		name     string
		statuses []string
		preview  string
		timeout  time.Duration
		want     int
	}{
		{"failed", []string{"building", "failed"}, "", 0, ExitRolloutFailed},
		{"crashed", []string{"crashed"}, "", 0, ExitRolloutFailed},
		{"timeout", []string{"building"}, "", 10 * time.Second, ExitWaitTimeout},
		{"preview failed", []string{"failed"}, "pr-42", 0, ExitRolloutFailed},
		{"preview timeout", []string{"building"}, "pr-42", 10 * time.Second, ExitWaitTimeout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := writeParentHatchToml(t, "mysite-x1y2")
			getApp, _ := statusSequence("boom", tt.statuses...)
			waitDeps(t, &mockAPIClient{getAppFn: getApp})
			deployTarget, runtime, previewRef, waitTimeout = tmp, "static", tt.preview, tt.timeout

			var err error
			captureOutput(func() { err = runDeploy(nil, nil) })
			assertExitCode(t, err, tt.want)
		})
	}
}

// A12: an upload (image build) failure exits 1 and --json still reports it.
func TestRunDeploy_UploadFailure_JSONAndExit1(t *testing.T) {
	tmp := writeParentHatchToml(t, "mysite-x1y2")
	waitDeps(t, &mockAPIClient{
		uploadArtifactFn: func(string, []byte, string, string) error { return errors.New("API error 500: build failed") },
	})
	deployTarget, runtime, jsonOut = tmp, "static", true

	var err error
	out := captureOutput(func() { err = runDeploy(nil, nil) })
	assertExitCode(t, err, ExitDeployError)
	p := parseSingleJSONLine(t, out)
	if p["status"] != "error" || p["slug"] != "mysite-x1y2" || p["waited"] != false ||
		!strings.Contains(p["error"].(string), "build failed") {
		t.Fatalf("unexpected JSON payload: %v", p)
	}
}

// A13: failures before any egg is known still print one JSON line.
func TestRunDeploy_PreUploadErrors_JSON(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		runtime string
		want    string
	}{
		{"missing deploy-target", "", "static", "--deploy-target is required"},
		{"missing runtime", "dir", "", "--runtime is required"},
		{"unknown runtime", "dir", "cobol", "unknown runtime"},
		{"target not found", "/nonexistent-dir", "static", "deploy-target directory not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := writeParentHatchToml(t, "mysite-x1y2")
			waitDeps(t, &mockAPIClient{})
			deployTarget, runtime, jsonOut = tt.target, tt.runtime, true
			if tt.target == "dir" {
				deployTarget = tmp
			}

			var err error
			out := captureOutput(func() { err = runDeploy(nil, nil) })
			assertExitCode(t, err, ExitDeployError)
			p := parseSingleJSONLine(t, out)
			if p["status"] != "error" || !strings.Contains(p["error"].(string), tt.want) {
				t.Fatalf("unexpected JSON payload: %v", p)
			}
			if _, ok := p["slug"]; ok {
				t.Fatalf("no egg is known yet, slug must be absent: %v", p)
			}
		})
	}
}

// Without --json, a pre-upload error prints nothing on stdout.
func TestRunDeploy_PreUploadError_NoJSONWithoutFlag(t *testing.T) {
	waitDeps(t, &mockAPIClient{})
	deployTarget, runtime = "/nonexistent-dir", "static"

	var err error
	out := captureOutput(func() { err = runDeploy(nil, nil) })
	assertExitCode(t, err, ExitDeployError)
	if strings.Contains(out, "{") {
		t.Fatalf("stdout must not carry JSON without --json:\n%s", out)
	}
}

func TestRunDeploy_SuccessCarriesNoExitError(t *testing.T) {
	tmp := writeParentHatchToml(t, "mysite-x1y2")
	waitDeps(t, &mockAPIClient{})
	deployTarget, runtime = tmp, "static"

	var err error
	captureOutput(func() { err = runDeploy(nil, nil) })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
