package logs

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

func TestRunLogs_NotLoggedIn(t *testing.T) {
	deps = &Deps{
		GetToken: func() (string, error) { return "", nil },
	}
	defer func() { deps = defaultDeps() }()

	err := runLogs(nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "not logged in. Run 'hatch login', set HATCH_TOKEN, or use --token" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunLogs_WithSlugArg(t *testing.T) {
	streamedSlug := ""
	deps = &Deps{
		GetToken: func() (string, error) { return "tok123", nil },
		StreamLogs: func(token, slug string, linesN int, followN bool, logType string, handler func(string)) error {
			streamedSlug = slug
			handler("log line 1")
			return nil
		},
	}
	defer func() { deps = defaultDeps() }()

	output := captureOutput(func() {
		err := runLogs(nil, []string{"myapp"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if streamedSlug != "myapp" {
		t.Fatalf("expected slug 'myapp', got %q", streamedSlug)
	}
	if !contains(output, "log line 1") {
		t.Fatalf("expected log output, got: %s", output)
	}
}

func TestRunLogs_AutoDetectFromRemote(t *testing.T) {
	deps = &Deps{
		GetToken:     func() (string, error) { return "tok123", nil },
		HasRemote:    func(name string) bool { return true },
		GetRemoteURL: func(name string) (string, error) { return "https://tok@git.gethatch.eu/deploy/detected.git", nil },
		StreamLogs: func(token, slug string, linesN int, followN bool, logType string, handler func(string)) error {
			if slug != "detected" {
				t.Fatalf("expected auto-detected slug 'detected', got %q", slug)
			}
			return nil
		},
	}
	defer func() { deps = defaultDeps() }()

	captureOutput(func() {
		err := runLogs(nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestRunLogs_NoRemoteNoArg(t *testing.T) {
	deps = &Deps{
		GetToken:  func() (string, error) { return "tok123", nil },
		HasRemote: func(name string) bool { return false },
	}
	defer func() { deps = defaultDeps() }()

	err := runLogs(nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !contains(err.Error(), "no app specified") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunLogs_StreamError(t *testing.T) {
	deps = &Deps{
		GetToken: func() (string, error) { return "tok123", nil },
		StreamLogs: func(token, slug string, linesN int, followN bool, logType string, handler func(string)) error {
			return fmt.Errorf("connection reset")
		},
	}
	defer func() { deps = defaultDeps() }()

	captureOutput(func() {
		err := runLogs(nil, []string{"myapp"})
		if err == nil {
			t.Fatal("expected error")
		}
		if err.Error() != "connection reset" {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestRunLogs_BuildFlag(t *testing.T) {
	var capturedLogType string
	deps = &Deps{
		GetToken: func() (string, error) { return "tok123", nil },
		StreamLogs: func(token, slug string, linesN int, followN bool, logType string, handler func(string)) error {
			capturedLogType = logType
			handler("build output")
			return nil
		},
	}
	defer func() { deps = defaultDeps() }()

	build = true
	defer func() { build = false }()

	output := captureOutput(func() {
		err := runLogs(nil, []string{"myapp"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if capturedLogType != "build" {
		t.Fatalf("expected logType 'build', got %q", capturedLogType)
	}
	if !contains(output, "build output") {
		t.Fatalf("expected build output, got: %s", output)
	}
}

func TestRunLogs_LinesFlag(t *testing.T) {
	var capturedLines int
	deps = &Deps{
		GetToken: func() (string, error) { return "tok123", nil },
		StreamLogs: func(token, slug string, linesN int, followN bool, logType string, handler func(string)) error {
			capturedLines = linesN
			return nil
		},
	}
	defer func() { deps = defaultDeps() }()

	lines = 50
	defer func() { lines = 100 }()

	captureOutput(func() {
		err := runLogs(nil, []string{"myapp"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if capturedLines != 50 {
		t.Fatalf("expected 50 lines, got %d", capturedLines)
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
