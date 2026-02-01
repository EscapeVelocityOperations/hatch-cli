package open

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

func TestRunOpen_NotLoggedIn(t *testing.T) {
	deps = &Deps{
		GetToken: func() (string, error) { return "", nil },
	}
	defer func() { deps = defaultDeps() }()

	err := runOpen(nil, []string{"myapp"})
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "not logged in. Run 'hatch login' first" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunOpen_WithSlug(t *testing.T) {
	openedURL := ""
	deps = &Deps{
		GetToken: func() (string, error) { return "tok123", nil },
		OpenBrowser: func(url string) error {
			openedURL = url
			return nil
		},
	}
	defer func() { deps = defaultDeps() }()

	output := captureOutput(func() {
		err := runOpen(nil, []string{"myapp"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if openedURL != "https://myapp.gethatch.eu" {
		t.Fatalf("expected URL 'https://myapp.gethatch.eu', got %q", openedURL)
	}
	if !contains(output, "https://myapp.gethatch.eu") {
		t.Fatalf("expected URL in output, got: %s", output)
	}
}

func TestRunOpen_AutoDetect(t *testing.T) {
	openedURL := ""
	deps = &Deps{
		GetToken:     func() (string, error) { return "tok123", nil },
		HasRemote:    func(name string) bool { return true },
		GetRemoteURL: func(name string) (string, error) { return "https://t@git.gethatch.eu/deploy/detected.git", nil },
		OpenBrowser: func(url string) error {
			openedURL = url
			return nil
		},
	}
	defer func() { deps = defaultDeps() }()

	captureOutput(func() {
		err := runOpen(nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if openedURL != "https://detected.gethatch.eu" {
		t.Fatalf("expected URL 'https://detected.gethatch.eu', got %q", openedURL)
	}
}

func TestRunOpen_NoRemoteNoArg(t *testing.T) {
	deps = &Deps{
		GetToken:  func() (string, error) { return "tok123", nil },
		HasRemote: func(name string) bool { return false },
	}
	defer func() { deps = defaultDeps() }()

	err := runOpen(nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !contains(err.Error(), "no app specified") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunOpen_BrowserError(t *testing.T) {
	deps = &Deps{
		GetToken: func() (string, error) { return "tok123", nil },
		OpenBrowser: func(url string) error {
			return fmt.Errorf("no browser found")
		},
	}
	defer func() { deps = defaultDeps() }()

	captureOutput(func() {
		err := runOpen(nil, []string{"myapp"})
		if err == nil {
			t.Fatal("expected error")
		}
		if err.Error() != "opening browser: no browser found" {
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
