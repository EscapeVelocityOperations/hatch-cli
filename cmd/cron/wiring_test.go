package cron

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
)

// h-t3ju6 GAP-2b: the production deps must wire the REAL *api.Client and REAL
// slug resolution — the h-wb661 stub (stubClient + "no app detected") must be
// gone. These tests fail while the stub is in place.

// TestDefaultDepsUsesRealAPIClient proves NewAPIClient returns the real client,
// not a stub that answers "cron API client not wired yet".
func TestDefaultDepsUsesRealAPIClient(t *testing.T) {
	c := defaultDeps().NewAPIClient("tok123")
	if _, ok := c.(*api.Client); !ok {
		t.Fatalf("defaultDeps().NewAPIClient returned %T, want *api.Client (stub not killed)", c)
	}
}

// TestDefaultDepsResolveSlugFromToml proves slug resolution reads .hatch.toml
// (replacing the stub that always returned "no app detected").
func TestDefaultDepsResolveSlugFromToml(t *testing.T) {
	t.Run("reads .hatch.toml", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, ".hatch.toml"), []byte("slug = \"demo-app\"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Chdir(dir)

		slug, err := defaultDeps().ResolveSlug()
		if err != nil {
			t.Fatalf("ResolveSlug: %v", err)
		}
		if slug != "demo-app" {
			t.Errorf("slug = %q, want demo-app", slug)
		}
	})

	t.Run("errors when no .hatch.toml", func(t *testing.T) {
		t.Chdir(t.TempDir())
		if _, err := defaultDeps().ResolveSlug(); err == nil {
			t.Fatal("expected error when no .hatch.toml is present")
		}
	})
}

// TestCronAddReachesAPI is the end-to-end wiring proof (AC 3b): `hatch cron add`
// routed through the real *api.Client issues a POST the fake hatch-api receives,
// rather than failing with "cron API client not wired yet".
func TestCronAddReachesAPI(t *testing.T) {
	var gotMethod, gotPath string
	var body struct {
		Schedule string `json:"schedule"`
		Command  string `json:"command"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(api.CronJob{ID: "cron-1", Schedule: body.Schedule, Command: body.Command, Enabled: true})
	}))
	defer srv.Close()

	deps = &Deps{
		GetToken:     func() (string, error) { return "tok123", nil },
		ResolveSlug:  func() (string, error) { return "demo-app", nil },
		NewAPIClient: func(token string) APIClient { return api.NewTestClient(token, srv.URL) },
	}
	defer func() { deps = defaultDeps() }()

	var err error
	captureOutput(func() { err = execCron("add", "*/5 * * * *", "--", "echo", "hi") })
	if err != nil {
		t.Fatalf("cron add against fake API: %v", err)
	}
	if gotMethod != "POST" || gotPath != "/v1/apps/demo-app/crons" {
		t.Fatalf("server got %s %s, want POST /v1/apps/demo-app/crons", gotMethod, gotPath)
	}
	if body.Schedule != "*/5 * * * *" || body.Command != "echo hi" {
		t.Errorf("server body = %+v, want schedule=%q command=%q", body, "*/5 * * * *", "echo hi")
	}
}
