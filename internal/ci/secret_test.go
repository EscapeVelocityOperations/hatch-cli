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

// h-8tvak.2 (Vigie): the -R target keeps the FULL namespace (GitLab subgroups)
// and the host of origin. Truncating to the last two path components pointed
// `glab variable set` at a different project (org/team/project -> team/project),
// and dropping the host pointed a self-hosted instance's repo at gitlab.com.
func TestRepoRefFromRemote(t *testing.T) {
	cases := []struct {
		name, provider, remote, want string
	}{
		// public host, two segments: the short, unambiguous owner/repo form.
		{"github scp", "github", "git@github.com:acme/repo.git", "acme/repo"},
		{"github https", "github", "https://github.com/acme/repo.git", "acme/repo"},
		{"github https no .git", "github", "https://github.com/acme/repo", "acme/repo"},
		{"github ssh url", "github", "ssh://git@github.com/acme/repo.git", "acme/repo"},
		{"gitlab https", "gitlab", "https://gitlab.com/acme/repo", "acme/repo"},
		{"trailing newline and slash", "gitlab", "https://gitlab.com/acme/repo/\n", "acme/repo"},
		{"uppercase host", "github", "git@GitHub.com:acme/repo.git", "acme/repo"},
		{"leading slash in scp path", "gitlab", "git@gitlab.com:/acme/repo.git", "acme/repo"},
		{"dotted repo names", "github", "git@github.com:acme/.github.git", "acme/.github"},

		// subgroups: the whole namespace, as an explicit URL (a bare a/b/c is
		// ambiguous with HOST/GROUP/REPO because group paths may contain dots).
		{"gitlab subgroup https", "gitlab", "https://gitlab.com/org/team/project.git", "https://gitlab.com/org/team/project"},
		{"gitlab subgroup scp", "gitlab", "git@gitlab.com:org/team/project.git", "https://gitlab.com/org/team/project"},
		{"gitlab subgroup ssh url", "gitlab", "ssh://git@gitlab.com/org/team/project.git", "https://gitlab.com/org/team/project"},
		{"gitlab deep subgroups", "gitlab", "https://gitlab.com/org/a/b/c/project.git", "https://gitlab.com/org/a/b/c/project"},
		{"dotted group name", "gitlab", "git@gitlab.com:my.group/sub/project.git", "https://gitlab.com/my.group/sub/project"},

		// self-hosted instances: the host is kept.
		{"self-hosted gitlab scp", "gitlab", "git@gitlab.example.com:grp/sub.git", "https://gitlab.example.com/grp/sub"},
		{"self-hosted gitlab subgroup https", "gitlab", "https://gitlab.example.com/grp/sub/project.git", "https://gitlab.example.com/grp/sub/project"},
		{"self-hosted gitlab ssh port is not the https port", "gitlab", "ssh://git@gitlab.example.com:2222/grp/sub/project.git", "https://gitlab.example.com/grp/sub/project"},
		{"self-hosted https port is kept", "gitlab", "https://gitlab.example.com:8443/grp/sub/project.git", "https://gitlab.example.com:8443/grp/sub/project"},
		{"http scheme is kept", "gitlab", "http://gitlab.internal/grp/project.git", "http://gitlab.internal/grp/project"},
		{"GitHub Enterprise", "github", "git@ghe.example.com:acme/repo.git", "https://ghe.example.com/acme/repo"},
		{"remote on another forge than the provider", "gitlab", "git@github.com:acme/repo.git", "https://github.com/acme/repo"},

		// credentials in the remote never reach the command.
		{"userinfo with password is dropped", "gitlab", "https://user:s3cret@gitlab.example.com/grp/sub/project.git", "https://gitlab.example.com/grp/sub/project"},
		{"userinfo on default host is dropped", "github", "https://x-access-token:s3cret@github.com/acme/repo.git", "acme/repo"},

		// not derivable -> "" (the provider CLI then resolves the current repo).
		{"empty", "github", "", ""},
		{"not a url", "github", "not-a-url", ""},
		{"single segment", "gitlab", "git@gitlab.com:project.git", ""},
		{"host only", "gitlab", "https://gitlab.com/", ""},
		{"empty segment", "gitlab", "https://gitlab.com/org//project.git", ""},
		{"dot-dot segment", "gitlab", "https://gitlab.com/org/../project.git", ""},
		{"local path", "github", "/srv/git/acme/repo.git", ""},
		{"file url", "github", "file:///srv/git/acme/repo.git", ""},
		{"unsupported scheme", "github", "ftp://github.com/acme/repo.git", ""},
		{"shell metacharacters in path", "gitlab", "git@gitlab.com:acme/repo;rm -rf ~.git", ""},
		{"command substitution in path", "gitlab", "https://gitlab.com/acme/$(id)/repo.git", ""},
		{"shell metacharacters in host", "gitlab", "https://gitlab.com;id/acme/repo.git", ""},
		{"option-like owner", "github", "git@github.com:-x/repo.git", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := RepoRefFromRemote(c.provider, c.remote); got != c.want {
				t.Errorf("RepoRefFromRemote(%q, %q) = %q, want %q", c.provider, c.remote, got, c.want)
			}
		})
	}
}

// h-8tvak.2: the command built from a subgroup/self-hosted target carries the
// full target in -R, for both providers.
func TestSecretCommand_FullTarget(t *testing.T) {
	if got := SecretCommandString("gitlab", "https://gitlab.com/org/team/project"); got != "glab variable set HATCH_TOKEN --masked --protected -R https://gitlab.com/org/team/project" {
		t.Errorf("gitlab subgroup command = %q", got)
	}
	if got := SecretCommandString("github", "https://ghe.example.com/acme/repo"); got != "gh secret set HATCH_TOKEN -R https://ghe.example.com/acme/repo" {
		t.Errorf("github enterprise command = %q", got)
	}
}
