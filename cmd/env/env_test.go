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

func TestRunList_AutoDetect(t *testing.T) {
	appSlug = ""
	deps = &Deps{
		GetToken:     func() (string, error) { return "tok123", nil },
		HasRemote:    func(name string) bool { return true },
		GetRemoteURL: func(name string) (string, error) { return "https://t@git.gethatch.eu/deploy/auto.git", nil },
		GetEnvVars: func(token, slug string) ([]api.EnvVar, error) {
			if slug != "auto" {
				t.Fatalf("expected slug 'auto', got %q", slug)
			}
			return nil, nil
		},
	}
	defer func() { deps = defaultDeps() }()

	captureOutput(func() {
		err := runList(nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
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
