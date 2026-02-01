package git

import (
	"os/exec"
	"strings"
)

// IsGitRepo checks if the current directory is inside a git repository.
func IsGitRepo() bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}

// Init initializes a new git repository in the current directory.
func Init() error {
	return exec.Command("git", "init").Run()
}

// HasChanges returns true if there are uncommitted changes (staged or unstaged)
// or untracked files.
func HasChanges() (bool, error) {
	// Check for staged/unstaged changes
	cmd := exec.Command("git", "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) != "", nil
}

// CommitAll stages all changes and commits with the given message.
func CommitAll(message string) error {
	if err := exec.Command("git", "add", "-A").Run(); err != nil {
		return err
	}
	return exec.Command("git", "commit", "-m", message).Run()
}
