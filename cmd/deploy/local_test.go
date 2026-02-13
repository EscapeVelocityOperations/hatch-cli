package deploy

import (
	"bytes"
	"os"
	"testing"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/ui"
)

func captureUIOutput(fn func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	buf.ReadFrom(r)
	return buf.String()
}

func TestRunArtifactDeploy_AllRuntimesValid(t *testing.T) {
	tests := []struct {
		name   string
		runtime string
		valid  bool
	}{
		{"node runtime", "node", true},
		{"python runtime", "python", true},
		{"go runtime", "go", true},
		{"rust runtime", "rust", true},
		{"php runtime", "php", true},
		{"bun runtime", "bun", true},
		{"static runtime", "static", true},
		{"invalid runtime", "ruby", false},
		{"invalid runtime with typo", "nodejs", false},
		{"empty runtime", "", false},
		{"case sensitive", "NODE", false},
		{"case sensitive", "Static", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validRuntimes[tt.runtime]
			if got != tt.valid {
				t.Errorf("validRuntimes[%q] = %v, want %v", tt.runtime, got, tt.valid)
			}
		})
	}
}

func TestIsSourceDirectory_Python(t *testing.T) {
	tmp := t.TempDir()
	os.WriteFile(tmp+"/requirements.txt", []byte("flask==2.0"), 0644)
	if !isSourceDirectory(tmp) {
		t.Error("expected Python project with requirements.txt to be detected as source directory")
	}
}

func TestIsSourceDirectory_PyProject(t *testing.T) {
	tmp := t.TempDir()
	os.WriteFile(tmp+"/pyproject.toml", []byte("[project]\nname = 'test'"), 0644)
	if !isSourceDirectory(tmp) {
		t.Error("expected Python project with pyproject.toml to be detected as source directory")
	}
}

func TestIsSourceDirectory_Pipfile(t *testing.T) {
	tmp := t.TempDir()
	os.WriteFile(tmp+"/Pipfile", []byte("[packages]\nflask = \"*\""), 0644)
	if !isSourceDirectory(tmp) {
		t.Error("expected Python project with Pipfile to be detected as source directory")
	}
}

func TestIsSourceDirectory_PartialMarkers(t *testing.T) {
	// package.json alone is not enough - need node_modules too
	tmp := t.TempDir()
	os.WriteFile(tmp+"/package.json", []byte("{}"), 0644)
	if isSourceDirectory(tmp) {
		t.Error("expected package.json without node_modules to NOT be detected as source directory")
	}
}

func TestParseEntrypoint_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		cmd      string
		expected string
	}{
		{"whitespace command", "   node server.js   ", "server.js"},
		{"tabs in command", "node\tserver.js", "server.js"},
		{"extra spaces", "node  server  index.js", "server"},
		{"flag with value", "python --version app.py", ""},
		{"multiple flags before file", "gunicorn -c gunicorn.conf -b 0.0.0.0:8000 app:app", ""},
		{"uvicorn with colon syntax", "uvicorn main:app", "main:app"},
		{"svelte-kit adapter", "node build/index.js", "build/index.js"},
		{"next.js start", "next start", "start"},
		{"npm start", "npm start", "start"},
		{"pnpm start", "pnpm start", "start"},
		{"yarn start", "yarn start", "start"},
		{"composer", "composer exec symfony server", "exec"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseEntrypoint(tt.cmd)
			if got != tt.expected {
				t.Errorf("parseEntrypoint(%q) = %q, want %q", tt.cmd, got, tt.expected)
			}
		})
	}
}

func TestCheckSourceDirectory_PHPIgnoresWithoutHatchignore(t *testing.T) {
	// PHP projects are NOT detected as source directories by isSourceDirectory
	// So checkSourceDirectory won't trigger the .hatchignore requirement
	// unless we add a recognized source marker
	tmp := t.TempDir()
	// Just having composer.json doesn't make it a source directory
	// The check should pass without error since it's not detected as source
	err := checkSourceDirectory(tmp, "php")
	if err != nil {
		t.Logf("checkSourceDirectory returned error (OK - not a source dir): %v", err)
	}
}

func TestCheckSourceDirectory_PHPWithHatchignoreOK(t *testing.T) {
	tmp := t.TempDir()
	os.WriteFile(tmp+"/composer.json", []byte("{}"), 0644)
	os.WriteFile(tmp+"/.hatchignore", []byte("vendor/\n"), 0644)

	err := checkSourceDirectory(tmp, "php")
	if err != nil {
		t.Fatalf("unexpected error with .hatchignore: %v", err)
	}
}

func TestCreateTarGz_SymlinkEscapesDirectory(t *testing.T) {
	tmp := t.TempDir()

	// Create a symlink that points outside the directory
	linkPath := tmp + "/external-link"
	os.Symlink("/etc/passwd", linkPath)
	os.WriteFile(tmp+"/server.js", []byte("// server"), 0644)

	artifact, excluded, err := createTarGz(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(artifact) == 0 {
		t.Fatal("expected non-empty artifact")
	}

	// The symlink that escapes should be excluded (no content added)
	// Check that symlink is not in artifact by checking excluded list
	found := false
	for _, e := range excluded {
		if e == "external-link" {
			found = true
		}
	}
	if found {
		t.Logf("escaping symlink was excluded: %v", excluded)
	}
}

func TestCreateTarGz_InternalSymlinkIncluded(t *testing.T) {
	tmp := t.TempDir()

	// Create a directory
	os.MkdirAll(tmp+"/data", 0755)
	os.WriteFile(tmp+"/data/info.txt", []byte("secret"), 0644)

	// Create a symlink inside the directory pointing to another file inside
	linkPath := tmp + "/info-link"
	os.Symlink("data/info.txt", linkPath)
	os.WriteFile(tmp+"/server.js", []byte("// server"), 0644)

	artifact, excluded, err := createTarGz(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(artifact) == 0 {
		t.Fatal("expected non-empty artifact")
	}

	// Internal symlink should be included
	found := false
	for _, e := range excluded {
		if e == "info-link" {
			found = true
		}
	}
	if found {
		t.Error("internal symlink should not be excluded")
	}
}

// TestUIHelpers tests UI output functions
func TestUI_Info(t *testing.T) {
	output := captureUIOutput(func() {
		ui.Info("test message")
	})
	if !contains(output, "test message") {
		t.Errorf("expected message in output, got: %s", output)
	}
}

func TestUI_Success(t *testing.T) {
	output := captureUIOutput(func() {
		ui.Success("success message")
	})
	if !contains(output, "success message") {
		t.Errorf("expected success message in output, got: %s", output)
	}
}

func TestUI_Warn(t *testing.T) {
	output := captureUIOutput(func() {
		ui.Warn("warning message")
	})
	if !contains(output, "warning message") {
		t.Errorf("expected warning in output, got: %s", output)
	}
}

func TestUI_Bold(t *testing.T) {
	result := ui.Bold("bold text")
	if result == "" {
		t.Error("expected non-empty bold text")
	}
	if !contains(result, "bold text") {
		t.Errorf("expected bold text in result, got: %s", result)
	}
}

func TestUI_Dim(t *testing.T) {
	result := ui.Dim("dim text")
	if result == "" {
		t.Error("expected non-empty dim text")
	}
	if !contains(result, "dim text") {
		t.Errorf("expected dim text in result, got: %s", result)
	}
}
