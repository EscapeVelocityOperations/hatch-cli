package mcpserver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeToml(t *testing.T, dir, content string) string {
	t.Helper()
	p := filepath.Join(dir, ".hatch.toml")
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatalf("writing toml: %v", err)
	}
	return p
}

func TestResolveDeploySlug_ExplicitAppWins(t *testing.T) {
	target := t.TempDir()
	writeToml(t, target, "[app]\nslug = \"toml-slug-123\"\nname = \"toml-app\"\n")

	res, err := resolveDeploySlug("explicit-slug", "", target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Slug != "explicit-slug" {
		t.Errorf("expected explicit slug to win, got %q", res.Slug)
	}
	if !strings.Contains(res.Provenance, "app") || !strings.Contains(res.Provenance, "parameter") {
		t.Errorf("expected provenance to mention the app parameter, got %q", res.Provenance)
	}
}

func TestResolveDeploySlug_CwdTomlIgnored(t *testing.T) {
	// The incident case: MCP server cwd contains a .hatch.toml pinning an
	// unrelated app; deploy_target points elsewhere and has no toml.
	cwd := t.TempDir()
	writeToml(t, cwd, "[app]\nslug = \"jbi-whitepaper-7omkvw9p\"\nname = \"jbi-whitepaper\"\n")
	target := t.TempDir()

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	res, err := resolveDeploySlug("", "voice-showcase", target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Slug != "" {
		t.Errorf("cwd .hatch.toml must be ignored; expected empty slug (new app), got %q", res.Slug)
	}
}

func TestResolveDeploySlug_TomlInDeployTargetUsed(t *testing.T) {
	target := t.TempDir()
	tomlPath := writeToml(t, target, "[app]\nslug = \"target-slug-9\"\nname = \"target-app\"\n")

	res, err := resolveDeploySlug("", "", target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Slug != "target-slug-9" {
		t.Errorf("expected slug from deploy_target toml, got %q", res.Slug)
	}
	if !strings.Contains(res.Provenance, tomlPath) {
		t.Errorf("expected provenance to name the toml path %q, got %q", tomlPath, res.Provenance)
	}
}

func TestResolveDeploySlug_NameConflictWithTomlErrors(t *testing.T) {
	target := t.TempDir()
	writeToml(t, target, "[app]\nslug = \"target-slug-9\"\nname = \"target-app\"\n")

	_, err := resolveDeploySlug("", "different-name", target)
	if err == nil {
		t.Fatal("expected conflict error when name differs from toml app name")
	}
	if !strings.Contains(err.Error(), "app") {
		t.Errorf("expected error to guide toward explicit app parameter, got %q", err.Error())
	}
}

func TestResolveDeploySlug_NameMatchingTomlOK(t *testing.T) {
	target := t.TempDir()
	writeToml(t, target, "[app]\nslug = \"target-slug-9\"\nname = \"target-app\"\n")

	res, err := resolveDeploySlug("", "target-app", target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Slug != "target-slug-9" {
		t.Errorf("expected toml slug when name matches, got %q", res.Slug)
	}
}

func TestParseHatchTomlApp_SectionScoped(t *testing.T) {
	slug, name := parseHatchTomlApp([]byte("[database]\nslug = \"db-slug\"\n\n[app]\nslug = \"app-slug\"\nname = \"my-app\"\n"))
	if slug != "app-slug" {
		t.Errorf("expected slug from [app] section only, got %q", slug)
	}
	if name != "my-app" {
		t.Errorf("expected name from [app] section, got %q", name)
	}

	slug, name = parseHatchTomlApp([]byte("[database]\nslug = \"db-slug\"\n"))
	if slug != "" || name != "" {
		t.Errorf("expected no match outside [app] section, got slug=%q name=%q", slug, name)
	}
}

func TestWriteHatchToml_WritesToDir(t *testing.T) {
	target := t.TempDir()
	if err := writeHatchToml(target, "new-slug-1", "new-app"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(target, ".hatch.toml"))
	if err != nil {
		t.Fatalf("expected .hatch.toml in target dir: %v", err)
	}
	gotSlug, gotName := parseHatchTomlApp(data)
	if gotSlug != "new-slug-1" || gotName != "new-app" {
		t.Errorf("round-trip mismatch: slug=%q name=%q", gotSlug, gotName)
	}
}
