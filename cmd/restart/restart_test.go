package restart

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"testing"
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

func TestRunRestart_NotLoggedIn(t *testing.T) {
	deps = &Deps{
		GetToken: func() (string, error) { return "", nil },
	}
	defer func() { deps = defaultDeps() }()

	err := runRestart(nil, []string{"myapp"})
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "not logged in. Run 'hatch login', set HATCH_TOKEN, or use --token" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunRestart_Confirmed(t *testing.T) {
	restarted := ""
	deps = &Deps{
		GetToken: func() (string, error) { return "tok123", nil },
		Confirm:  func(prompt string) bool { return true },
		RestartApp: func(token, slug string) error {
			restarted = slug
			return nil
		},
	}
	defer func() { deps = defaultDeps() }()

	output := captureOutput(func() {
		err := runRestart(nil, []string{"myapp"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if restarted != "myapp" {
		t.Fatalf("expected restart of 'myapp', got %q", restarted)
	}
	if !contains(output, "Restarted myapp") {
		t.Fatalf("expected success message, got: %s", output)
	}
}

func TestRunRestart_Cancelled(t *testing.T) {
	deps = &Deps{
		GetToken: func() (string, error) { return "tok123", nil },
		Confirm:  func(prompt string) bool { return false },
	}
	defer func() { deps = defaultDeps() }()

	output := captureOutput(func() {
		err := runRestart(nil, []string{"myapp"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !contains(output, "Cancelled") {
		t.Fatalf("expected cancel message, got: %s", output)
	}
}

func TestRunRestart_APIError(t *testing.T) {
	deps = &Deps{
		GetToken: func() (string, error) { return "tok123", nil },
		Confirm:  func(prompt string) bool { return true },
		RestartApp: func(token, slug string) error {
			return fmt.Errorf("server error")
		},
	}
	defer func() { deps = defaultDeps() }()

	captureOutput(func() {
		err := runRestart(nil, []string{"myapp"})
		if err == nil {
			t.Fatal("expected error")
		}
		if err.Error() != "restarting app: server error" {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestRunRestart_AutoDetect(t *testing.T) {
	deps = &Deps{
		GetToken:     func() (string, error) { return "tok123", nil },
		HasRemote:    func(name string) bool { return true },
		GetRemoteURL: func(name string) (string, error) { return "https://t@git.gethatch.eu/deploy/detected.git", nil },
		Confirm:      func(prompt string) bool { return true },
		RestartApp: func(token, slug string) error {
			if slug != "detected" {
				t.Fatalf("expected slug 'detected', got %q", slug)
			}
			return nil
		},
	}
	defer func() { deps = defaultDeps() }()

	captureOutput(func() {
		err := runRestart(nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
