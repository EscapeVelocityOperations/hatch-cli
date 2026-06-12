// Package preview implements `hatch preview list|rm` — managing the PR
// preview eggs of a parent app (h-qtie8 sub-bead 2).
//
// TDD-red stub (h-6brzi): preview_test.go encodes the contract; the impl
// step fills runList/runRm and the cobra wiring.
package preview

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/auth"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/resolve"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/ui"
)

// APIClient is the preview-facing slice of the Hatch API.
type APIClient interface {
	ListPreviews(parentSlug string) ([]api.Preview, error)
	DeletePreview(parentSlug string, prNumber int) error
}

// Deps holds injectable dependencies for testing.
type Deps struct {
	GetToken     func() (string, error)
	NewAPIClient func(token string) APIClient
	// ResolveParent resolves the parent app slug (.hatch.toml or --app).
	ResolveParent func() (string, error)
}

type realAPIClient struct {
	client *api.Client
}

func (r *realAPIClient) ListPreviews(parentSlug string) ([]api.Preview, error) {
	return r.client.ListPreviews(parentSlug)
}

func (r *realAPIClient) DeletePreview(parentSlug string, prNumber int) error {
	return r.client.DeletePreview(parentSlug, prNumber)
}

func defaultDeps() *Deps {
	return &Deps{
		GetToken: auth.GetToken,
		NewAPIClient: func(token string) APIClient {
			return &realAPIClient{client: api.NewClient(token)}
		},
		ResolveParent: func() (string, error) {
			if slug := resolve.SlugFromToml(); slug != "" {
				return slug, nil
			}
			return "", fmt.Errorf("no .hatch.toml found — run from your app directory or pass --app")
		},
	}
}

var deps = defaultDeps()

// NewCmd returns the `hatch preview` command group.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "preview",
		Short: "Manage PR preview eggs",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List the parent egg's active previews",
		RunE:  runList,
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "rm <pr-N|N>",
		Short: "Tear down the preview for a PR",
		Args:  cobra.ExactArgs(1),
		RunE:  runRm,
	})
	return cmd
}

// previewClient resolves the parent app and builds the API client.
func previewClient() (APIClient, string, error) {
	parent, err := deps.ResolveParent()
	if err != nil {
		return nil, "", err
	}
	token, err := deps.GetToken()
	if err != nil {
		return nil, "", fmt.Errorf("not logged in: %w (run 'hatch login' first)", err)
	}
	return deps.NewAPIClient(token), parent, nil
}

func runList(cmd *cobra.Command, args []string) error {
	client, parent, err := previewClient()
	if err != nil {
		return err
	}

	previews, err := client.ListPreviews(parent)
	if err != nil {
		return fmt.Errorf("listing previews: %w", err)
	}
	if len(previews) == 0 {
		fmt.Printf("No previews for %s.\n", parent)
		return nil
	}

	table := ui.NewTable(os.Stdout, "PR", "SLUG", "STATUS", "URL", "EXPIRES")
	for _, p := range previews {
		table.AddRow(strconv.Itoa(p.PRNumber), p.Slug, p.Status, p.URL, p.ExpiresAt.UTC().Format("2006-01-02 15:04"))
	}
	table.Render()
	return nil
}

// parsePreviewRef parses "pr-<n>" or a bare "<n>", n > 0.
func parsePreviewRef(ref string) (int, error) {
	n, err := strconv.Atoi(strings.TrimPrefix(ref, "pr-"))
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid preview ref %q: want pr-<number> or <number> (positive PR number)", ref)
	}
	return n, nil
}

func runRm(cmd *cobra.Command, args []string) error {
	pr, err := parsePreviewRef(args[0])
	if err != nil {
		return err
	}

	client, parent, err := previewClient()
	if err != nil {
		return err
	}

	if err := client.DeletePreview(parent, pr); err != nil {
		return fmt.Errorf("removing preview pr-%d: %w", pr, err)
	}
	fmt.Printf("Preview pr-%d of %s torn down.\n", pr, parent)
	return nil
}
