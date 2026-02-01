package git

import (
	"os/exec"
	"strings"
)

// HasRemote checks if a remote with the given name exists.
func HasRemote(name string) bool {
	cmd := exec.Command("git", "remote", "get-url", name)
	return cmd.Run() == nil
}

// AddRemote adds a new git remote.
func AddRemote(name, url string) error {
	return exec.Command("git", "remote", "add", name, url).Run()
}

// SetRemoteURL updates the URL of an existing remote.
func SetRemoteURL(name, url string) error {
	return exec.Command("git", "remote", "set-url", name, url).Run()
}

// GetRemoteURL returns the URL of the named remote.
func GetRemoteURL(name string) (string, error) {
	cmd := exec.Command("git", "remote", "get-url", name)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Push pushes the given branch to the named remote with --force.
// It returns the combined stdout+stderr output for parsing.
func Push(remote, branch string) (string, error) {
	cmd := exec.Command("git", "push", remote, branch, "--force")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// CurrentBranch returns the name of the current git branch.
func CurrentBranch() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
