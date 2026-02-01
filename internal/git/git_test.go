package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func setupTempGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	// Configure git user for commits
	for _, args := range [][]string{
		{"config", "user.email", "test@test.com"},
		{"config", "user.name", "Test"},
	} {
		c := exec.Command("git", args...)
		c.Dir = dir
		c.Run()
	}
	return dir
}

func TestIsGitRepo(t *testing.T) {
	dir := setupTempGitRepo(t)
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(dir)

	if !IsGitRepo() {
		t.Error("expected IsGitRepo() to return true in git repo")
	}

	// Test in non-git directory
	nonGit := t.TempDir()
	os.Chdir(nonGit)
	if IsGitRepo() {
		t.Error("expected IsGitRepo() to return false outside git repo")
	}
}

func TestInit(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(dir)

	if err := Init(); err != nil {
		t.Fatalf("Init() failed: %v", err)
	}

	if !IsGitRepo() {
		t.Error("expected IsGitRepo() to return true after Init()")
	}
}

func TestHasChanges(t *testing.T) {
	dir := setupTempGitRepo(t)
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(dir)

	// Empty repo - create initial commit
	os.WriteFile(filepath.Join(dir, "init.txt"), []byte("init"), 0644)
	exec.Command("git", "-C", dir, "add", "-A").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "init").Run()

	// Clean state
	has, err := HasChanges()
	if err != nil {
		t.Fatalf("HasChanges() error: %v", err)
	}
	if has {
		t.Error("expected no changes in clean repo")
	}

	// Add a file
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello"), 0644)
	has, err = HasChanges()
	if err != nil {
		t.Fatalf("HasChanges() error: %v", err)
	}
	if !has {
		t.Error("expected changes after adding file")
	}
}

func TestCommitAll(t *testing.T) {
	dir := setupTempGitRepo(t)
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(dir)

	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello"), 0644)

	if err := CommitAll("test commit"); err != nil {
		t.Fatalf("CommitAll() error: %v", err)
	}

	has, _ := HasChanges()
	if has {
		t.Error("expected no changes after CommitAll")
	}
}
