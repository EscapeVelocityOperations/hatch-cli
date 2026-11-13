package ignore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultMatcher_ExcludesGit(t *testing.T) {
	m := DefaultMatcher()
	if !m.ShouldExclude(".git", true) {
		t.Error("expected .git/ to be excluded")
	}
}

func TestDefaultMatcher_ExcludesEnv(t *testing.T) {
	m := DefaultMatcher()
	if !m.ShouldExclude(".env", false) {
		t.Error("expected .env to be excluded")
	}
}

func TestDefaultMatcher_ExcludesEnvLocal(t *testing.T) {
	m := DefaultMatcher()
	if !m.ShouldExclude(".env.local", false) {
		t.Error("expected .env.local to be excluded")
	}
}

func TestDefaultMatcher_ExcludesEnvProduction(t *testing.T) {
	m := DefaultMatcher()
	if !m.ShouldExclude(".env.production", false) {
		t.Error("expected .env.production to be excluded")
	}
}

func TestDefaultMatcher_ExcludesDSStore(t *testing.T) {
	m := DefaultMatcher()
	if !m.ShouldExclude(".DS_Store", false) {
		t.Error("expected .DS_Store to be excluded")
	}
}

func TestDefaultMatcher_ExcludesHatchToml(t *testing.T) {
	m := DefaultMatcher()
	if !m.ShouldExclude(".hatch.toml", false) {
		t.Error("expected .hatch.toml to be excluded")
	}
}

func TestDefaultMatcher_AllowsRegularFiles(t *testing.T) {
	m := DefaultMatcher()
	if m.ShouldExclude("server.js", false) {
		t.Error("expected server.js to be included")
	}
}

func TestDefaultMatcher_AllowsNodeModules(t *testing.T) {
	m := DefaultMatcher()
	if m.ShouldExclude("node_modules", true) {
		t.Error("expected node_modules to be included by default (no blanket dotfile exclusion)")
	}
}

func TestDefaultMatcher_AllowsNitroDir(t *testing.T) {
	m := DefaultMatcher()
	// .nitro inside node_modules must NOT be excluded (regression test)
	if m.ShouldExclude("node_modules/.nitro", true) {
		t.Error("expected node_modules/.nitro to be included (regression: was excluded by blanket dotfile rule)")
	}
}

func TestDefaultMatcher_GitDirOnlyMatchesDirs(t *testing.T) {
	m := DefaultMatcher()
	// .git as a file (e.g. submodule pointer) should still be excluded by the safety default
	// because the default ".git/" is dirOnly but we enforce safety defaults on basename
	// The safety check matches .git regardless — but the default pattern is dirOnly
	// Let's verify: .git as a non-dir should NOT match the ".git/" pattern
	// However, our safety defaults enforce unconditionally for the basename
	// Actually, the safety default ".git/" has dirOnly=true, so it skips non-dirs
	// But .git files in submodules should be fine to exclude too
	// For now, .git/ only matches directories — a .git file would pass through
	if m.ShouldExclude(".git", false) {
		// .git as a file is NOT excluded because the pattern is dirOnly
		t.Error("expected .git file to pass through (dirOnly pattern)")
	}
}

func TestLoadFile_ParsesPatterns(t *testing.T) {
	tmp := t.TempDir()
	content := "# comment\nnode_modules/\n*.log\n\n!important.log\n"
	os.WriteFile(filepath.Join(tmp, ".hatchignore"), []byte(content), 0644)

	m, err := LoadFile(filepath.Join(tmp, ".hatchignore"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// node_modules/ should be excluded (dirOnly)
	if !m.ShouldExclude("node_modules", true) {
		t.Error("expected node_modules/ to be excluded")
	}
	// node_modules as file should not match dirOnly pattern
	if m.ShouldExclude("node_modules", false) {
		t.Error("expected node_modules (file) to not match dirOnly pattern")
	}
	// *.log should be excluded
	if !m.ShouldExclude("app.log", false) {
		t.Error("expected app.log to be excluded")
	}
	// !important.log negates
	if m.ShouldExclude("important.log", false) {
		t.Error("expected important.log to be included (negated)")
	}
}

func TestLoadFile_NotFound(t *testing.T) {
	_, err := LoadFile("/nonexistent/.hatchignore")
	if err == nil {
		t.Error("expected error for missing file")
	}
	if !os.IsNotExist(err) {
		t.Errorf("expected os.IsNotExist error, got: %v", err)
	}
}

func TestShouldExclude_NestedPath(t *testing.T) {
	tmp := t.TempDir()
	content := "*.log\n"
	os.WriteFile(filepath.Join(tmp, ".hatchignore"), []byte(content), 0644)

	m, err := LoadFile(filepath.Join(tmp, ".hatchignore"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// *.log should match files at any depth (basename match)
	if !m.ShouldExclude("logs/app.log", false) {
		t.Error("expected logs/app.log to be excluded (basename match)")
	}
	if !m.ShouldExclude("deep/nested/error.log", false) {
		t.Error("expected deep/nested/error.log to be excluded (basename match)")
	}
}

func TestShouldExclude_PathPattern(t *testing.T) {
	tmp := t.TempDir()
	content := "src/test/\n"
	os.WriteFile(filepath.Join(tmp, ".hatchignore"), []byte(content), 0644)

	m, err := LoadFile(filepath.Join(tmp, ".hatchignore"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// src/test should match as a path pattern
	if !m.ShouldExclude("src/test", true) {
		t.Error("expected src/test/ to be excluded")
	}
	// test/ at root should NOT match (path pattern requires src/test)
	if m.ShouldExclude("test", true) {
		t.Error("expected test/ at root to not match src/test/ pattern")
	}
}

func TestShouldExclude_SafetyDefaultsCannotBeNegated(t *testing.T) {
	tmp := t.TempDir()
	// Try to force-include .env via negation
	content := "!.env\n!.git/\n"
	os.WriteFile(filepath.Join(tmp, ".hatchignore"), []byte(content), 0644)

	m, err := LoadFile(filepath.Join(tmp, ".hatchignore"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Safety defaults must still apply
	if !m.ShouldExclude(".env", false) {
		t.Error("expected .env to remain excluded (safety default cannot be negated)")
	}
	if !m.ShouldExclude(".git", true) {
		t.Error("expected .git/ to remain excluded (safety default cannot be negated)")
	}
}

func TestShouldExclude_WildcardExtension(t *testing.T) {
	tmp := t.TempDir()
	content := "*.test.js\n*.spec.ts\n"
	os.WriteFile(filepath.Join(tmp, ".hatchignore"), []byte(content), 0644)

	m, err := LoadFile(filepath.Join(tmp, ".hatchignore"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !m.ShouldExclude("auth.test.js", false) {
		t.Error("expected auth.test.js to be excluded")
	}
	if !m.ShouldExclude("utils.spec.ts", false) {
		t.Error("expected utils.spec.ts to be excluded")
	}
	if m.ShouldExclude("auth.js", false) {
		t.Error("expected auth.js to be included")
	}
}

func TestShouldExclude_LastPatternWins(t *testing.T) {
	tmp := t.TempDir()
	// Exclude all logs, then re-include, then exclude again
	content := "*.log\n!*.log\n*.log\n"
	os.WriteFile(filepath.Join(tmp, ".hatchignore"), []byte(content), 0644)

	m, err := LoadFile(filepath.Join(tmp, ".hatchignore"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Last pattern is *.log (exclude), so it should be excluded
	if !m.ShouldExclude("app.log", false) {
		t.Error("expected app.log to be excluded (last pattern wins)")
	}
}

func TestDefaultMatcher_ExcludesNestedEnvFiles(t *testing.T) {
	m := DefaultMatcher()
	for _, rel := range []string{
		"app/.env",
		"app/.env.local",
		"a/b/c/.env.production",
		"backend/.env.staging",
		".env.prod",
	} {
		if !m.ShouldExclude(rel, false) {
			t.Errorf("expected %s to be excluded", rel)
		}
	}
}

func TestDefaultMatcher_AllowsEnvTemplates(t *testing.T) {
	m := DefaultMatcher()
	for _, rel := range []string{
		".env.example",
		".env.sample",
		"app/.env.example",
		"app/.env.sample",
	} {
		if m.ShouldExclude(rel, false) {
			t.Errorf("expected %s to be kept: it is a template without secrets", rel)
		}
	}
}

func TestDefaultMatcher_EnvDirIsNotAnEnvFile(t *testing.T) {
	// A directory called ".env" (Python virtualenv convention) is not a
	// secrets file but must stay out of the artifact too: it holds a whole
	// interpreter tree, and leaving it in leaks the same secrets via
	// activate scripts. Exclusion applies to dirs as well as files.
	m := DefaultMatcher()
	if !m.ShouldExclude(".env", true) {
		t.Error("expected .env directory to be excluded")
	}
}

func TestEnvNegations_ReportsReincludedEnvPatterns(t *testing.T) {
	tmp := t.TempDir()
	content := "# comment\n!.env\n!app/.env.production\n!.env.*\n!.env*\n!dist/\n!.env.example\n!keep.js\n"
	path := filepath.Join(tmp, ".hatchignore")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	m, err := LoadFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := m.EnvNegations()
	want := []string{"!.env", "!app/.env.production", "!.env.*", "!.env*"}
	if len(got) != len(want) {
		t.Fatalf("EnvNegations() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("EnvNegations()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestEnvNegations_NoneForDefaultMatcher(t *testing.T) {
	if got := DefaultMatcher().EnvNegations(); len(got) != 0 {
		t.Errorf("EnvNegations() = %v, want none", got)
	}
}

func TestIsSensitiveEnvName(t *testing.T) {
	for name, want := range map[string]bool{
		".env":            true,
		".env.local":      true,
		".env.production": true,
		".env.example":    false,
		".env.sample":     false,
		".envrc":          false,
		"env":             false,
		"app.env":         false,
		"server.js":       false,
	} {
		if got := IsSensitiveEnvName(name); got != want {
			t.Errorf("IsSensitiveEnvName(%q) = %v, want %v", name, got, want)
		}
	}
}

// h-71gza F2: only a pattern that explicitly targets an env file counts as a
// re-include. Globs that merely could match one (!*, !*.local, !dist/**) are
// ordinary negations: the safety default keeps env files out silently.
func TestEnvNegations_NoFalsePositivesOnGenericGlobs(t *testing.T) {
	for _, line := range []string{
		"!public/*",
		"!dist/**",
		"!*",
		"!*.local",
		"!static/assets/*",
		"!**/*",
		"!app/",
		"!.envrc",
		"!.env.example",
		"!.env.sample",
		"!config/.env.example",
		"!environment.js",
	} {
		tmp := t.TempDir()
		path := filepath.Join(tmp, ".hatchignore")
		if err := os.WriteFile(path, []byte(line+"\n"), 0644); err != nil {
			t.Fatal(err)
		}
		m, err := LoadFile(path)
		if err != nil {
			t.Fatalf("%q: unexpected error: %v", line, err)
		}
		if got := m.EnvNegations(); len(got) != 0 {
			t.Errorf("EnvNegations() for %q = %v, want none", line, got)
		}
	}
}

func TestEnvNegations_ReportsExplicitEnvTargets(t *testing.T) {
	for _, line := range []string{
		"!.env",
		"!.env.local",
		"!.env.production",
		"!.env.staging",
		"!.env.*",
		"!.env*",
		"!app/.env",
		"!app/.env.*",
		"!**/.env",
		"!**/.env.local",
		"!a/b/.env.staging",
	} {
		tmp := t.TempDir()
		path := filepath.Join(tmp, ".hatchignore")
		if err := os.WriteFile(path, []byte(line+"\n"), 0644); err != nil {
			t.Fatal(err)
		}
		m, err := LoadFile(path)
		if err != nil {
			t.Fatalf("%q: unexpected error: %v", line, err)
		}
		if got := m.EnvNegations(); len(got) != 1 || got[0] != line {
			t.Errorf("EnvNegations() for %q = %v, want [%q]", line, got, line)
		}
	}
}

// A generic negation must not re-include env files: the default still wins.
func TestShouldExclude_GenericNegationDoesNotReincludeEnv(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, ".hatchignore")
	if err := os.WriteFile(path, []byte("!*\n!dist/**\n"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{".env", "app/.env", "dist/.env.local"} {
		if !m.ShouldExclude(rel, false) {
			t.Errorf("ShouldExclude(%q) = false, want true", rel)
		}
	}
}
