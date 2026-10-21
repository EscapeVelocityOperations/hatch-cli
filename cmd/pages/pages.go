// Package pages implements `hatch pages` — publish, replace (as numbered
// versions), list and remove pages on a protected egg (h-8iz8). Pages are
// served by the Hatch gateway under /pages/<name>/ with no rebuild and
// without waking the egg.
//
// Every verb is its own subcommand with an explicit Args rule and the egg as
// a required positional, and the group itself rejects positionals: a parent
// with RunE and no Args would silently ignore the slug (h-abmr). Every
// failure exits non-zero, terminal or not (h-59ej).
package pages

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/auth"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/exitcode"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// APIClient is the pages surface of the Hatch API (*api.Client).
type APIClient interface {
	PublishPage(slug string, archive []byte, meta api.PagePublishMeta) (*api.PagePublishResult, error)
	ListPages(slug string) (*api.PageList, error)
	ListPageVersions(slug, page string) (*api.PageVersions, error)
	RemovePage(slug, page string, version int, purge bool) (*api.PageRemoveResult, error)
}

// Deps holds injectable dependencies for testing (cmd/comments pattern).
type Deps struct {
	GetToken        func() (string, error)
	NewAPIClient    func(token string) APIClient
	Now             func() time.Time
	StdinIsTerminal func() bool
}

var deps = defaultDeps()

func defaultDeps() *Deps {
	return &Deps{
		GetToken:        auth.GetToken,
		NewAPIClient:    func(token string) APIClient { return api.NewClient(token) },
		Now:             time.Now,
		StdinIsTerminal: func() bool { return term.IsTerminal(int(os.Stdin.Fd())) },
	}
}

// NewCmd returns the `hatch pages` command group.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pages",
		Short: "Publish versioned pages on your protected eggs, without a rebuild",
		Long: "Hatch pages publishes an HTML page (or a folder with an index.html) onto an egg at\n" +
			"/pages/<name>/. Publishing again at the same name creates a new version: the main link\n" +
			"always shows the latest, and every version stays readable at /pages/<name>/v<N>/.\n\n" +
			"Pages are served by the Hatch gateway — no build, no deploy, the egg may stay asleep —\n" +
			"so they are available on password- or email-protected eggs only.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	cmd.AddCommand(newPublishCmd(), newListCmd(), newVersionsCmd(), newRemoveCmd())
	return cmd
}

// runE wraps a RunE so every failure carries exit status 1, even outside a
// terminal (h-59ej): scripts and agents must see a refused publish.
func runE(fn func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		return exitcode.Wrap(1, fn(cmd, args))
	}
}

func client() (APIClient, error) {
	token, err := deps.GetToken()
	if err != nil || token == "" {
		return nil, errors.New("not logged in (run 'hatch login' first)")
	}
	return deps.NewAPIClient(token), nil
}

// apiErr wraps an API error with a hint for the pages error codes
// (hatch-api / hatch-control h-8iz8).
func apiErr(action, slug string, err error) error {
	msg := err.Error()
	hint := ""
	switch {
	case strings.Contains(msg, "pages_require_gateway"):
		hint = fmt.Sprintf("pages are served by the Hatch gateway, so %s must be protected first: "+
			"'hatch protect password enable' or 'hatch protect email enable'", slug)
	case strings.Contains(msg, "pages_index_required"):
		hint = "a published folder needs an index.html at its root"
	case strings.Contains(msg, "pages_file_too_large"):
		hint = "a page file is at most 50 MB"
	case strings.Contains(msg, "pages_publication_too_large"), strings.Contains(msg, "pages_too_large"):
		hint = "a publication is at most 200 MB"
	case strings.Contains(msg, "pages_too_many_files"):
		hint = "a version holds at most 2000 files"
	case strings.Contains(msg, "pages_limit_reached"):
		hint = fmt.Sprintf("%s has the maximum number of pages (200): remove some first", slug)
	case strings.Contains(msg, "pages_version_limit"):
		hint = "this page has the maximum number of versions (50): remove old ones with 'hatch pages remove <egg> <page> --version N'"
	case strings.Contains(msg, "pages_quota_exceeded"):
		hint = fmt.Sprintf("%s's pages storage is full (2 GB): remove old pages or versions", slug)
	case strings.Contains(msg, "pages_unsafe_path"), strings.Contains(msg, "pages_unsafe_entry"):
		hint = "the folder holds a name or file type Hatch refuses (links, devices, '..', control characters)"
	case strings.Contains(msg, "invalid_page_name"):
		hint = "a page name is lowercase letters, digits and hyphens: --at /pages/bilan-2026/"
	case strings.Contains(msg, "page version not found"):
		hint = "no such version (see 'hatch pages versions')"
	case strings.Contains(msg, "page not found"):
		hint = fmt.Sprintf("no such page on %s (see 'hatch pages list %s')", slug, slug)
	case strings.Contains(msg, "API error 404"):
		hint = fmt.Sprintf("no such egg, or you are not an owner/collaborator of %s", slug)
	}
	if hint != "" {
		return fmt.Errorf("%s: %w (%s)", action, err, hint)
	}
	return fmt.Errorf("%s: %w", action, err)
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func humanBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
