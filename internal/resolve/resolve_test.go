package resolve

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSlugFromToml(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "flat format with slug",
			content:  `slug = "my-app"`,
			expected: "my-app",
		},
		{
			name:     "app section format with slug",
			content:  `[app]
slug = "my-app"`,
			expected: "my-app",
		},
		{
			name:     "app section format with slug and other fields",
			content:  `[app]
slug = "my-app"
name = "My App"`,
			expected: "my-app",
		},
		{
			name:     "flat format with empty slug",
			content:  `slug = ""`,
			expected: "",
		},
		{
			name:     "app section format with empty slug",
			content:  `[app]
slug = ""`,
			expected: "",
		},
		{
			name:     "no slug in flat format",
			content:  `name = "My App"`,
			expected: "",
		},
		{
			name:     "no slug in app section",
			content:  `[app]
name = "My App"`,
			expected: "",
		},
		{
			name:     "empty file",
			content:  ``,
			expected: "",
		},
		{
			name:     "flat format with quoted slug containing special chars",
			content:  `slug = "my-app-123"`,
			expected: "my-app-123",
		},
		{
			name:     "app section format with quoted slug containing special chars",
			content:  `[app]
slug = "my-app-456"`,
			expected: "my-app-456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory
			dir := t.TempDir()
			configPath := filepath.Join(dir, ".hatch.toml")

			// Write test content
			if err := os.WriteFile(configPath, []byte(tt.content), 0644); err != nil {
				t.Fatal(err)
			}

			// Change to temp directory
			oldWd, _ := os.Getwd()
			if err := os.Chdir(dir); err != nil {
				t.Fatal(err)
			}
			defer os.Chdir(oldWd)

			// Test SlugFromToml
			got := SlugFromToml()
			if got != tt.expected {
				t.Errorf("SlugFromToml() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestSlugFromToml_NoFile(t *testing.T) {
	// Create temp directory without .hatch.toml
	dir := t.TempDir()

	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	got := SlugFromToml()
	if got != "" {
		t.Errorf("SlugFromToml() with no file = %q, want empty string", got)
	}
}

func TestSlugFromToml_PrefersFlatFormat(t *testing.T) {
	// When both formats are present, flat format should take precedence
	content := `slug = "flat-slug"
[app]
slug = "app-slug"`

	dir := t.TempDir()
	configPath := filepath.Join(dir, ".hatch.toml")

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	got := SlugFromToml()
	if got != "flat-slug" {
		t.Errorf("SlugFromToml() with both formats = %q, want 'flat-slug'", got)
	}
}

func TestSlugFromToml_InvalidToml(t *testing.T) {
	// Invalid TOML should return empty string
	content := `slug = "my-app`
	// Missing closing quote

	dir := t.TempDir()
	configPath := filepath.Join(dir, ".hatch.toml")

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	got := SlugFromToml()
	if got != "" {
		t.Errorf("SlugFromToml() with invalid TOML = %q, want empty string", got)
	}
}
