package open

import (
	"fmt"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/auth"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/git"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/ui"
	"github.com/spf13/cobra"
)

// Deps holds injectable dependencies for testing.
type Deps struct {
	GetToken     func() (string, error)
	HasRemote    func(name string) bool
	GetRemoteURL func(name string) (string, error)
	OpenBrowser  func(url string) error
}

func defaultDeps() *Deps {
	return &Deps{
		GetToken:     auth.GetToken,
		HasRemote:    git.HasRemote,
		GetRemoteURL: git.GetRemoteURL,
		OpenBrowser:  auth.OpenBrowser,
	}
}

var deps = defaultDeps()

// NewCmd returns the open command.
func NewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "open [slug]",
		Short: "Open application in browser",
		Long:  "Open the Hatch application URL in your default browser. If no slug is provided, the app is detected from the current git remote.",
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

	appURL := fmt.Sprintf("https://%s.gethatch.eu", slug)
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
	if !deps.HasRemote("hatch") {
		return "", fmt.Errorf("no app specified and no hatch git remote found. Usage: hatch open <slug>")
	}
	url, err := deps.GetRemoteURL("hatch")
	if err != nil {
		return "", fmt.Errorf("reading hatch remote: %w", err)
	}
	return api.SlugFromRemote(url)
}
