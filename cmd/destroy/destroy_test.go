package destroy

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

func TestRunDestroy_NotLoggedIn(t *testing.T) {
	deps = &Deps{
		GetToken: func() (string, error) { return "", nil },
	}
	defer func() { deps = defaultDeps() }()

	err := runDestroy(nil, []string{"myapp"})
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "not logged in. Run 'hatch login', set HATCH_TOKEN, or use --token" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunDestroy_Confirmed(t *testing.T) {
	deleted := ""
	deps = &Deps{
		GetToken: func() (string, error) { return "tok123", nil },
		ReadInput: func(prompt string) (string, error) {
			return "myapp\n", nil
		},
		DeleteApp: func(token, slug string) error {
			deleted = slug
			return nil
		},
	}
	defer func() { deps = defaultDeps() }()

	output := captureOutput(func() {
		err := runDestroy(nil, []string{"myapp"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if deleted != "myapp" {
		t.Fatalf("expected deletion of 'myapp', got %q", deleted)
	}
	if !contains(output, "Destroyed myapp") {
		t.Fatalf("expected success message, got: %s", output)
	}
}

func TestRunDestroy_WrongName(t *testing.T) {
	deps = &Deps{
		GetToken: func() (string, error) { return "tok123", nil },
		ReadInput: func(prompt string) (string, error) {
			return "wrong-name\n", nil
		},
	}
	defer func() { deps = defaultDeps() }()

	output := captureOutput(func() {
		err := runDestroy(nil, []string{"myapp"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !contains(output, "Cancelled") {
		t.Fatalf("expected cancel message, got: %s", output)
	}
}

func TestRunDestroy_APIError(t *testing.T) {
	deps = &Deps{
		GetToken: func() (string, error) { return "tok123", nil },
		ReadInput: func(prompt string) (string, error) {
			return "myapp\n", nil
		},
		DeleteApp: func(token, slug string) error {
			return fmt.Errorf("forbidden")
		},
	}
	defer func() { deps = defaultDeps() }()

	captureOutput(func() {
		err := runDestroy(nil, []string{"myapp"})
		if err == nil {
			t.Fatal("expected error")
		}
		if err.Error() != "deleting egg: forbidden" {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestRunDestroy_AutoDetect(t *testing.T) {
	deps = &Deps{
		GetToken:     func() (string, error) { return "tok123", nil },
		HasRemote:    func(name string) bool { return true },
		GetRemoteURL: func(name string) (string, error) { return "https://t@git.gethatch.eu/deploy/autoapp.git", nil },
		ReadInput: func(prompt string) (string, error) {
			return "autoapp\n", nil
		},
		DeleteApp: func(token, slug string) error {
			if slug != "autoapp" {
				t.Fatalf("expected slug 'autoapp', got %q", slug)
			}
			return nil
		},
	}
	defer func() { deps = defaultDeps() }()

	captureOutput(func() {
		err := runDestroy(nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestRunDestroy_InputError(t *testing.T) {
	deps = &Deps{
		GetToken: func() (string, error) { return "tok123", nil },
		ReadInput: func(prompt string) (string, error) {
			return "", fmt.Errorf("stdin closed")
		},
	}
	defer func() { deps = defaultDeps() }()

	captureOutput(func() {
		err := runDestroy(nil, []string{"myapp"})
		if err == nil {
			t.Fatal("expected error")
		}
		if !contains(err.Error(), "reading input") {
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
