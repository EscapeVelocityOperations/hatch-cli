package deploy

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
)

// mockAPIClient implements the APIClient interface for testing.
type mockAPIClient struct {
	createAppFn func(name string) (*api.App, error)
}

func (m *mockAPIClient) CreateApp(name string) (*api.App, error) {
	if m.createAppFn != nil {
		return m.createAppFn(name)
	}
	// Default: return app with slug = name + "-abc1"
	return &api.App{Slug: name + "-abc1", Name: name}, nil
}

// newMockAPIClient returns a factory function that returns the mock.
func newMockAPIClient(mock *mockAPIClient) func(token string) APIClient {
	return func(token string) APIClient {
		return mock
	}
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

func TestRunDeploy_NotLoggedIn(t *testing.T) {
	deps = &Deps{
		GetToken: func() (string, error) { return "", nil },
	}
	defer func() { deps = defaultDeps() }()

	err := runDeploy(nil, nil)
	if err == nil {
		t.Fatal("expected error for unauthenticated user")
	}
	if err.Error() != "not logged in. Run 'hatch login', set HATCH_TOKEN, or use --token" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunDeploy_TokenError(t *testing.T) {
	deps = &Deps{
		GetToken: func() (string, error) { return "", fmt.Errorf("disk error") },
	}
	defer func() { deps = defaultDeps() }()

	err := runDeploy(nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "checking auth: disk error" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunDeploy_InitGitIfNeeded(t *testing.T) {
	gitInitCalled := false
	deps = &Deps{
		GetToken:  func() (string, error) { return "tok123", nil },
		IsGitRepo: func() bool { return false },
		GitInit:   func() error { gitInitCalled = true; return nil },
		HasChanges: func() (bool, error) { return false, nil },
		HasRemote: func(name string) bool { return false },
		AddRemote: func(name, url string) error { return nil },
		CurrentBranch: func() (string, error) { return "main", nil },
		Push: func(remote, branch string) (string, error) {
			return "remote: https://myapp-abc1.gethatch.eu\n", nil
		},
		GetCwd:       func() (string, error) { return "/tmp/myapp", nil },
		NewAPIClient: newMockAPIClient(&mockAPIClient{}),
	}
	defer func() { deps = defaultDeps() }()

	captureOutput(func() {
		err := runDeploy(nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !gitInitCalled {
		t.Fatal("expected git init to be called")
	}
}

func TestRunDeploy_CommitsChanges(t *testing.T) {
	commitMsg := ""
	deps = &Deps{
		GetToken:  func() (string, error) { return "tok123", nil },
		IsGitRepo: func() bool { return true },
		HasChanges: func() (bool, error) { return true, nil },
		CommitAll: func(msg string) error { commitMsg = msg; return nil },
		HasRemote: func(name string) bool { return false },
		AddRemote: func(name, url string) error { return nil },
		CurrentBranch: func() (string, error) { return "main", nil },
		Push:         func(remote, branch string) (string, error) { return "", nil },
		GetCwd:       func() (string, error) { return "/tmp/myapp", nil },
		NewAPIClient: newMockAPIClient(&mockAPIClient{}),
	}
	defer func() { deps = defaultDeps() }()

	captureOutput(func() {
		err := runDeploy(nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if commitMsg != "Deploy via hatch" {
		t.Fatalf("expected commit message 'Deploy via hatch', got %q", commitMsg)
	}
}

func TestRunDeploy_AddsRemoteWhenMissing(t *testing.T) {
	addedRemote := ""
	addedURL := ""
	deps = &Deps{
		GetToken:  func() (string, error) { return "tok123", nil },
		IsGitRepo: func() bool { return true },
		HasChanges: func() (bool, error) { return false, nil },
		HasRemote: func(name string) bool { return false },
		AddRemote: func(name, url string) error {
			addedRemote = name
			addedURL = url
			return nil
		},
		CurrentBranch: func() (string, error) { return "main", nil },
		Push:         func(remote, branch string) (string, error) { return "", nil },
		GetCwd:       func() (string, error) { return "/tmp/myapp", nil },
		NewAPIClient: newMockAPIClient(&mockAPIClient{}),
	}
	defer func() { deps = defaultDeps() }()

	captureOutput(func() {
		err := runDeploy(nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if addedRemote != "hatch" {
		t.Fatalf("expected remote name 'hatch', got %q", addedRemote)
	}
	// URL now uses /deploy/ path and slug (name + "-abc1" from mock)
	expected := "https://x:tok123@git.gethatch.eu/deploy/myapp-abc1.git"
	if addedURL != expected {
		t.Fatalf("expected URL %q, got %q", expected, addedURL)
	}
}

func TestRunDeploy_UpdatesExistingRemote(t *testing.T) {
	updatedURL := ""
	deps = &Deps{
		GetToken:  func() (string, error) { return "tok123", nil },
		IsGitRepo: func() bool { return true },
		HasChanges: func() (bool, error) { return false, nil },
		HasRemote: func(name string) bool { return true },
		SetRemoteURL: func(name, url string) error {
			updatedURL = url
			return nil
		},
		CurrentBranch: func() (string, error) { return "main", nil },
		Push:         func(remote, branch string) (string, error) { return "", nil },
		GetCwd:       func() (string, error) { return "/tmp/myapp", nil },
		NewAPIClient: newMockAPIClient(&mockAPIClient{}),
	}
	defer func() { deps = defaultDeps() }()

	captureOutput(func() {
		err := runDeploy(nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// URL now uses /deploy/ path and slug (name + "-abc1" from mock)
	expected := "https://x:tok123@git.gethatch.eu/deploy/myapp-abc1.git"
	if updatedURL != expected {
		t.Fatalf("expected URL %q, got %q", expected, updatedURL)
	}
}

func TestRunDeploy_PushesBranchAsMain(t *testing.T) {
	pushedRemote := ""
	pushedBranch := ""
	deps = &Deps{
		GetToken:  func() (string, error) { return "tok123", nil },
		IsGitRepo: func() bool { return true },
		HasChanges: func() (bool, error) { return false, nil },
		HasRemote: func(name string) bool { return false },
		AddRemote: func(name, url string) error { return nil },
		CurrentBranch: func() (string, error) { return "feature-x", nil },
		Push: func(remote, branch string) (string, error) {
			pushedRemote = remote
			pushedBranch = branch
			return "", nil
		},
		GetCwd:       func() (string, error) { return "/tmp/myapp", nil },
		NewAPIClient: newMockAPIClient(&mockAPIClient{}),
	}
	defer func() { deps = defaultDeps() }()

	captureOutput(func() {
		err := runDeploy(nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if pushedRemote != "hatch" {
		t.Fatalf("expected push to 'hatch', got %q", pushedRemote)
	}
	if pushedBranch != "feature-x:main" {
		t.Fatalf("expected push ref 'feature-x:main', got %q", pushedBranch)
	}
}

func TestRunDeploy_CustomName(t *testing.T) {
	addedURL := ""
	appName = "custom-app"
	defer func() { appName = "" }()

	deps = &Deps{
		GetToken:  func() (string, error) { return "tok123", nil },
		IsGitRepo: func() bool { return true },
		HasChanges: func() (bool, error) { return false, nil },
		HasRemote: func(name string) bool { return false },
		AddRemote: func(name, url string) error {
			addedURL = url
			return nil
		},
		CurrentBranch: func() (string, error) { return "main", nil },
		Push:         func(remote, branch string) (string, error) { return "", nil },
		GetCwd:       func() (string, error) { return "/tmp/myapp", nil },
		NewAPIClient: newMockAPIClient(&mockAPIClient{}),
	}
	defer func() { deps = defaultDeps() }()

	captureOutput(func() {
		err := runDeploy(nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// URL now uses /deploy/ path and slug (custom-app + "-abc1" from mock)
	expected := "https://x:tok123@git.gethatch.eu/deploy/custom-app-abc1.git"
	if addedURL != expected {
		t.Fatalf("expected URL %q, got %q", expected, addedURL)
	}
}

func TestRunDeploy_PushFailure(t *testing.T) {
	deps = &Deps{
		GetToken:  func() (string, error) { return "tok123", nil },
		IsGitRepo: func() bool { return true },
		HasChanges: func() (bool, error) { return false, nil },
		HasRemote: func(name string) bool { return false },
		AddRemote: func(name, url string) error { return nil },
		CurrentBranch: func() (string, error) { return "main", nil },
		Push: func(remote, branch string) (string, error) {
			return "fatal: repository not found", fmt.Errorf("exit status 128")
		},
		GetCwd:       func() (string, error) { return "/tmp/myapp", nil },
		NewAPIClient: newMockAPIClient(&mockAPIClient{}),
	}
	defer func() { deps = defaultDeps() }()

	captureOutput(func() {
		err := runDeploy(nil, nil)
		if err == nil {
			t.Fatal("expected error on push failure")
		}
		if err.Error() != "push failed: fatal: repository not found" {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestRunDeploy_CreateAppFailure(t *testing.T) {
	deps = &Deps{
		GetToken:   func() (string, error) { return "tok123", nil },
		IsGitRepo:  func() bool { return true },
		HasChanges: func() (bool, error) { return false, nil },
		GetCwd:     func() (string, error) { return "/tmp/myapp", nil },
		NewAPIClient: newMockAPIClient(&mockAPIClient{
			createAppFn: func(name string) (*api.App, error) {
				return nil, fmt.Errorf("API error 500: internal server error")
			},
		}),
	}
	defer func() { deps = defaultDeps() }()

	captureOutput(func() {
		err := runDeploy(nil, nil)
		if err == nil {
			t.Fatal("expected error on API failure")
		}
		if err.Error() != "creating app: API error 500: internal server error" {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestParseAppURL(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			name:   "URL in output",
			output: "remote: Deploying...\nremote: https://myapp.gethatch.eu\nTo ...",
			want:   "https://myapp.gethatch.eu",
		},
		{
			name:   "URL with path",
			output: "remote: https://myapp.gethatch.eu/status",
			want:   "https://myapp.gethatch.eu/status",
		},
		{
			name:   "no URL",
			output: "Everything up-to-date",
			want:   "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseAppURL(tt.output)
			if got != tt.want {
				t.Errorf("parseAppURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRunDeploy_SuccessOutput(t *testing.T) {
	deps = &Deps{
		GetToken:  func() (string, error) { return "tok123", nil },
		IsGitRepo: func() bool { return true },
		HasChanges: func() (bool, error) { return false, nil },
		HasRemote: func(name string) bool { return false },
		AddRemote: func(name, url string) error { return nil },
		CurrentBranch: func() (string, error) { return "main", nil },
		Push: func(remote, branch string) (string, error) {
			return "remote: https://coolapp-abc1.gethatch.eu\n", nil
		},
		GetCwd:       func() (string, error) { return "/tmp/coolapp", nil },
		NewAPIClient: newMockAPIClient(&mockAPIClient{}),
	}
	defer func() { deps = defaultDeps() }()

	output := captureOutput(func() {
		err := runDeploy(nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !contains(output, "Deployed to Hatch!") {
		t.Errorf("expected success message in output, got: %s", output)
	}
	if !contains(output, "https://coolapp-abc1.gethatch.eu") {
		t.Errorf("expected app URL in output, got: %s", output)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
