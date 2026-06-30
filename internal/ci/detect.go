// Package ci powers the `hatch ci` assist command: it detects the CI provider
// and project runtime, then explains/generates the CI wiring (it never pushes —
// see the assist boundary on the umbrella bead).
package ci

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DetectProvider classifies a git remote URL into a CI provider. github.com →
// "github"; any GitLab host (gitlab.com or a self-hosted gitlab.*) → "gitlab";
// anything else (or empty) → "unknown". Works for both ssh (git@host:owner/repo)
// and https (https://host/owner/repo) remote forms since it matches on host
// substrings.
func DetectProvider(remote string) string {
	r := strings.ToLower(remote)
	switch {
	case strings.Contains(r, "github.com"):
		return "github"
	case strings.Contains(r, "gitlab"): // gitlab.com or self-hosted gitlab.<domain>
		return "gitlab"
	default:
		return "unknown"
	}
}

// runtimeSignatures maps a project signature file to the hatch base runtime, in
// detection priority order. bun.lockb precedes package.json because a bun project
// carries both; the first match wins.
var runtimeSignatures = []struct{ file, runtime string }{
	{"bun.lockb", "bun"},
	{"package.json", "node"},
	{"go.mod", "go"},
	{"requirements.txt", "python"},
	{"pyproject.toml", "python"},
	{"Cargo.toml", "rust"},
	{"composer.json", "php"},
}

var hatchTomlRuntime = regexp.MustCompile(`(?m)^\s*runtime\s*=\s*"([^"]+)"`)

// DetectRuntime infers the hatch base runtime (node, go, python, rust, php, bun,
// or static) for a project directory. An existing .hatch.toml that declares a
// runtime takes precedence; otherwise the first matching signature file decides;
// a directory with no recognized signature is treated as static.
func DetectRuntime(dir string) string {
	if rt := runtimeFromHatchToml(dir); rt != "" {
		return rt
	}
	for _, s := range runtimeSignatures {
		if fileExists(filepath.Join(dir, s.file)) {
			return s.runtime
		}
	}
	return "static"
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// runtimeFromHatchToml returns the runtime declared in dir/.hatch.toml, or "" if
// the file is absent or declares none. A minimal line match avoids pulling in a
// TOML parser just to read one optional field.
func runtimeFromHatchToml(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, ".hatch.toml"))
	if err != nil {
		return ""
	}
	if m := hatchTomlRuntime.FindSubmatch(data); m != nil {
		return string(m[1])
	}
	return ""
}
