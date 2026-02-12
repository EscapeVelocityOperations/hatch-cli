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

// --- .hatchignore integration tests ---

func TestCreateTarGz_DefaultsExcludeGitAndEnv(t *testing.T) {
	tmp := t.TempDir()
	os.MkdirAll(filepath.Join(tmp, ".git"), 0755)
	os.WriteFile(filepath.Join(tmp, ".env"), []byte("SECRET=x"), 0644)
	os.WriteFile(filepath.Join(tmp, ".env.local"), []byte("LOCAL=y"), 0644)
	os.WriteFile(filepath.Join(tmp, "server.js"), []byte("console.log('hi')"), 0644)

	artifact, excluded, err := createTarGz(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(artifact) == 0 {
		t.Fatal("expected non-empty artifact")
	}

	// .git/ and .env* should be excluded
	foundGit := false
	foundEnv := false
	for _, e := range excluded {
		if e == ".git/" {
			foundGit = true
		}
		if e == ".env" || e == ".env.local" {
			foundEnv = true
		}
	}
	if !foundGit {
		t.Errorf("expected .git/ in excluded list, got: %v", excluded)
	}
	if !foundEnv {
		t.Errorf("expected .env files in excluded list, got: %v", excluded)
	}
}

func TestCreateTarGz_HatchignoreExcludesNodeModules(t *testing.T) {
	tmp := t.TempDir()
	os.MkdirAll(filepath.Join(tmp, "node_modules", "express"), 0755)
	os.WriteFile(filepath.Join(tmp, "node_modules", "express", "index.js"), []byte("module.exports = {}"), 0644)
	os.WriteFile(filepath.Join(tmp, "server.js"), []byte("require('express')"), 0644)
	os.WriteFile(filepath.Join(tmp, ".hatchignore"), []byte("node_modules/\n"), 0644)

	_, excluded, err := createTarGz(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, e := range excluded {
		if e == "node_modules/" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected node_modules/ in excluded list, got: %v", excluded)
	}
}

func TestCreateTarGz_NitroNotExcludedByDefault(t *testing.T) {
	// Regression test: .nitro inside node_modules must NOT be excluded
	tmp := t.TempDir()
	nitroDir := filepath.Join(tmp, "node_modules", ".nitro")
	os.MkdirAll(nitroDir, 0755)
	os.WriteFile(filepath.Join(nitroDir, "index.js"), []byte("// nitro"), 0644)
	os.WriteFile(filepath.Join(tmp, "server.js"), []byte("// server"), 0644)

	_, excluded, err := createTarGz(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, e := range excluded {
		if contains(e, ".nitro") {
			t.Errorf(".nitro should NOT be excluded by defaults, but found in excluded: %v", excluded)
		}
	}
}

func TestCreateTarGz_HatchignoreNegation(t *testing.T) {
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "app.log"), []byte("log"), 0644)
	os.WriteFile(filepath.Join(tmp, "important.log"), []byte("keep"), 0644)
	os.WriteFile(filepath.Join(tmp, "server.js"), []byte("// srv"), 0644)
	os.WriteFile(filepath.Join(tmp, ".hatchignore"), []byte("*.log\n!important.log\n"), 0644)

	_, excluded, err := createTarGz(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	foundApp := false
	foundImportant := false
	for _, e := range excluded {
		if e == "app.log" {
			foundApp = true
		}
		if e == "important.log" {
			foundImportant = true
		}
	}
	if !foundApp {
		t.Errorf("expected app.log in excluded list, got: %v", excluded)
	}
	if foundImportant {
		t.Errorf("expected important.log NOT in excluded (negated), got: %v", excluded)
	}
}

func TestIsSourceDirectory(t *testing.T) {
	// Node project
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "package.json"), []byte("{}"), 0644)
	os.MkdirAll(filepath.Join(tmp, "node_modules"), 0755)
	if !isSourceDirectory(tmp) {
		t.Error("expected Node project to be detected as source directory")
	}

	// Go project
	tmp2 := t.TempDir()
	os.WriteFile(filepath.Join(tmp2, "go.mod"), []byte("module test"), 0644)
	if !isSourceDirectory(tmp2) {
		t.Error("expected Go project to be detected as source directory")
	}

	// Build output (no markers)
	tmp3 := t.TempDir()
	os.WriteFile(filepath.Join(tmp3, "server.js"), []byte("// server"), 0644)
	if isSourceDirectory(tmp3) {
		t.Error("expected build output to NOT be detected as source directory")
	}
}

func TestCheckSourceDirectory_StaticRequiresHatchignore(t *testing.T) {
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "package.json"), []byte("{}"), 0644)
	os.MkdirAll(filepath.Join(tmp, "node_modules"), 0755)

	err := checkSourceDirectory(tmp, "static")
	if err == nil {
		t.Fatal("expected error for static runtime without .hatchignore")
	}
	if !contains(err.Error(), "requires a .hatchignore") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestCheckSourceDirectory_StaticWithHatchignoreOK(t *testing.T) {
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "package.json"), []byte("{}"), 0644)
	os.MkdirAll(filepath.Join(tmp, "node_modules"), 0755)
	os.WriteFile(filepath.Join(tmp, ".hatchignore"), []byte("node_modules/\n"), 0644)

	err := checkSourceDirectory(tmp, "static")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckSourceDirectory_NodeWarnsButProceeds(t *testing.T) {
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "package.json"), []byte("{}"), 0644)
	os.MkdirAll(filepath.Join(tmp, "node_modules"), 0755)

	// Should warn but return nil (no error)
	err := checkSourceDirectory(tmp, "node")
	if err != nil {
		t.Fatalf("expected no error for node runtime (just warning), got: %v", err)
	}
}

func TestCheckSourceDirectory_BuildOutputNoWarning(t *testing.T) {
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "server.js"), []byte("// server"), 0644)

	err := checkSourceDirectory(tmp, "node")
	if err != nil {
		t.Fatalf("expected no error for build output directory, got: %v", err)
	}
}
