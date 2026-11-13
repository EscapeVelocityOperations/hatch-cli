package ignore

import (
	"bufio"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Matcher evaluates file paths against .hatchignore patterns.
type Matcher struct {
	patterns []pattern
	// envNegations lists .hatchignore lines that try to re-include an env file.
	envNegations []string
}

type pattern struct {
	negate    bool
	dirOnly   bool
	pathMatch bool   // pattern contains / — match against full relative path
	glob      string // cleaned glob for matching
}

// defaultPatterns are always applied and cannot be overridden by negation.
// Env files (.env, .env.*) are handled by IsSensitiveEnvName instead of a
// glob, because .env.example and .env.sample are kept (h-vma4h).
var defaultPatterns = []string{
	".git/",
	".DS_Store",
	".hatch.toml",
}

// envTemplateNames are .env.* files that hold no secrets and ship normally.
var envTemplateNames = map[string]bool{
	".env.example": true,
	".env.sample":  true,
}

// envProbeNames are representative sensitive names used to tell whether a
// glob that starts with ".env" (for example ".env*" or ".env.stag*") would
// re-include an env file.
var envProbeNames = []string{
	".env", ".env.local", ".env.production", ".env.staging", ".env.development", ".env.test",
}

// IsSensitiveEnvName reports whether a base name is an env file that may hold
// secrets: ".env" or ".env.<suffix>", except the .env.example / .env.sample
// templates. It never ships in an artifact, at any depth (h-vma4h: a real
// app/.env reached the registry).
func IsSensitiveEnvName(name string) bool {
	if envTemplateNames[name] {
		return false
	}
	return name == ".env" || strings.HasPrefix(name, ".env.")
}

// LoadFile parses a .hatchignore file and returns a Matcher.
// Built-in safety defaults are always prepended.
func LoadFile(path string) (*Matcher, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	m := DefaultMatcher()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		p := parsePattern(line)
		if p.negate && negatesEnvFile(p) {
			m.envNegations = append(m.envNegations, line)
		}
		m.patterns = append(m.patterns, p)
	}
	return m, scanner.Err()
}

// EnvNegations returns the .hatchignore lines that try to re-include an env
// file (for example "!.env" or "!app/.env.*"). Safety defaults cannot be
// negated, so callers refuse the deploy instead of ignoring the line silently.
func (m *Matcher) EnvNegations() []string {
	return m.envNegations
}

// negatesEnvFile reports whether a negated pattern explicitly targets a
// sensitive env file. Only the last path element is looked at (the safety
// default applies at every depth), and it must start with the literal ".env":
// generic globs such as "!*", "!*.local", "!public/*" or "!dist/**" could match
// an env file in theory, but they do not aim at one, so they are not reported
// (the default still keeps env files out). h-71gza: reporting them refused
// .hatchignore files that deployed fine before.
func negatesEnvFile(p pattern) bool {
	base := path.Base(p.glob)
	if !strings.HasPrefix(base, ".env") {
		return false
	}
	if !strings.ContainsAny(base, "*?[") {
		return IsSensitiveEnvName(base)
	}
	for _, probe := range envProbeNames {
		if matched, _ := filepath.Match(base, probe); matched {
			return true
		}
	}
	return false
}

// DefaultMatcher returns a Matcher with only built-in safety defaults.
// Used when no .hatchignore file exists.
func DefaultMatcher() *Matcher {
	m := &Matcher{}
	for _, raw := range defaultPatterns {
		m.patterns = append(m.patterns, parsePattern(raw))
	}
	return m
}

func parsePattern(raw string) pattern {
	p := pattern{}
	s := raw

	if strings.HasPrefix(s, "!") {
		p.negate = true
		s = s[1:]
	}
	if strings.HasSuffix(s, "/") {
		p.dirOnly = true
		s = strings.TrimSuffix(s, "/")
	}
	// If pattern contains a slash, match against full relative path
	if strings.Contains(s, "/") {
		p.pathMatch = true
	}
	p.glob = s
	return p
}

// ShouldExclude returns true if the given relative path should be excluded.
// Safety defaults (.git, .env, .env.*, etc.) always apply and cannot be negated.
func (m *Matcher) ShouldExclude(rel string, isDir bool) bool {
	// Safety defaults are enforced unconditionally
	name := filepath.Base(rel)
	if IsSensitiveEnvName(name) {
		return true
	}
	for _, raw := range defaultPatterns {
		p := parsePattern(raw)
		if p.dirOnly && !isDir {
			continue
		}
		if matched, _ := filepath.Match(p.glob, name); matched {
			return true
		}
	}

	// Evaluate user patterns — last matching pattern wins
	excluded := false
	for _, p := range m.patterns {
		if p.dirOnly && !isDir {
			continue
		}
		matched := false
		if p.pathMatch {
			matched, _ = filepath.Match(p.glob, rel)
		} else {
			matched, _ = filepath.Match(p.glob, name)
		}
		if matched {
			excluded = !p.negate
		}
	}
	return excluded
}
