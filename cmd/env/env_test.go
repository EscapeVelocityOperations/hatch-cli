package env

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"testing"

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

func TestRunList_ShowsVars(t *testing.T) {
	appSlug = "myapp"
	defer func() { appSlug = "" }()

	deps = &Deps{
		GetToken: func() (string, error) { return "tok123", nil },
		GetEnvVars: func(token, slug string) ([]api.EnvVar, error) {
			return []api.EnvVar{
				{Key: "PORT", Value: "8080"},
				{Key: "NODE_ENV", Value: "production"},
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

	if !contains(output, "PORT") {
		t.Fatalf("expected 'PORT' in output, got: %s", output)
	}
	if !contains(output, "8080") {
		t.Fatalf("expected '8080' in output, got: %s", output)
	}
}

func TestRunList_Empty(t *testing.T) {
	appSlug = "myapp"
	defer func() { appSlug = "" }()

	deps = &Deps{
		GetToken:   func() (string, error) { return "tok123", nil },
		GetEnvVars: func(token, slug string) ([]api.EnvVar, error) { return nil, nil },
	}
	defer func() { deps = defaultDeps() }()

	output := captureOutput(func() {
		err := runList(nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !contains(output, "No environment variables") {
		t.Fatalf("expected empty message, got: %s", output)
	}
}

func TestRunList_NoSlug(t *testing.T) {
	appSlug = ""
	deps = &Deps{
		GetToken: func() (string, error) { return "tok123", nil },
	}
	defer func() { deps = defaultDeps() }()

	err := runList(nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !contains(err.Error(), "no egg specified") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunSet_Success(t *testing.T) {
	appSlug = "myapp"
	defer func() { appSlug = "" }()

	setKeys := []string{}
	deps = &Deps{
		GetToken: func() (string, error) { return "tok123", nil },
		SetEnvVar: func(token, slug, key, value string) error {
			setKeys = append(setKeys, key+"="+value)
			return nil
		},
	}
	defer func() { deps = defaultDeps() }()

	output := captureOutput(func() {
		err := runSet(nil, []string{"PORT=8080", "DB=postgres://localhost"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if len(setKeys) != 2 {
		t.Fatalf("expected 2 set calls, got %d", len(setKeys))
	}
	if !contains(output, "Set PORT") {
		t.Fatalf("expected success message, got: %s", output)
	}
}

func TestRunSet_InvalidFormat(t *testing.T) {
	appSlug = "myapp"
	defer func() { appSlug = "" }()

	deps = &Deps{
		GetToken: func() (string, error) { return "tok123", nil },
	}
	defer func() { deps = defaultDeps() }()

	err := runSet(nil, []string{"INVALID"})
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
	if !contains(err.Error(), "invalid format") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunSet_APIError(t *testing.T) {
	appSlug = "myapp"
	defer func() { appSlug = "" }()

	deps = &Deps{
		GetToken: func() (string, error) { return "tok123", nil },
		SetEnvVar: func(token, slug, key, value string) error {
			return fmt.Errorf("permission denied")
		},
	}
	defer func() { deps = defaultDeps() }()

	err := runSet(nil, []string{"PORT=8080"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !contains(err.Error(), "setting PORT") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunUnset_Success(t *testing.T) {
	appSlug = "myapp"
	defer func() { appSlug = "" }()

	unsetKeys := []string{}
	deps = &Deps{
		GetToken: func() (string, error) { return "tok123", nil },
		UnsetEnvVar: func(token, slug, key string) error {
			unsetKeys = append(unsetKeys, key)
			return nil
		},
	}
	defer func() { deps = defaultDeps() }()

	output := captureOutput(func() {
		err := runUnset(nil, []string{"PORT", "DB"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if len(unsetKeys) != 2 {
		t.Fatalf("expected 2 unset calls, got %d", len(unsetKeys))
	}
	if !contains(output, "Unset PORT") {
		t.Fatalf("expected success message, got: %s", output)
	}
}

func TestRunList_MasksSensitiveValues(t *testing.T) {
	appSlug = "myapp"
	showSecrets = false
	defer func() { appSlug = ""; showSecrets = false }()

	deps = &Deps{
		GetToken: func() (string, error) { return "tok123", nil },
		GetEnvVars: func(token, slug string) ([]api.EnvVar, error) {
			return []api.EnvVar{
				{Key: "DATABASE_URL", Value: "postgres://user:pass@host:5432/db"},
				{Key: "API_KEY", Value: "sk-1234567890abcdef"},
				{Key: "SHORT_SECRET", Value: "abc"},
				{Key: "NODE_ENV", Value: "production"},
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

	// DATABASE_URL should be masked
	if contains(output, "postgres://user:pass@host:5432/db") {
		t.Fatalf("expected DATABASE_URL to be masked, got: %s", output)
	}
	if !contains(output, "post****2/db") {
		t.Fatalf("expected masked DATABASE_URL in output, got: %s", output)
	}
	// Short secret should be fully masked
	if contains(output, "abc") && !contains(output, "****") {
		t.Fatalf("expected SHORT_SECRET to be masked, got: %s", output)
	}
	// Non-sensitive should be visible
	if !contains(output, "production") {
		t.Fatalf("expected NODE_ENV to be visible, got: %s", output)
	}
}

func TestRunList_ShowSecrets(t *testing.T) {
	appSlug = "myapp"
	showSecrets = true
	defer func() { appSlug = ""; showSecrets = false }()

	deps = &Deps{
		GetToken: func() (string, error) { return "tok123", nil },
		GetEnvVars: func(token, slug string) ([]api.EnvVar, error) {
			return []api.EnvVar{
				{Key: "DATABASE_URL", Value: "postgres://user:pass@host:5432/db"},
				{Key: "API_KEY", Value: "sk-1234567890abcdef"},
				{Key: "NODE_ENV", Value: "production"},
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

	// With --show-secrets, all values should be visible
	if !contains(output, "postgres://user:pass@host:5432/db") {
		t.Fatalf("expected full DATABASE_URL with --show-secrets, got: %s", output)
	}
	if !contains(output, "sk-1234567890abcdef") {
		t.Fatalf("expected full API_KEY with --show-secrets, got: %s", output)
	}
	if !contains(output, "production") {
		t.Fatalf("expected NODE_ENV visible, got: %s", output)
	}
}

func TestRunUnset_APIError(t *testing.T) {
	appSlug = "myapp"
	defer func() { appSlug = "" }()

	deps = &Deps{
		GetToken: func() (string, error) { return "tok123", nil },
		UnsetEnvVar: func(token, slug, key string) error {
			return fmt.Errorf("not found")
		},
	}
	defer func() { deps = defaultDeps() }()

	err := runUnset(nil, []string{"MISSING"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !contains(err.Error(), "unsetting MISSING") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestRunSet_AppResolution(t *testing.T) {
	for _, ambiguous := range []bool{false, true} {
		t.Run(fmt.Sprintf("ambiguous=%v", ambiguous), func(t *testing.T) {
			oldDeps, oldSlug := deps, appSlug
			defer func() { deps, appSlug = oldDeps, oldSlug }()
			appSlug = "target"
			apps := []api.App{{Name: "target", Slug: "duplicate"}, {Name: "original", Slug: "target"}}
			if ambiguous {
				apps[1] = api.App{Name: "target", Slug: "another"}
			}
			called := false
			deps = &Deps{
				GetToken: func() (string, error) { return "test-token", nil },
				ListApps: func(string) ([]api.App, error) { return apps, nil },
				SetEnvVar: func(token, slug, key, value string) error {
					called = true
					if slug != "target" || key != "KEY" || value != "value" {
						t.Fatalf("wrong mutation: %s %s=%s", slug, key, value)
					}
					return nil
				},
			}
			err := runSet(nil, []string{"KEY=value"})
			if (err != nil) != ambiguous || called == ambiguous {
				t.Fatalf("err=%v mutation=%v ambiguous=%v", err, called, ambiguous)
			}
		})
	}
}
