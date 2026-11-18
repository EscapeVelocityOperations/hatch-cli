package ci

import (
	"net/url"
	"regexp"
	"strings"
)

// SecretCommand builds the provider CLI command that sets the HATCH_TOKEN CI
// secret: `gh secret set` for GitHub, `glab variable set --masked --protected`
// for GitLab. --masked only hides the value in job logs; --protected is what
// restricts the variable to protected branches and tags, so a pipeline on an
// unprotected branch (e.g. a contributor's own job) never receives the token.
// The command never carries the secret value: both CLIs read it from standard
// input when it is omitted from the arguments, so the token never appears in
// argv (visible to other local users via the process list) nor in any printed
// command. bin is the executable and args its arguments. repo is the -R target
// (see RepoRefFromRemote) and may be "" → the -R flag is omitted and the
// provider CLI targets the current repo. ok is false for an unsupported
// provider.
func SecretCommand(provider, repo string) (bin string, args []string, ok bool) {
	switch provider {
	case "github":
		args = []string{"secret", "set", "HATCH_TOKEN"}
		if repo != "" {
			args = append(args, "-R", repo)
		}
		return "gh", args, true
	case "gitlab":
		args = []string{"variable", "set", "HATCH_TOKEN", "--masked", "--protected"}
		if repo != "" {
			args = append(args, "-R", repo)
		}
		return "glab", args, true
	default:
		return "", nil, false
	}
}

// SecretCommandString renders a copy-pasteable command. It contains no secret
// value: the provider CLI reads the token from standard input (gh prompts for it
// on a terminal).
func SecretCommandString(provider, repo string) string {
	bin, args, ok := SecretCommand(provider, repo)
	if !ok {
		return ""
	}
	return bin + " " + strings.Join(args, " ")
}

var (
	hostRe    = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$`)
	segmentRe = regexp.MustCompile(`^[A-Za-z0-9_.][A-Za-z0-9_.-]*$`)
	portRe    = regexp.MustCompile(`^[0-9]{1,5}$`)
)

// defaultHost is the public instance a provider CLI targets when -R carries no
// host.
func defaultHost(provider string) string {
	switch provider {
	case "github":
		return "github.com"
	case "gitlab":
		return "gitlab.com"
	}
	return ""
}

// RepoRefFromRemote derives the -R target for the provider CLI from a git
// remote URL, keeping everything that identifies the repository: the FULL
// namespace (GitLab subgroups: org/team/project) and the host.
//
//   - the provider's public host with a two-segment path → "owner/repo";
//   - anything else (subgroups, self-hosted GitLab, GitHub Enterprise) → an
//     explicit "https://host/full/path" — both gh and glab accept a repository
//     URL, and unlike a bare "a/b/c" it cannot be read as "HOST/GROUP/REPO"
//     (GitLab group paths may contain dots).
//
// SSH/git remotes (scp-like git@host:path, ssh://, git://) map to https and
// drop the SSH port; http(s) remotes keep their scheme and port. Credentials in
// the remote are dropped. The result is built only from validated characters
// because the command is printed to be pasted into a shell: "" is returned when
// the remote is not derivable or not clean, and the provider CLI then resolves
// the current repository itself.
func RepoRefFromRemote(provider, remote string) string {
	scheme, host, port, path, ok := parseRemote(strings.TrimSpace(remote))
	if !ok || !hostRe.MatchString(host) || (port != "" && !portRe.MatchString(port)) {
		return ""
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	segments := strings.Split(path, "/")
	if len(segments) < 2 {
		return ""
	}
	for _, seg := range segments {
		if seg == "." || seg == ".." || !segmentRe.MatchString(seg) {
			return ""
		}
	}
	if host == defaultHost(provider) && port == "" && len(segments) == 2 {
		return path
	}
	authority := host
	if port != "" {
		authority += ":" + port
	}
	return scheme + "://" + authority + "/" + path
}

// parseRemote splits a git remote into the https scheme to use, host, port (http
// remotes only) and path.
func parseRemote(remote string) (scheme, host, port, path string, ok bool) {
	if !strings.Contains(remote, "://") {
		// scp-like: [user@]host:path
		hostPart, rest, found := strings.Cut(remote, ":")
		if !found || strings.Contains(hostPart, "/") {
			return "", "", "", "", false
		}
		if at := strings.LastIndex(hostPart, "@"); at >= 0 {
			hostPart = hostPart[at+1:]
		}
		return "https", strings.ToLower(hostPart), "", rest, true
	}
	u, err := url.Parse(remote)
	if err != nil {
		return "", "", "", "", false
	}
	switch u.Scheme {
	case "http", "https":
		return u.Scheme, strings.ToLower(u.Hostname()), u.Port(), u.Path, true
	case "ssh", "git", "git+ssh", "ssh+git":
		// the port of an SSH remote is not the web port
		return "https", strings.ToLower(u.Hostname()), "", u.Path, true
	}
	return "", "", "", "", false
}
