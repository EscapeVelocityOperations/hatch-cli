package ci

import (
	"strings"
	"testing"
)

// h-mkyo: SecretCommand builds the provider CLI command that sets the HATCH_TOKEN
// CI secret — `gh secret set` for GitHub, `glab variable set --masked` for GitLab.
func TestSecretCommand(t *testing.T) {
	bin, args, ok := SecretCommand("github", "acme/repo", "hatch_tok123")
	if !ok || bin != "gh" {
		t.Fatalf("github: bin=%q ok=%v, want gh/true", bin, ok)
	}
	gh := bin + " " + strings.Join(args, " ")
	for _, want := range []string{"gh secret set HATCH_TOKEN", "--body hatch_tok123", "-R acme/repo"} {
		if !strings.Contains(gh, want) {
			t.Errorf("github command missing %q: %s", want, gh)
		}
	}

	bin, args, ok = SecretCommand("gitlab", "acme/repo", "hatch_tok123")
	if !ok || bin != "glab" {
		t.Fatalf("gitlab: bin=%q ok=%v, want glab/true", bin, ok)
	}
	gl := bin + " " + strings.Join(args, " ")
	for _, want := range []string{"glab variable set HATCH_TOKEN hatch_tok123", "--masked", "-R acme/repo"} {
		if !strings.Contains(gl, want) {
			t.Errorf("gitlab command missing %q: %s", want, gl)
		}
	}

	if _, _, ok := SecretCommand("bitbucket", "acme/repo", "t"); ok {
		t.Error("unknown provider should not be ok")
	}

	if _, args, _ := SecretCommand("github", "", "t"); strings.Contains(strings.Join(args, " "), "-R") {
		t.Error("empty ownerRepo must omit -R (provider CLI uses the current repo)")
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
