package initcmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/mcpserver"
)

func setupTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		os.Chdir(oldWd)
		// Reset flags
		claudeMDOnly = false
		mcpOnly = false
		force = false
	})

	return dir
}

func TestRemoveHatchSection(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no hatch section",
			input:    "# Project\n\nSome content\n",
			expected: "# Project\n\nSome content\n",
		},
		{
			name: "hatch section at end",
			input: `# Project

Some content

## Hatch Deployment

Hatch deployment instructions...

`,
			expected: `# Project

Some content
`,
		},
		{
			name: "hatch section in middle",
			input: `# Project

Some content

## Hatch Deployment

Hatch deployment instructions...

## Other Section

Other content
`,
			expected: `# Project

Some content

## Other Section

Other content
`,
		},
		{
			name: "hatch section followed by heading without space",
			input: `## Hatch Deployment
Content here
## Next Section
More content`,
			expected: `## Next Section
More content`,
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := removeHatchSection(tt.input)
			if got != tt.expected {
				t.Errorf("removeHatchSection() =\n%q\nwant\n%q", got, tt.expected)
			}
		})
	}
}

func TestWriteClaudeMD_CreatesNewFile(t *testing.T) {
	_ = setupTestDir(t)

	wrote, err := writeClaudeMD()
	if err != nil {
		t.Fatal(err)
	}

	if !wrote {
		t.Error("expected wrote=true, got false")
	}

	// Check file exists
	content, err := os.ReadFile("CLAUDE.md")
	if err != nil {
		t.Fatal(err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, hatchSectionMarker) {
		t.Error("CLAUDE.md does not contain hatch section marker")
	}

	if !strings.Contains(contentStr, mcpserver.ClaudeMDContent) {
		t.Error("CLAUDE.md does not contain expected content")
	}
}

func TestWriteClaudeMD_AppendsToExistingFile(t *testing.T) {
	_ = setupTestDir(t)

	// Create existing CLAUDE.md
	existingContent := `# My Project

This is my project.
`
	if err := os.WriteFile("CLAUDE.md", []byte(existingContent), 0644); err != nil {
		t.Fatal(err)
	}

	wrote, err := writeClaudeMD()
	if err != nil {
		t.Fatal(err)
	}

	if !wrote {
		t.Error("expected wrote=true, got false")
	}

	// Check file contains both original and new content
	content, err := os.ReadFile("CLAUDE.md")
	if err != nil {
		t.Fatal(err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "My Project") {
		t.Error("CLAUDE.md lost original content")
	}

	if !strings.Contains(contentStr, hatchSectionMarker) {
		t.Error("CLAUDE.md does not contain hatch section marker")
	}
}

func TestWriteClaudeMD_AlreadyExistsWithoutForce(t *testing.T) {
	_ = setupTestDir(t)

	// Create CLAUDE.md with hatch section
	content := `# My Project

` + mcpserver.ClaudeMDContent
	if err := os.WriteFile("CLAUDE.md", []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	wrote, err := writeClaudeMD()
	if err != nil {
		t.Fatal(err)
	}

	if wrote {
		t.Error("expected wrote=false when hatch section exists, got true")
	}
}

func TestWriteClaudeMD_OverwritesWithForce(t *testing.T) {
	_ = setupTestDir(t)

	// Create CLAUDE.md with hatch section
	content := `# My Project

## Hatch Deployment

Old hatch content

## Other Section

Other content
`
	if err := os.WriteFile("CLAUDE.md", []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	force = true

	wrote, err := writeClaudeMD()
	if err != nil {
		t.Fatal(err)
	}

	if !wrote {
		t.Error("expected wrote=true with force flag, got false")
	}

	// Check old section was removed
	newContent, err := os.ReadFile("CLAUDE.md")
	if err != nil {
		t.Fatal(err)
	}

	contentStr := string(newContent)
	// Should not contain "Old hatch content"
	if strings.Contains(contentStr, "Old hatch content") {
		t.Error("force flag did not remove old hatch section")
	}

	// Should still have other section
	if !strings.Contains(contentStr, "Other Section") {
		t.Error("force flag removed other sections")
	}
}

func TestWriteMCPConfig_CreatesNewConfig(t *testing.T) {
	dir := setupTestDir(t)

	wrote, err := writeMCPConfig()
	if err != nil {
		t.Fatal(err)
	}

	if !wrote {
		t.Error("expected wrote=true, got false")
	}

	// Check settings.json exists
	settingsPath := filepath.Join(dir, ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}

	mcpServers, ok := settings["mcpServers"].(map[string]any)
	if !ok {
		t.Fatal("mcpServers not found or not a map")
	}

	hatch, ok := mcpServers["hatch"].(map[string]any)
	if !ok {
		t.Fatal("hatch mcp server not found")
	}

	if hatch["command"] != "hatch" {
		t.Errorf("expected command 'hatch', got %v", hatch["command"])
	}
}

func TestWriteMCPConfig_AppendsToExisting(t *testing.T) {
	dir := setupTestDir(t)

	// Create existing settings.json
	settingsDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatal(err)
	}

	existingSettings := map[string]any{
		"mcpServers": map[string]any{
			"other": map[string]any{
				"command": "other-cmd",
			},
		},
	}

	data, _ := json.MarshalIndent(existingSettings, "", "  ")
	settingsPath := filepath.Join(settingsDir, "settings.json")
	if err := os.WriteFile(settingsPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	wrote, err := writeMCPConfig()
	if err != nil {
		t.Fatal(err)
	}

	if !wrote {
		t.Error("expected wrote=true, got false")
	}

	// Check both servers exist
	data, err = os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}

	mcpServers, ok := settings["mcpServers"].(map[string]any)
	if !ok {
		t.Fatal("mcpServers not found")
	}

	if _, ok := mcpServers["other"]; !ok {
		t.Error("lost existing mcp server 'other'")
	}

	if _, ok := mcpServers["hatch"]; !ok {
		t.Error("hatch mcp server not added")
	}
}

func TestWriteMCPConfig_AlreadyExists(t *testing.T) {
	dir := setupTestDir(t)

	// Create existing settings with hatch already configured
	settingsDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatal(err)
	}

	existingSettings := map[string]any{
		"mcpServers": map[string]any{
			"hatch": map[string]any{
				"command": "hatch",
			},
		},
	}

	data, _ := json.MarshalIndent(existingSettings, "", "  ")
	settingsPath := filepath.Join(settingsDir, "settings.json")
	if err := os.WriteFile(settingsPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	wrote, err := writeMCPConfig()
	if err != nil {
		t.Fatal(err)
	}

	if wrote {
		t.Error("expected wrote=false when hatch already exists, got true")
	}
}

func TestWriteMCPConfig_InvalidJSON(t *testing.T) {
	dir := setupTestDir(t)

	// Create invalid settings.json
	settingsDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatal(err)
	}

	settingsPath := filepath.Join(settingsDir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte("invalid json"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := writeMCPConfig()
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestNewCmd_ReturnsValidCommand(t *testing.T) {
	cmd := NewCmd()

	if cmd == nil {
		t.Fatal("NewCmd() returned nil")
	}

	if cmd.Use != "init" {
		t.Errorf("expected Use 'init', got %q", cmd.Use)
	}

	if cmd.Short == "" {
		t.Error("Short description is empty")
	}

	if cmd.Long == "" {
		t.Error("Long description is empty")
	}

	// Check flags
	flag := cmd.Flag("claude-md-only")
	if flag == nil {
		t.Error("claude-md-only flag not found")
	}

	flag = cmd.Flag("mcp-only")
	if flag == nil {
		t.Error("mcp-only flag not found")
	}

	flag = cmd.Flag("force")
	if flag == nil {
		t.Error("force flag not found")
	}
}

func TestWriteClaudeMD_AddsNewlineWhenNeeded(t *testing.T) {
	tests := []struct {
		name     string
		existing string
	}{
		{
			name:     "file without trailing newline",
			existing: "# Project",
		},
		{
			name:     "file with single trailing newline",
			existing: "# Project\n",
		},
		{
			name:     "file with double trailing newline",
			existing: "# Project\n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = setupTestDir(t)

			if err := os.WriteFile("CLAUDE.md", []byte(tt.existing), 0644); err != nil {
				t.Fatal(err)
			}

			wrote, err := writeClaudeMD()
			if err != nil {
				t.Fatal(err)
			}

			if !wrote {
				t.Error("expected wrote=true")
			}

			// Just verify no error and file was written
			content, err := os.ReadFile("CLAUDE.md")
			if err != nil {
				t.Fatal(err)
			}

			if !strings.Contains(string(content), hatchSectionMarker) {
				t.Error("hatch section not added")
			}
		})
	}
}
