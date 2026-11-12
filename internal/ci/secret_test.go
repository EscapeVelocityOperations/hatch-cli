package ci

import (
	"strings"
	"testing"
)

// h-mkyo: SecretCommand builds the provider CLI command that sets the HATCH_TOKEN
// CI secret — `gh secret set` for GitHub, `glab variable set --masked --protected`
// for GitLab. h-8tvak.1: the command never carries the value (both CLIs read it on
// stdin). h-8tvak.3: the GitLab variable is protected as well as masked.
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
	if gl != "glab variable set HATCH_TOKEN --masked --protected -R acme/repo" {
		t.Errorf("gitlab command = %q", gl)
	}

	if _, _, ok := SecretCommand("bitbucket", "acme/repo"); ok {
		t.Error("unknown provider should not be ok")
	}

	if _, args, _ := SecretCommand("github", ""); strings.Contains(strings.Join(args, " "), "-R") {
		t.Error("empty ownerRepo must omit -R (provider CLI uses the current repo)")
	}
	if got := SecretCommandString("gitlab", ""); got != "glab variable set HATCH_TOKEN --masked --protected" {
		t.Errorf("SecretCommandString = %q", got)
	}
}

// h-8tvak.3: masking only hides the value in job logs; it does not restrict which
// pipelines receive it. A GitLab variable that is not protected is exposed to a
// job on any branch, so a contributor who can push a branch could read the
// deploy token. The GitLab command must therefore set --protected (limits the
// variable to protected branches and tags) as well as --masked, with or without
// an explicit repo, and the GitHub command must be unchanged.
func TestSecretCommand_GitLabVariableIsProtectedAndMasked(t *testing.T) {
	for _, repo := range []string{"", "acme/repo"} {
		_, args, ok := SecretCommand("gitlab", repo)
		if !ok {
			t.Fatalf("gitlab repo=%q: not ok", repo)
		}
		have := map[string]int{}
		for _, a := range args {
			have[a]++
		}
		for _, flag := range []string{"--masked", "--protected"} {
			if have[flag] != 1 {
				t.Errorf("gitlab repo=%q: %s appears %d times in %v, want exactly once", repo, flag, have[flag], args)
			}
		}
		// The variable name must stay the first positional argument after `set`,
		// and the value must still never be an argument (stdin only).
		if len(args) < 3 || args[0] != "variable" || args[1] != "set" || args[2] != "HATCH_TOKEN" {
			t.Errorf("gitlab repo=%q: args = %v, want variable set HATCH_TOKEN …", repo, args)
		}
		if repo != "" && !strings.HasSuffix(strings.Join(args, " "), "-R "+repo) {
			t.Errorf("gitlab repo=%q: -R not kept last: %v", repo, args)
		}
	}
	_, args, _ := SecretCommand("github", "acme/repo")
	if strings.Contains(strings.Join(args, " "), "protected") {
		t.Errorf("github command must not carry GitLab flags: %v", args)
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
