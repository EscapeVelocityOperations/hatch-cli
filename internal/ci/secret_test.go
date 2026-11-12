package ci

import (
	"strings"
	"testing"
)

// h-mkyo: SecretCommand builds the provider CLI command that sets the HATCH_TOKEN
// CI secret — `gh secret set` for GitHub, `glab variable set --masked` for GitLab.
// h-8tvak.1: the command never carries the value (both CLIs read it on stdin).
func TestSecretCommand(t *testing.T) {
	bin, args, ok := SecretCommand("github", "acme/repo")
	if !ok || bin != "gh" {
		t.Fatalf("github: bin=%q ok=%v, want gh/true", bin, ok)
	}
	gh := bin + " " + strings.Join(args, " ")
	if gh != "gh secret set HATCH_TOKEN -R acme/repo" {
		t.Errorf("github command = %q", gh)
	}
	for _, a := range args {
		if a == "--body" || a == "-b" {
			t.Errorf("github command must not pass the value as an argument: %s", gh)
		}
	}

	bin, args, ok = SecretCommand("gitlab", "acme/repo")
	if !ok || bin != "glab" {
		t.Fatalf("gitlab: bin=%q ok=%v, want glab/true", bin, ok)
	}
	gl := bin + " " + strings.Join(args, " ")
	if gl != "glab variable set HATCH_TOKEN --masked -R acme/repo" {
		t.Errorf("gitlab command = %q", gl)
	}

	if _, _, ok := SecretCommand("bitbucket", "acme/repo"); ok {
		t.Error("unknown provider should not be ok")
	}

	if _, args, _ := SecretCommand("github", ""); strings.Contains(strings.Join(args, " "), "-R") {
		t.Error("empty ownerRepo must omit -R (provider CLI uses the current repo)")
	}
	if got := SecretCommandString("gitlab", ""); got != "glab variable set HATCH_TOKEN --masked" {
		t.Errorf("SecretCommandString = %q", got)
	}
}

func TestOwnerRepoFromRemote(t *testing.T) {
	cases := map[string]string{
		"git@github.com:acme/repo.git":       "acme/repo",
		"https://github.com/acme/repo.git":   "acme/repo",
		"https://gitlab.com/acme/repo":       "acme/repo",
		"git@gitlab.example.com:grp/sub.git": "grp/sub",
		"":                                   "",
		"not-a-url":                          "",
	}
	for remote, want := range cases {
		if got := OwnerRepoFromRemote(remote); got != want {
			t.Errorf("OwnerRepoFromRemote(%q) = %q, want %q", remote, got, want)
		}
	}
}
