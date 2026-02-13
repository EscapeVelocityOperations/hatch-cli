package initignore

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

)

func setupTestDir(t *testing.T) (string, func()) {
	t.Helper()
	dir := t.TempDir()

	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	cleanup := func() {
		os.Chdir(oldWd)
	}

	return dir, cleanup
}

func TestDetectRuntime(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		expected string
	}{
		{
			name:     "go project",
			files:    map[string]string{"go.mod": "module test\n"},
			expected: "go",
		},
		{
			name:     "rust project",
			files:    map[string]string{"Cargo.toml": "[package]\n"},
			expected: "rust",
		},
		{
			name:     "python project with requirements.txt",
			files:    map[string]string{"requirements.txt": "flask\n"},
			expected: "python",
		},
		{
			name:     "python project with pyproject.toml",
			files:    map[string]string{"pyproject.toml": "[project]\n"},
			expected: "python",
		},
		{
			name:     "python project with Pipfile",
			files:    map[string]string{"Pipfile": "[packages]\n"},
			expected: "python",
		},
		{
			name:     "php project",
			files:    map[string]string{"composer.json": "{}\n"},
			expected: "php",
		},
		{
			name:     "bun project with bun.lockb",
			files:    map[string]string{"bun.lockb": "binary\n"},
			expected: "bun",
		},
		{
			name:     "bun project with bunfig.toml",
			files:    map[string]string{"bunfig.toml": "[config]\n"},
			expected: "bun",
		},
		{
			name:     "node project",
			files:    map[string]string{"package.json": "{}\n"},
			expected: "node",
		},
		{
			name:     "static site",
			files:    map[string]string{"index.html": "<html></html>\n"},
			expected: "static",
		},
		{
			name:     "empty directory",
			files:    map[string]string{},
			expected: "",
		},
		{
			name:     "go takes precedence over node",
			files:    map[string]string{"go.mod": "module test\n", "package.json": "{}\n"},
			expected: "go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()

			// Create test files
			for name, content := range tt.files {
				path := filepath.Join(dir, name)
				if err := os.WriteFile(path, []byte(content), 0644); err != nil {
					t.Fatal(err)
				}
			}

			// Test detection
			got := detectRuntime(dir)
			if got != tt.expected {
				t.Errorf("detectRuntime() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestRunInitIgnore_WithRuntimeFlag(t *testing.T) {
	tests := []struct {
		name         string
		runtime      string
		expectError  bool
		expectOutput string
	}{
		{
			name:         "node runtime",
			runtime:      "node",
			expectError:  false,
			expectOutput: "Created .hatchignore for node runtime",
		},
		{
			name:         "python runtime",
			runtime:      "python",
			expectError:  false,
			expectOutput: "Created .hatchignore for python runtime",
		},
		{
			name:         "go runtime",
			runtime:      "go",
			expectError:  false,
			expectOutput: "Created .hatchignore for go runtime",
		},
		{
			name:         "rust runtime",
			runtime:      "rust",
			expectError:  false,
			expectOutput: "Created .hatchignore for rust runtime",
		},
		{
			name:         "php runtime",
			runtime:      "php",
			expectError:  false,
			expectOutput: "Created .hatchignore for php runtime",
		},
		{
			name:         "bun runtime",
			runtime:      "bun",
			expectError:  false,
			expectOutput: "Created .hatchignore for bun runtime",
		},
		{
			name:         "static runtime",
			runtime:      "static",
			expectError:  false,
			expectOutput: "Created .hatchignore for static runtime",
		},
		{
			name:         "invalid runtime",
			runtime:      "invalid",
			expectError:  true,
			expectOutput: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, cleanup := setupTestDir(t)
			defer cleanup()

			// Capture output using cobra buffers
			var outBuf bytes.Buffer
			var errBuf bytes.Buffer

			// Create and run command
			cmd := NewCmd()
			cmd.SetOut(&outBuf)
			cmd.SetErr(&errBuf)

			// Set runtime flag value directly on the command
			if err := cmd.Flags().Set("runtime", tt.runtime); err != nil {
				t.Fatal(err)
			}

			// Set the package-level variable as well (needed by runInitIgnore)
			oldRuntimeFlag := runtimeFlag
			runtimeFlag = tt.runtime
			defer func() { runtimeFlag = oldRuntimeFlag }()

			err := runInitIgnore(cmd, []string{})

			// Combine output for checking
			output := outBuf.String() + errBuf.String()

			// Check error expectation
			if tt.expectError && err == nil {
				t.Errorf("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			// Check output
			if tt.expectOutput != "" && !strings.Contains(output, tt.expectOutput) {
				t.Errorf("expected output to contain %q, got %q", tt.expectOutput, output)
			}

			// Check file was created
			if !tt.expectError {
				path := filepath.Join(dir, ".hatchignore")
				if _, err := os.Stat(path); err != nil {
					t.Errorf(".hatchignore file not created: %v", err)
				}
			}
		})
	}
}

func TestRunInitIgnore_FileAlreadyExists(t *testing.T) {
	dir, cleanup := setupTestDir(t)
	defer cleanup()

	// Create existing .hatchignore
	path := filepath.Join(dir, ".hatchignore")
	if err := os.WriteFile(path, []byte("existing content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Try to create again
	cmd := NewCmd()
	runtimeFlag = "node"
	defer func() { runtimeFlag = "" }()

	err := runInitIgnore(cmd, []string{})
	if err == nil {
		t.Error("expected error when file exists, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected 'already exists' error, got: %v", err)
	}
}

func TestRunInitIgnore_AutoDetectRuntime(t *testing.T) {
	tests := []struct {
		name        string
		files       map[string]string
		expectError bool
		expectMatch string
	}{
		{
			name:        "detect go",
			files:       map[string]string{"go.mod": "module test\n"},
			expectError: false,
			expectMatch: "go runtime",
		},
		{
			name:        "detect node",
			files:       map[string]string{"package.json": "{}\n"},
			expectError: false,
			expectMatch: "node runtime",
		},
		{
			name:        "no runtime detected",
			files:       map[string]string{},
			expectError: true,
			expectMatch: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, cleanup := setupTestDir(t)
			defer cleanup()

			// Create test files
			for name, content := range tt.files {
				path := filepath.Join(dir, name)
				if err := os.WriteFile(path, []byte(content), 0644); err != nil {
					t.Fatal(err)
				}
			}

			// Capture output
			var outBuf bytes.Buffer
			var errBuf bytes.Buffer

			// Don't set runtime flag - should auto-detect
			runtimeFlag = ""
			defer func() { runtimeFlag = "" }()

			cmd := NewCmd()
			cmd.SetOut(&outBuf)
			cmd.SetErr(&errBuf)

			err := runInitIgnore(cmd, []string{})

			if tt.expectError {
				if err == nil {
					t.Error("expected error when no runtime detected, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if tt.expectMatch != "" {
					output := outBuf.String() + errBuf.String()
					if !strings.Contains(output, tt.expectMatch) {
						t.Errorf("expected output to contain %q, got %q", tt.expectMatch, output)
					}
				}
			}
		})
	}
}

func TestTemplates_ContainExpectedContent(t *testing.T) {
	tests := []struct {
		runtime         string
		expectedContent []string
	}{
		{
			runtime: "node",
			expectedContent: []string{
				"node_modules/",
				".next/",
				"*.test.js",
				"tsconfig.json",
			},
		},
		{
			runtime: "python",
			expectedContent: []string{
				"__pycache__/",
				"*.pyc",
				".venv/",
			},
		},
		{
			runtime: "go",
			expectedContent: []string{
				"*.go",
				"*_test.go",
				"go.mod",
				"vendor/",
			},
		},
		{
			runtime: "rust",
			expectedContent: []string{
				"src/",
				"target/",
				"Cargo.toml",
			},
		},
		{
			runtime: "static",
			expectedContent: []string{
				"*.ts",
				"*.tsx",
				"package.json",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.runtime, func(t *testing.T) {
			tmpl, ok := templates[tt.runtime]
			if !ok {
				t.Fatalf("no template for runtime %q", tt.runtime)
			}

			for _, expected := range tt.expectedContent {
				if !strings.Contains(tmpl, expected) {
					t.Errorf("template for %q missing expected content %q", tt.runtime, expected)
				}
			}
		})
	}
}

func TestNewCmd_ReturnsValidCommand(t *testing.T) {
	cmd := NewCmd()

	if cmd == nil {
		t.Fatal("NewCmd() returned nil")
	}

	if cmd.Use != "init-ignore" {
		t.Errorf("expected Use 'init-ignore', got %q", cmd.Use)
	}

	if cmd.Short == "" {
		t.Error("Short description is empty")
	}

	if cmd.Long == "" {
		t.Error("Long description is empty")
	}

	// Check runtime flag exists
	flag := cmd.Flag("runtime")
	if flag == nil {
		t.Error("runtime flag not found")
	}
	if flag != nil && flag.Name != "runtime" {
		t.Errorf("expected 'runtime' flag, got %q", flag.Name)
	}
}

func TestNewCmd_IsCobraCommand(t *testing.T) {
	cmd := NewCmd()

	// Verify it's a valid cobra.Command by checking RunE is set
	if cmd.RunE == nil {
		t.Error("RunE function not set")
	}

	// Verify the command can be executed (will fail without proper setup, but should not panic)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	// Execute with invalid runtime should return error without panicking
	runtimeFlag = "invalid"
	defer func() { runtimeFlag = "" }()

	err := cmd.RunE(cmd, []string{})
	if err == nil {
		t.Error("expected error for invalid runtime")
	}
}
