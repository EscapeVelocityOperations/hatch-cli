package apps

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
)

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

func TestRunList_NotLoggedIn(t *testing.T) {
	deps = &Deps{
		GetToken: func() (string, error) { return "", nil },
	}
	defer func() { deps = defaultDeps() }()

	err := runList(nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "not logged in. Run 'hatch login', set HATCH_TOKEN, or use --token" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunList_TokenError(t *testing.T) {
	deps = &Deps{
		GetToken: func() (string, error) { return "", fmt.Errorf("disk error") },
	}
	defer func() { deps = defaultDeps() }()

	err := runList(nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "checking auth: disk error" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunList_Empty(t *testing.T) {
	deps = &Deps{
		GetToken: func() (string, error) { return "tok123", nil },
		ListApps: func(token string) ([]api.App, error) { return nil, nil },
	}
	defer func() { deps = defaultDeps() }()

	output := captureOutput(func() {
		err := runList(nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	if !contains(output, "No eggs found") {
		t.Fatalf("expected 'No eggs found' message, got: %s", output)
	}
}

func TestRunList_ShowsApps(t *testing.T) {
	deps = &Deps{
		GetToken: func() (string, error) { return "tok123", nil },
		ListApps: func(token string) ([]api.App, error) {
			return []api.App{
				{Slug: "myapp", Name: "My App", Status: "running", URL: "https://myapp.gethatch.eu"},
				{Slug: "other", Name: "Other App", Status: "stopped", URL: "https://other.gethatch.eu"},
			}, nil
		},
	}
	defer func() { deps = defaultDeps() }()

	output := captureOutput(func() {
		err := runList(nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	if !contains(output, "myapp") {
		t.Fatalf("expected 'myapp' slug in output, got: %s", output)
	}
	if !contains(output, "My App") {
		t.Fatalf("expected 'My App' name in output, got: %s", output)
	}
	if !contains(output, "other") {
		t.Fatalf("expected 'other' slug in output, got: %s", output)
	}
}

func TestRunList_GeneratesURLFromSlug(t *testing.T) {
	deps = &Deps{
		GetToken: func() (string, error) { return "tok123", nil },
		ListApps: func(token string) ([]api.App, error) {
			return []api.App{
				{Slug: "nourl-app", Name: "No URL App", Status: "running", URL: ""},
			}, nil
		},
	}
	defer func() { deps = defaultDeps() }()

	output := captureOutput(func() {
		err := runList(nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	if !contains(output, "https://nourl-app.nest.gethatch.eu") {
		t.Fatalf("expected generated URL in output, got: %s", output)
	}
}

func TestRunList_APIError(t *testing.T) {
	deps = &Deps{
		GetToken: func() (string, error) { return "tok123", nil },
		ListApps: func(token string) ([]api.App, error) {
			return nil, fmt.Errorf("connection refused")
		},
	}
	defer func() { deps = defaultDeps() }()

	output := captureOutput(func() {
		err := runList(nil, nil)
		if err == nil {
			t.Fatal("expected error")
		}
		if err.Error() != "fetching eggs: connection refused" {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	_ = output
}

func TestRunInfo_NotLoggedIn(t *testing.T) {
	deps = &Deps{
		GetToken: func() (string, error) { return "", nil },
	}
	defer func() { deps = defaultDeps() }()

	err := runInfo(nil, []string{"myapp"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRunInfo_ShowsDetails(t *testing.T) {
	deps = &Deps{
		GetToken: func() (string, error) { return "tok123", nil },
		GetApp: func(token, slug string) (*api.App, error) {
			if slug != "myapp" {
				t.Fatalf("unexpected slug: %s", slug)
			}
			return &api.App{
				Slug:      "myapp",
				Name:      "My App",
				Status:    "running",
				URL:       "https://myapp.gethatch.eu",
				Region:    "eu-west",
				CreatedAt: time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC),
			}, nil
		},
	}
	defer func() { deps = defaultDeps() }()

	output := captureOutput(func() {
		err := runInfo(nil, []string{"myapp"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !contains(output, "My App") {
		t.Fatalf("expected 'My App' in output, got: %s", output)
	}
	if !contains(output, "eu-west") {
		t.Fatalf("expected 'eu-west' in output, got: %s", output)
	}
}

func TestRunInfo_APIError(t *testing.T) {
	deps = &Deps{
		GetToken: func() (string, error) { return "tok123", nil },
		GetApp: func(token, slug string) (*api.App, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	defer func() { deps = defaultDeps() }()

	captureOutput(func() {
		err := runInfo(nil, []string{"badapp"})
		if err == nil {
			t.Fatal("expected error")
		}
		if err.Error() != "fetching egg: not found" {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestStatusColor(t *testing.T) {
	// Just verify it doesn't panic for various statuses
	statusColor("running")
	statusColor("stopped")
	statusColor("crashed")
	statusColor("deploying")
	statusColor("unknown")
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
