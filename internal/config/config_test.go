package config

import (
	"os"
	"path/filepath"
	"testing"
)

func setupTestHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	return dir
}

func TestDirReturnsHatchDir(t *testing.T) {
	home := setupTestHome(t)
	d, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".hatch")
	if d != want {
		t.Errorf("Dir() = %q, want %q", d, want)
	}
}

func TestPathReturnsConfigFile(t *testing.T) {
	home := setupTestHome(t)
	p, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".hatch", "config.json")
	if p != want {
		t.Errorf("Path() = %q, want %q", p, want)
	}
}

func TestLoadReturnsEmptyWhenNoFile(t *testing.T) {
	setupTestHome(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Token != "" {
		t.Errorf("expected empty token, got %q", cfg.Token)
	}
}

func TestSaveAndLoad(t *testing.T) {
	setupTestHome(t)
	want := &Config{Token: "test-token-123"}
	if err := Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Token != want.Token {
		t.Errorf("Token = %q, want %q", got.Token, want.Token)
	}
}

func TestClearToken(t *testing.T) {
	setupTestHome(t)
	if err := Save(&Config{Token: "to-clear"}); err != nil {
		t.Fatal(err)
	}
	if err := ClearToken(); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Token != "" {
		t.Errorf("expected empty token after clear, got %q", cfg.Token)
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	home := setupTestHome(t)
	dir := filepath.Join(home, ".hatch")
	os.MkdirAll(dir, 0700)
	os.WriteFile(filepath.Join(dir, "config.json"), []byte("{invalid"), 0600)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestSaveCreatesDirectory(t *testing.T) {
	setupTestHome(t)
	cfg := &Config{Token: "new"}
	if err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	d, _ := Dir()
	info, err := os.Stat(d)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Error("expected directory to be created")
	}
}

func TestSaveFilePermissions(t *testing.T) {
	setupTestHome(t)
	if err := Save(&Config{Token: "secret"}); err != nil {
		t.Fatal(err)
	}
	p, _ := Path()
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("file perm = %o, want 0600", perm)
	}
}

func TestSaveOverwrite(t *testing.T) {
	setupTestHome(t)
	Save(&Config{Token: "first"})
	Save(&Config{Token: "second"})
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Token != "second" {
		t.Errorf("Token = %q, want %q", cfg.Token, "second")
	}
}

func TestDirPermissions(t *testing.T) {
	setupTestHome(t)
	Save(&Config{Token: "x"})
	d, _ := Dir()
	info, err := os.Stat(d)
	if err != nil {
		t.Fatal(err)
	}
	perm := info.Mode().Perm()
	if perm != 0700 {
		t.Errorf("dir perm = %o, want 0700", perm)
	}
}

func TestLoadEmptyFile(t *testing.T) {
	home := setupTestHome(t)
	dir := filepath.Join(home, ".hatch")
	os.MkdirAll(dir, 0700)
	os.WriteFile(filepath.Join(dir, "config.json"), []byte("{}"), 0600)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Token != "" {
		t.Errorf("expected empty token, got %q", cfg.Token)
	}
}
