package ci

import (
	"os"
	"path/filepath"
	"testing"
)

// h-tymh: runtime detection by project signature file. An existing .hatch.toml
// runtime wins; otherwise the first matching signature file decides, with
// bun.lockb taking precedence over package.json (a bun project has both); no
// match → static.
func TestDetectRuntime(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string // filename → contents
		want  string
	}{
		{"node", map[string]string{"package.json": "{}"}, "node"},
		{"go", map[string]string{"go.mod": "module x"}, "go"},
		{"python requirements", map[string]string{"requirements.txt": ""}, "python"},
		{"python pyproject", map[string]string{"pyproject.toml": ""}, "python"},
		{"rust", map[string]string{"Cargo.toml": ""}, "rust"},
		{"php", map[string]string{"composer.json": "{}"}, "php"},
		{"bun", map[string]string{"bun.lockb": ""}, "bun"},
		{"bun beats node", map[string]string{"bun.lockb": "", "package.json": "{}"}, "bun"},
		{"empty → static", map[string]string{}, "static"},
		{"hatch.toml runtime wins", map[string]string{".hatch.toml": "runtime = \"python\"\n", "package.json": "{}"}, "python"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, content := range tc.files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
					t.Fatalf("write %s: %v", name, err)
				}
			}
			if got := DetectRuntime(dir); got != tc.want {
				t.Errorf("DetectRuntime(%v) = %q, want %q", tc.files, got, tc.want)
			}
		})
	}
}
