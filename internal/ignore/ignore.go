package ignore

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// Matcher evaluates file paths against .hatchignore patterns.
type Matcher struct {
	patterns []pattern
}

type pattern struct {
	negate    bool
	dirOnly   bool
	pathMatch bool   // pattern contains / — match against full relative path
	glob      string // cleaned glob for matching
}

// defaultPatterns are always applied and cannot be overridden by negation.
var defaultPatterns = []string{
	".git/",
	".env",
	".env.*",
	".DS_Store",
	".hatch.toml",
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
		m.patterns = append(m.patterns, parsePattern(line))
	}
	return m, scanner.Err()
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
// Safety defaults (.git, .env, etc.) always apply and cannot be negated.
func (m *Matcher) ShouldExclude(rel string, isDir bool) bool {
	// Safety defaults are enforced unconditionally
	name := filepath.Base(rel)
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
