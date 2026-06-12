package cron

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
)

// mockAPIClient implements the APIClient interface for testing.
type mockAPIClient struct {
	createCronFn func(slug, schedule, command string) (*api.CronJob, error)
	listCronsFn  func(slug string) ([]api.CronJob, error)
	deleteCronFn func(slug, cronID string) error
	listRunsFn   func(slug, cronID string) ([]api.CronRun, error)
	getRunLogsFn func(slug, cronID, runID string) (string, error)
}

func (m *mockAPIClient) CreateCron(slug, schedule, command string) (*api.CronJob, error) {
	if m.createCronFn != nil {
		return m.createCronFn(slug, schedule, command)
	}
	return &api.CronJob{ID: "cron-1", Schedule: schedule, Command: command, Enabled: true}, nil
}

func (m *mockAPIClient) ListCrons(slug string) ([]api.CronJob, error) {
	if m.listCronsFn != nil {
		return m.listCronsFn(slug)
	}
	return nil, nil
}

func (m *mockAPIClient) DeleteCron(slug, cronID string) error {
	if m.deleteCronFn != nil {
		return m.deleteCronFn(slug, cronID)
	}
	return nil
}

func (m *mockAPIClient) ListCronRuns(slug, cronID string) ([]api.CronRun, error) {
	if m.listRunsFn != nil {
		return m.listRunsFn(slug, cronID)
	}
	return nil, nil
}

func (m *mockAPIClient) GetCronRunLogs(slug, cronID, runID string) (string, error) {
	if m.getRunLogsFn != nil {
		return m.getRunLogsFn(slug, cronID, runID)
	}
	return "", nil
}

func newMockAPIClient(mock *mockAPIClient) func(token string) APIClient {
	return func(token string) APIClient {
		return mock
	}
}

// setTestDeps points the package deps at a logged-in user on app demo-app
// backed by the given mock. Returns a restore func for defer.
func setTestDeps(mock *mockAPIClient) func() {
	deps = &Deps{
		GetToken:     func() (string, error) { return "tok123", nil },
		ResolveSlug:  func() (string, error) { return "demo-app", nil },
		NewAPIClient: newMockAPIClient(mock),
	}
	return func() { deps = defaultDeps() }
}

func captureOutput(fn func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

// execCron runs `hatch cron <args...>` against a fresh command tree.
func execCron(args ...string) error {
	cmd := NewCmd()
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

// T-013 (h-p7lvr): `hatch cron add <schedule> -- <command>` — schedule and
// command reach CreateCron, multi-word commands are joined, the `--`
// separator is mandatory, and auth is required (spec h-k6yeh CLI surface).
func TestCronAdd(t *testing.T) {
	t.Run("parses schedule and command", func(t *testing.T) {
		var gotSlug, gotSchedule, gotCommand string
		mock := &mockAPIClient{
			createCronFn: func(slug, schedule, command string) (*api.CronJob, error) {
				gotSlug, gotSchedule, gotCommand = slug, schedule, command
				return &api.CronJob{ID: "cron-1", Schedule: schedule, Command: command, Enabled: true}, nil
			},
		}
		defer setTestDeps(mock)()

		captureOutput(func() {
			if err := execCron("add", "*/5 * * * *", "--", "echo", "hi"); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})

		if gotSlug != "demo-app" {
			t.Errorf("CreateCron slug = %q, want demo-app", gotSlug)
		}
		if gotSchedule != "*/5 * * * *" {
			t.Errorf("CreateCron schedule = %q, want */5 * * * *", gotSchedule)
		}
		if gotCommand != "echo hi" {
			t.Errorf("CreateCron command = %q, want %q", gotCommand, "echo hi")
		}
	})

	t.Run("multi-word command joined", func(t *testing.T) {
		var gotCommand string
		mock := &mockAPIClient{
			createCronFn: func(slug, schedule, command string) (*api.CronJob, error) {
				gotCommand = command
				return &api.CronJob{ID: "cron-2", Schedule: schedule, Command: command, Enabled: true}, nil
			},
		}
		defer setTestDeps(mock)()

		captureOutput(func() {
			if err := execCron("add", "0 3 * * *", "--", "npm", "run", "cleanup", "--prod"); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})

		if gotCommand != "npm run cleanup --prod" {
			t.Errorf("CreateCron command = %q, want %q", gotCommand, "npm run cleanup --prod")
		}
	})

	// G-004 (h-ya0fo, review h-l9gzs MEDIUM): a command argument that contains
	// spaces (or shell metacharacters) must keep its boundary. Joining argv with
	// a bare space collapses `sh -c 'echo a b'` into `sh -c echo a b`, which the
	// server then runs as `/bin/sh -c "sh -c echo a b"` — the inner command loses
	// its argument. Each argv element is shell-quoted so the stored command
	// round-trips through `/bin/sh -c`.
	t.Run("preserves quoted argument boundaries", func(t *testing.T) {
		var gotCommand string
		mock := &mockAPIClient{
			createCronFn: func(slug, schedule, command string) (*api.CronJob, error) {
				gotCommand = command
				return &api.CronJob{ID: "cron-3", Schedule: schedule, Command: command, Enabled: true}, nil
			},
		}
		defer setTestDeps(mock)()

		captureOutput(func() {
			if err := execCron("add", "*/5 * * * *", "--", "sh", "-c", "echo a b"); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})

		want := "sh -c 'echo a b'"
		if gotCommand != want {
			t.Errorf("CreateCron command = %q, want %q", gotCommand, want)
		}
	})

	t.Run("quotes shell metacharacters", func(t *testing.T) {
		var gotCommand string
		mock := &mockAPIClient{
			createCronFn: func(slug, schedule, command string) (*api.CronJob, error) {
				gotCommand = command
				return &api.CronJob{ID: "cron-4", Schedule: schedule, Command: command, Enabled: true}, nil
			},
		}
		defer setTestDeps(mock)()

		captureOutput(func() {
			if err := execCron("add", "0 3 * * *", "--", "sh", "-c", "rm -rf tmp; echo done"); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})

		want := "sh -c 'rm -rf tmp; echo done'"
		if gotCommand != want {
			t.Errorf("CreateCron command = %q, want %q", gotCommand, want)
		}
	})

	t.Run("missing -- is a usage error", func(t *testing.T) {
		createCalled := false
		mock := &mockAPIClient{
			createCronFn: func(slug, schedule, command string) (*api.CronJob, error) {
				createCalled = true
				return nil, nil
			},
		}
		defer setTestDeps(mock)()

		var err error
		captureOutput(func() { err = execCron("add", "*/5 * * * *", "echo", "hi") })
		if err == nil {
			t.Fatal("expected usage error when -- separator is missing")
		}
		if createCalled {
			t.Error("CreateCron must not be called on usage error")
		}
	})

	t.Run("not logged in", func(t *testing.T) {
		deps = &Deps{
			GetToken:     func() (string, error) { return "", nil },
			ResolveSlug:  func() (string, error) { return "demo-app", nil },
			NewAPIClient: newMockAPIClient(&mockAPIClient{}),
		}
		defer func() { deps = defaultDeps() }()

		var err error
		captureOutput(func() { err = execCron("add", "*/5 * * * *", "--", "echo", "hi") })
		if err == nil {
			t.Fatal("expected error for unauthenticated user")
		}
		if !strings.Contains(err.Error(), "not logged in") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

// T-014 (h-p7lvr): `hatch cron list` renders schedule, command, enabled
// state, last-run status+time and next run; `hatch cron rm` deletes by id
// and surfaces unknown ids as errors (spec h-k6yeh CLI surface).
func TestCronListAndRm(t *testing.T) {
	t.Run("list renders columns", func(t *testing.T) {
		lastRun := time.Date(2026, 6, 10, 3, 0, 0, 0, time.UTC)
		nextRun := time.Date(2026, 6, 12, 3, 0, 0, 0, time.UTC)
		mock := &mockAPIClient{
			listCronsFn: func(slug string) ([]api.CronJob, error) {
				return []api.CronJob{
					{
						ID:            "cron-1",
						Schedule:      "*/5 * * * *",
						Command:       "echo hi",
						Enabled:       true,
						LastRunStatus: "success",
						LastRunAt:     &lastRun,
						NextRunAt:     &nextRun,
					},
					{
						ID:       "cron-2",
						Schedule: "0 4 * * *",
						Command:  "npm run cleanup",
						Enabled:  false,
					},
				}, nil
			},
		}
		defer setTestDeps(mock)()

		var err error
		out := captureOutput(func() { err = execCron("list") })
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for _, want := range []string{"*/5 * * * *", "echo hi", "success", "2026-06-10", "2026-06-12"} {
			if !strings.Contains(out, want) {
				t.Errorf("list output missing %q (got: %q)", want, out)
			}
		}
		if !strings.Contains(strings.ToLower(out), "disabled") {
			t.Errorf("list output does not render enabled state (got: %q)", out)
		}
	})

	t.Run("rm deletes by id", func(t *testing.T) {
		var gotSlug, gotID string
		mock := &mockAPIClient{
			deleteCronFn: func(slug, cronID string) error {
				gotSlug, gotID = slug, cronID
				return nil
			},
		}
		defer setTestDeps(mock)()

		captureOutput(func() {
			if err := execCron("rm", "cron-1"); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})

		if gotSlug != "demo-app" || gotID != "cron-1" {
			t.Errorf("DeleteCron called with (%q, %q), want (demo-app, cron-1)", gotSlug, gotID)
		}
	})

	t.Run("rm unknown id errors", func(t *testing.T) {
		mock := &mockAPIClient{
			deleteCronFn: func(slug, cronID string) error {
				return fmt.Errorf("cron %s not found", cronID)
			},
		}
		defer setTestDeps(mock)()

		var err error
		captureOutput(func() { err = execCron("rm", "nope") })
		if err == nil {
			t.Fatal("expected error for unknown cron id")
		}
	})
}

// T-015 (h-p7lvr): `hatch cron logs <cron-id>` shows the latest run's logs
// by default; `--run <id>` selects a specific run (spec h-k6yeh CLI surface).
func TestCronLogs(t *testing.T) {
	newest := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	runsMock := func(slug, cronID string) ([]api.CronRun, error) {
		return []api.CronRun{
			{ID: "run-2", Status: "success", StartedAt: newest},
			{ID: "run-1", Status: "failed", StartedAt: newest.Add(-time.Hour)},
		}, nil
	}

	t.Run("default shows latest run", func(t *testing.T) {
		var gotCronID, gotRunID string
		mock := &mockAPIClient{
			listRunsFn: runsMock,
			getRunLogsFn: func(slug, cronID, runID string) (string, error) {
				gotCronID, gotRunID = cronID, runID
				return "hello from run", nil
			},
		}
		defer setTestDeps(mock)()

		var err error
		out := captureOutput(func() { err = execCron("logs", "cron-1") })
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if gotCronID != "cron-1" {
			t.Errorf("GetCronRunLogs cron = %q, want cron-1", gotCronID)
		}
		if gotRunID != "run-2" {
			t.Errorf("GetCronRunLogs run = %q, want run-2 (latest)", gotRunID)
		}
		if !strings.Contains(out, "hello from run") {
			t.Errorf("logs output missing run logs (got: %q)", out)
		}
	})

	t.Run("--run selects a specific run", func(t *testing.T) {
		var gotRunID string
		mock := &mockAPIClient{
			listRunsFn: runsMock,
			getRunLogsFn: func(slug, cronID, runID string) (string, error) {
				gotRunID = runID
				return "older run logs", nil
			},
		}
		defer setTestDeps(mock)()

		captureOutput(func() {
			if err := execCron("logs", "cron-1", "--run", "run-1"); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})

		if gotRunID != "run-1" {
			t.Errorf("GetCronRunLogs run = %q, want run-1", gotRunID)
		}
	})
}
