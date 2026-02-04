package api

import (
	"fmt"
	"regexp"
	"strings"
)

// slugPattern matches: https://<token>@git.gethatch.eu/<slug>.git
// Also supports legacy format with /deploy/ prefix: git.gethatch.eu/deploy/<slug>.git
var slugPattern = regexp.MustCompile(`git\.gethatch\.eu/(?:deploy/)?([^/.]+)`)

// SlugFromRemote extracts the app slug from a hatch git remote URL.
func SlugFromRemote(remoteURL string) (string, error) {
	matches := slugPattern.FindStringSubmatch(remoteURL)
	if len(matches) < 2 {
		return "", fmt.Errorf("could not detect app from git remote URL: %s", remoteURL)
	}
	return matches[1], nil
}

// NormalizeSlug cleans up a slug value, trimming whitespace.
func NormalizeSlug(s string) string {
	return strings.TrimSpace(s)
}
