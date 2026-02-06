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
	uploadArtifactFn func(slug string, artifact []byte, framework, startCommand string) error
}

func (m *mockAPIClient) CreateApp(name string) (*api.App, error) {
	if m.createAppFn != nil {
		return m.createAppFn(name)
	}
	return &api.App{Slug: name + "-abc1", Name: name}, nil
}

func (m *mockAPIClient) UploadArtifact(slug string, artifact []byte, framework, startCommand string) error {
	if m.uploadArtifactFn != nil {
		return m.uploadArtifactFn(slug, artifact, framework, startCommand)
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

func TestRunDeploy_ArtifactMode_MissingFramework(t *testing.T) {
	// Create a temp artifact file
	tmp := t.TempDir()
	artifactFile := filepath.Join(tmp, "site.tar.gz")
	os.WriteFile(artifactFile, []byte("fake"), 0644)

	deps = &Deps{
		GetToken: func() (string, error) { return "tok123", nil },
		GetCwd:   func() (string, error) { return tmp, nil },
	}
	defer func() { deps = defaultDeps(); artifactPath = ""; framework = "" }()

	artifactPath = artifactFile
	framework = "" // Missing!

	err := runDeploy(nil, nil)
	if err == nil {
		t.Fatal("expected error for missing framework")
	}
	if err.Error() != "--framework is required when using --artifact" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunDeploy_ArtifactMode_InvalidFramework(t *testing.T) {
	tmp := t.TempDir()
	artifactFile := filepath.Join(tmp, "site.tar.gz")
	os.WriteFile(artifactFile, []byte("fake"), 0644)

	deps = &Deps{
		GetToken: func() (string, error) { return "tok123", nil },
		GetCwd:   func() (string, error) { return tmp, nil },
	}
	defer func() { deps = defaultDeps(); artifactPath = ""; framework = "" }()

	artifactPath = artifactFile
	framework = "django" // Invalid

	err := runDeploy(nil, nil)
	if err == nil {
		t.Fatal("expected error for invalid framework")
	}
	if !contains(err.Error(), "unknown framework") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunDeploy_ArtifactMode_NonStaticMissingStartCommand(t *testing.T) {
	tmp := t.TempDir()
	artifactFile := filepath.Join(tmp, "app.tar.gz")
	os.WriteFile(artifactFile, []byte("fake"), 0644)

	deps = &Deps{
		GetToken: func() (string, error) { return "tok123", nil },
		GetCwd:   func() (string, error) { return tmp, nil },
	}
	defer func() { deps = defaultDeps(); artifactPath = ""; framework = ""; startCommand = "" }()

	artifactPath = artifactFile
	framework = "node"
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
	artifactFile := filepath.Join(tmp, "site.tar.gz")
	os.WriteFile(artifactFile, []byte("fake-artifact"), 0644)

	var uploadedSlug, uploadedFramework string
	deps = &Deps{
		GetToken:     func() (string, error) { return "tok123", nil },
		GetCwd:       func() (string, error) { return tmp, nil },
		NewAPIClient: newMockAPIClient(&mockAPIClient{
			uploadArtifactFn: func(slug string, artifact []byte, fw, sc string) error {
				uploadedSlug = slug
				uploadedFramework = fw
				return nil
			},
		}),
	}
	defer func() { deps = defaultDeps(); artifactPath = ""; framework = ""; startCommand = "" }()

	artifactPath = artifactFile
	framework = "static"

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
	if uploadedFramework != "static" {
		t.Fatalf("expected framework 'static', got %q", uploadedFramework)
	}
}

func TestRunDeploy_ArtifactMode_ReadsHatchToml(t *testing.T) {
	tmp := t.TempDir()
	artifactFile := filepath.Join(tmp, "site.tar.gz")
	os.WriteFile(artifactFile, []byte("fake-artifact"), 0644)

	// Write .hatch.toml
	tomlContent := "[app]\nslug = \"mysite-x1y2\"\nname = \"mysite\"\n"
	os.WriteFile(filepath.Join(tmp, ".hatch.toml"), []byte(tomlContent), 0644)

	var uploadedSlug string
	deps = &Deps{
		GetToken:     func() (string, error) { return "tok123", nil },
		GetCwd:       func() (string, error) { return tmp, nil },
		NewAPIClient: newMockAPIClient(&mockAPIClient{
			uploadArtifactFn: func(slug string, artifact []byte, fw, sc string) error {
				uploadedSlug = slug
				return nil
			},
		}),
	}
	defer func() { deps = defaultDeps(); artifactPath = ""; framework = "" }()

	artifactPath = artifactFile
	framework = "static"

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
	artifactFile := filepath.Join(tmp, "site.tar.gz")
	os.WriteFile(artifactFile, []byte("fake"), 0644)

	deps = &Deps{
		GetToken: func() (string, error) { return "tok123", nil },
		GetCwd:   func() (string, error) { return tmp, nil },
		NewAPIClient: newMockAPIClient(&mockAPIClient{
			createAppFn: func(name string) (*api.App, error) {
				return nil, fmt.Errorf("API error 500: internal server error")
			},
		}),
	}
	defer func() { deps = defaultDeps(); artifactPath = ""; framework = ""; startCommand = "" }()

	artifactPath = artifactFile
	framework = "static"

	// Change to tmp dir so no stale .hatch.toml is found
	oldDir, _ := os.Getwd()
	os.Chdir(tmp)
	defer os.Chdir(oldDir)

	captureOutput(func() {
		err := runDeploy(nil, nil)
		if err == nil {
			t.Fatal("expected error on API failure")
		}
		if !contains(err.Error(), "creating app") {
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
