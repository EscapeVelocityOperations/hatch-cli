package open

import (
	"fmt"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/auth"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/resolve"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/ui"
	"github.com/spf13/cobra"
)

// Deps holds injectable dependencies for testing.
type Deps struct {
	GetToken    func() (string, error)
	OpenBrowser func(url string) error
}

func defaultDeps() *Deps {
	return &Deps{
		GetToken:    auth.GetToken,
		OpenBrowser: auth.OpenBrowser,
	}
}

var deps = defaultDeps()

// NewCmd returns the open command.
func NewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "open [slug]",
		Short: "Open egg in browser",
		Long:  "Open the Hatch egg URL in your default browser. If no slug is provided, the egg is detected from the current git remote.",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runOpen,
	}
}

func runOpen(cmd *cobra.Command, args []string) error {
	token, err := deps.GetToken()
	if err != nil {
		return fmt.Errorf("checking auth: %w", err)
	}
	if token == "" {
		return fmt.Errorf("not logged in. Run 'hatch login', set HATCH_TOKEN, or use --token")
	}

	slug, err := resolveSlug(args)
	if err != nil {
		return err
	}

	appURL := fmt.Sprintf("https://%s.nest.gethatch.eu", slug)
	ui.Info(fmt.Sprintf("Opening %s...", appURL))

	if err := deps.OpenBrowser(appURL); err != nil {
		return fmt.Errorf("opening browser: %w", err)
	}
	return nil
}

func resolveSlug(args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}
	if slug := resolve.SlugFromToml(); slug != "" {
		return slug, nil
	}
	return "", fmt.Errorf("no egg specified. Usage: hatch open <slug> (or set slug in .hatch.toml)")
}
