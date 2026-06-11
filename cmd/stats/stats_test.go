package stats

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// h-e5w0d TDD-red: `hatch stats <app>` renders the v1 metrics table.
// House style is the mock-client deps pattern (not a golden file) — same
// reviewer flag as h-p7lvr; the rendered-content assertions below pin the
// table fields, which is what the spec's "golden test" is after.

func captureOutput(fn func() error) (string, error) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := fn()

	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String(), err
}

func fixtureMetrics() AppMetrics {
	return AppMetrics{
		Status:        "running",
		UptimeSeconds: 5400, // 1h30m
		CPUPercent:    12.5,
		MemoryMB:      128,
		MemoryLimitMB: 512,
		LastDeployAt:  time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC),
		WakesToday:    7,
		SampledAt:     time.Date(2026, 6, 11, 22, 0, 0, 0, time.UTC),
	}
}

// TestRunStats_RendersTable (T-005): the one-shot table shows every v1
// field from the fixture payload.
func TestRunStats_RendersTable(t *testing.T) {
	var gotSlug string
	deps = &Deps{
		GetToken: func() (string, error) { return "tok", nil },
		GetMetrics: func(token, slug string) (AppMetrics, error) {
			gotSlug = slug
			return fixtureMetrics(), nil
		},
	}
	defer func() { deps = defaultDeps() }()

	out, err := captureOutput(func() error { return runStats("my-app", false) })
	if err != nil {
		t.Fatalf("runStats: %v", err)
	}
	if gotSlug != "my-app" {
		t.Fatalf("GetMetrics called with %q, want my-app", gotSlug)
	}

	for _, want := range []string{
		"running", // status
		"12.5",    // cpu_percent
		"128",     // memory_mb
		"512",     // memory_limit_mb
		"7",       // wakes_today
	} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing %q, got:\n%s", want, out)
		}
	}
	lower := strings.ToLower(out)
	for _, label := range []string{"status", "cpu", "memory", "uptime", "wakes"} {
		if !strings.Contains(lower, label) {
			t.Errorf("table missing label %q, got:\n%s", label, out)
		}
	}
}

// TestRunStats_NotLoggedIn: standard auth gate.
func TestRunStats_NotLoggedIn(t *testing.T) {
	deps = &Deps{
		GetToken: func() (string, error) { return "", errors.New("no token") },
	}
	defer func() { deps = defaultDeps() }()

	_, err := captureOutput(func() error { return runStats("my-app", false) })
	if err == nil || !strings.Contains(err.Error(), "not logged in") {
		t.Fatalf("err = %v, want 'not logged in'", err)
	}
}

// TestNewCmd_Structure: command shape — stats with metrics alias and the
// --watch flag.
func TestNewCmd_Structure(t *testing.T) {
	cmd := NewCmd()
	if !strings.HasPrefix(cmd.Use, "stats") {
		t.Fatalf("Use = %q, want stats <slug>", cmd.Use)
	}
	hasAlias := false
	for _, a := range cmd.Aliases {
		if a == "metrics" {
			hasAlias = true
		}
	}
	if !hasAlias {
		t.Errorf("metrics alias missing")
	}
	if cmd.Flags().Lookup("watch") == nil {
		t.Errorf("--watch flag missing")
	}
}
