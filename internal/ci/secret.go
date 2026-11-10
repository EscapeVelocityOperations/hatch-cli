package ci

import "strings"

// SecretCommand builds the provider CLI command that sets the HATCH_TOKEN CI
// secret: `gh secret set` for GitHub, `glab variable set --masked` for GitLab.
// The command never carries the secret value: both CLIs read it from standard
// input when it is omitted from the arguments, so the token never appears in
// argv (visible to other local users via the process list) nor in any printed
// command. bin is the executable and args its arguments. ownerRepo (owner/repo)
// may be "" → the -R flag is omitted and the provider CLI targets the current
// repo. ok is false for an unsupported provider.
func SecretCommand(provider, ownerRepo string) (bin string, args []string, ok bool) {
	switch provider {
	case "github":
		args = []string{"secret", "set", "HATCH_TOKEN"}
		if ownerRepo != "" {
			args = append(args, "-R", ownerRepo)
		}
		return "gh", args, true
	case "gitlab":
		args = []string{"variable", "set", "HATCH_TOKEN", "--masked"}
		if ownerRepo != "" {
			args = append(args, "-R", ownerRepo)
		}
		return "glab", args, true
	default:
		return "", nil, false
	}
}

// SecretCommandString renders a copy-pasteable command. It contains no secret
// value: the provider CLI reads the token from standard input (gh prompts for it
// on a terminal).
func SecretCommandString(provider, ownerRepo string) string {
	bin, args, ok := SecretCommand(provider, ownerRepo)
	if !ok {
		return ""
	}
	return bin + " " + strings.Join(args, " ")
}

// OwnerRepoFromRemote extracts owner/repo from a git remote URL
// (git@host:owner/repo.git or https://host/owner/repo.git); "" if not derivable.
func OwnerRepoFromRemote(remote string) string {
	remote = strings.TrimSuffix(strings.TrimSpace(remote), ".git")
	remote = strings.ReplaceAll(remote, ":", "/")
	parts := strings.Split(remote, "/")
	if len(parts) < 2 {
		return ""
	}
	owner, repo := parts[len(parts)-2], parts[len(parts)-1]
	if owner == "" || repo == "" {
		return ""
	}
	return owner + "/" + repo
}
