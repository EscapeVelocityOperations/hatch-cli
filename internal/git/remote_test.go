package git

import (
	"os"
	"os/exec"
	"testing"
)

func TestHasRemote(t *testing.T) {
	dir := setupTempGitRepo(t)
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(dir)

	if HasRemote("hatch") {
		t.Error("expected no 'hatch' remote in fresh repo")
	}

	exec.Command("git", "-C", dir, "remote", "add", "hatch", "https://example.com/test.git").Run()

	if !HasRemote("hatch") {
		t.Error("expected 'hatch' remote after adding it")
	}
}

func TestAddRemote(t *testing.T) {
	dir := setupTempGitRepo(t)
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(dir)

	if err := AddRemote("hatch", "https://example.com/test.git"); err != nil {
		t.Fatalf("AddRemote() error: %v", err)
	}

	if !HasRemote("hatch") {
		t.Error("expected 'hatch' remote after AddRemote")
	}
}

func TestSetRemoteURL(t *testing.T) {
	dir := setupTempGitRepo(t)
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(dir)

	AddRemote("hatch", "https://old.example.com/test.git")

	newURL := "https://new.example.com/test.git"
	if err := SetRemoteURL("hatch", newURL); err != nil {
		t.Fatalf("SetRemoteURL() error: %v", err)
	}

	got, err := GetRemoteURL("hatch")
	if err != nil {
		t.Fatalf("GetRemoteURL() error: %v", err)
	}
	if got != newURL {
		t.Errorf("expected URL %q, got %q", newURL, got)
	}
}

func TestGetRemoteURL(t *testing.T) {
	dir := setupTempGitRepo(t)
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(dir)

	url := "https://example.com/test.git"
	AddRemote("hatch", url)

	got, err := GetRemoteURL("hatch")
	if err != nil {
		t.Fatalf("GetRemoteURL() error: %v", err)
	}
	if got != url {
		t.Errorf("expected %q, got %q", url, got)
	}
}

func TestCurrentBranch(t *testing.T) {
	dir := setupTempGitRepo(t)
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(dir)

	// Need at least one commit to have a branch
	exec.Command("git", "-C", dir, "commit", "--allow-empty", "-m", "init").Run()

	branch, err := CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch() error: %v", err)
	}
	// Default branch could be main or master depending on git config
	if branch != "main" && branch != "master" {
		t.Errorf("expected 'main' or 'master', got %q", branch)
	}
}
