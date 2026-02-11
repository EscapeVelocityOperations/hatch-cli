package deploy

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
)

// mockAPIClient implements the APIClient interface for testing.
type mockAPIClient struct {
	createAppFn      func(name string) (*api.App, error)
	uploadArtifactFn func(slug string, artifact []byte, runtime, startCommand string) error
}

func (m *mockAPIClient) CreateApp(name string) (*api.App, error) {
	if m.createAppFn != nil {
		return m.createAppFn(name)
	}
	return &api.App{Slug: name + "-abc1", Name: name}, nil
}

func (m *mockAPIClient) UploadArtifact(slug string, artifact []byte, runtime, startCommand string) error {
	if m.uploadArtifactFn != nil {
		return m.uploadArtifactFn(slug, artifact, runtime, startCommand)
	}
	return nil
}

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
	tmp := t.TempDir()
	deps = &Deps{
		GetToken: func() (string, error) { return "", nil },
	}
	defer func() { deps = defaultDeps(); deployTarget = ""; runtime = "" }()

	deployTarget = tmp
	runtime = "node"

	err := runDeploy(nil, nil)
	if err == nil {
		t.Fatal("expected error for unauthenticated user")
	}
	if err.Error() != "not logged in. Run 'hatch login', set HATCH_TOKEN, or use --token" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunDeploy_TokenError(t *testing.T) {
	tmp := t.TempDir()
	deps = &Deps{
		GetToken: func() (string, error) { return "", fmt.Errorf("disk error") },
	}
	defer func() { deps = defaultDeps(); deployTarget = ""; runtime = "" }()

	deployTarget = tmp
	runtime = "node"

	err := runDeploy(nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "checking auth: disk error" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunDeploy_ArtifactMode_MissingRuntime(t *testing.T) {
	tmp := t.TempDir()

	deps = &Deps{
		GetToken: func() (string, error) { return "tok123", nil },
		GetCwd:   func() (string, error) { return tmp, nil },
	}
	defer func() { deps = defaultDeps(); deployTarget = ""; runtime = "" }()

	deployTarget = tmp
	runtime = "" // Missing!

	err := runDeploy(nil, nil)
	if err == nil {
		t.Fatal("expected error for missing runtime")
	}
	if !contains(err.Error(), "--runtime is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunDeploy_ArtifactMode_InvalidRuntime(t *testing.T) {
	tmp := t.TempDir()

	deps = &Deps{
		GetToken: func() (string, error) { return "tok123", nil },
		GetCwd:   func() (string, error) { return tmp, nil },
	}
	defer func() { deps = defaultDeps(); deployTarget = ""; runtime = "" }()

	deployTarget = tmp
	runtime = "django" // Invalid

	err := runDeploy(nil, nil)
	if err == nil {
		t.Fatal("expected error for invalid runtime")
	}
	if !contains(err.Error(), "unknown runtime") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunDeploy_ArtifactMode_NonStaticMissingStartCommand(t *testing.T) {
	tmp := t.TempDir()

	deps = &Deps{
		GetToken: func() (string, error) { return "tok123", nil },
		GetCwd:   func() (string, error) { return tmp, nil },
	}
	defer func() { deps = defaultDeps(); deployTarget = ""; runtime = ""; startCommand = "" }()

	deployTarget = tmp
	runtime = "node"
	startCommand = "" // Missing for non-static!

	err := runDeploy(nil, nil)
	if err == nil {
		t.Fatal("expected error for missing start command")
	}
	if !contains(err.Error(), "start-command is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunDeploy_ArtifactMode_StaticSuccess(t *testing.T) {
	tmp := t.TempDir()

	var uploadedSlug, uploadedRuntime string
	deps = &Deps{
		GetToken:     func() (string, error) { return "tok123", nil },
		GetCwd:       func() (string, error) { return tmp, nil },
		NewAPIClient: newMockAPIClient(&mockAPIClient{
			uploadArtifactFn: func(slug string, artifact []byte, rt, sc string) error {
				uploadedSlug = slug
				uploadedRuntime = rt
				return nil
			},
		}),
	}
	defer func() { deps = defaultDeps(); deployTarget = ""; runtime = ""; startCommand = "" }()

	deployTarget = tmp
	runtime = "static"

	captureOutput(func() {
		err := runDeploy(nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// Should have created an app (no .hatch.toml in tmp)
	if uploadedSlug == "" {
		t.Fatal("expected upload to be called")
	}
	if uploadedRuntime != "static" {
		t.Fatalf("expected runtime 'static', got %q", uploadedRuntime)
	}
}

func TestRunDeploy_ArtifactMode_ReadsHatchToml(t *testing.T) {
	tmp := t.TempDir()

	// Write .hatch.toml
	tomlContent := "[app]\nslug = \"mysite-x1y2\"\nname = \"mysite\"\n"
	os.WriteFile(filepath.Join(tmp, ".hatch.toml"), []byte(tomlContent), 0644)

	var uploadedSlug string
	deps = &Deps{
		GetToken:     func() (string, error) { return "tok123", nil },
		GetCwd:       func() (string, error) { return tmp, nil },
		NewAPIClient: newMockAPIClient(&mockAPIClient{
			uploadArtifactFn: func(slug string, artifact []byte, rt, sc string) error {
				uploadedSlug = slug
				return nil
			},
		}),
	}
	defer func() { deps = defaultDeps(); deployTarget = ""; runtime = "" }()

	deployTarget = tmp
	runtime = "static"

	// Need to change to tmp dir so readHatchConfig finds .hatch.toml
	oldDir, _ := os.Getwd()
	os.Chdir(tmp)
	defer os.Chdir(oldDir)

	captureOutput(func() {
		err := runDeploy(nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if uploadedSlug != "mysite-x1y2" {
		t.Fatalf("expected slug 'mysite-x1y2', got %q", uploadedSlug)
	}
}

func TestRunDeploy_CreateAppFailure(t *testing.T) {
	tmp := t.TempDir()

	deps = &Deps{
		GetToken: func() (string, error) { return "tok123", nil },
		GetCwd:   func() (string, error) { return tmp, nil },
		NewAPIClient: newMockAPIClient(&mockAPIClient{
			createAppFn: func(name string) (*api.App, error) {
				return nil, fmt.Errorf("API error 500: internal server error")
			},
		}),
	}
	defer func() { deps = defaultDeps(); deployTarget = ""; runtime = ""; startCommand = "" }()

	deployTarget = tmp
	runtime = "static"

	// Change to tmp dir so no stale .hatch.toml is found
	oldDir, _ := os.Getwd()
	os.Chdir(tmp)
	defer os.Chdir(oldDir)

	captureOutput(func() {
		err := runDeploy(nil, nil)
		if err == nil {
			t.Fatal("expected error on API failure")
		}
		if !contains(err.Error(), "creating egg") {
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
