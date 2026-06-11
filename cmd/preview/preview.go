// Package preview implements `hatch preview list|rm` — managing the PR
// preview eggs of a parent app (h-qtie8 sub-bead 2).
//
// TDD-red stub (h-6brzi): preview_test.go encodes the contract; the impl
// step fills runList/runRm and the cobra wiring.
package preview

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/auth"
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
			return "", errors.New("not implemented")
		},
	}
}

var deps = defaultDeps()

var errNotImplemented = errors.New("not implemented")

// NewCmd returns the `hatch preview` command group.
func NewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "preview",
		Short: "Manage PR preview eggs",
	}
}

func runList(cmd *cobra.Command, args []string) error {
	return errNotImplemented
}

func runRm(cmd *cobra.Command, args []string) error {
	return errNotImplemented
}
