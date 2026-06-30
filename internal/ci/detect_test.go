package ci

import "testing"

// h-tymh: provider detection from the git origin remote. github.com → github;
// any gitlab host (gitlab.com or self-hosted gitlab.*) → gitlab; else unknown.
// Handles both ssh (git@host:owner/repo) and https (https://host/owner/repo) forms.
func TestDetectProvider(t *testing.T) {
	cases := []struct {
		name   string
		remote string
		want   string
	}{
		{"github ssh", "git@github.com:acme/repo.git", "github"},
		{"github https", "https://github.com/acme/repo.git", "github"},
		{"github no .git", "https://github.com/acme/repo", "github"},
		{"gitlab.com ssh", "git@gitlab.com:acme/repo.git", "gitlab"},
		{"gitlab.com https", "https://gitlab.com/acme/repo.git", "gitlab"},
		{"gitlab self-hosted", "git@gitlab.example.com:acme/repo.git", "gitlab"},
		{"gitlab self-hosted https", "https://gitlab.internal.corp/acme/repo.git", "gitlab"},
		{"unknown bitbucket", "git@bitbucket.org:acme/repo.git", "unknown"},
		{"empty", "", "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DetectProvider(tc.remote); got != tc.want {
				t.Errorf("DetectProvider(%q) = %q, want %q", tc.remote, got, tc.want)
			}
		})
	}
}
