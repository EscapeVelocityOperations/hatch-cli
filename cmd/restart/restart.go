package restart

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/auth"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/resolve"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/ui"
	"github.com/spf13/cobra"
)

// Deps holds injectable dependencies for testing.
type Deps struct {
	GetToken   func() (string, error)
	RestartApp func(token, slug string) error
	Confirm    func(prompt string) bool
}

func defaultDeps() *Deps {
	return &Deps{
		GetToken: auth.GetToken,
		RestartApp: func(token, slug string) error {
			return api.NewClient(token).RestartApp(slug)
		},
		Confirm: confirmPrompt,
	}
}

var deps = defaultDeps()

var (
	appSlug    string
	skipPrompt bool
)

// NewCmd returns the restart command.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restart [slug]",
		Short: "Restart a egg",
		Long:  "Restart a Hatch egg. Requires confirmation unless --yes is provided.",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runRestart,
	}
	cmd.Flags().StringVarP(&appSlug, "app", "a", "", "egg slug (auto-detected from git remote if omitted)")
	cmd.Flags().BoolVarP(&skipPrompt, "yes", "y", false, "skip confirmation prompt")
	return cmd
}

func runRestart(cmd *cobra.Command, args []string) error {
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

	if skipPrompt {
		// Skip confirmation
	} else if !deps.Confirm(fmt.Sprintf("Restart %s?", slug)) {
		ui.Info("Cancelled.")
		return nil
	}

	sp := ui.NewSpinner(fmt.Sprintf("Restarting %s...", slug))
	sp.Start()
	err = deps.RestartApp(token, slug)
	sp.Stop()

	if err != nil {
		return fmt.Errorf("restarting egg: %w", err)
	}

	ui.Success(fmt.Sprintf("Restarted %s", slug))
	return nil
}

func resolveSlug(args []string) (string, error) {
	if appSlug != "" {
		return appSlug, nil
	}
	if len(args) > 0 {
		return args[0], nil
	}
	if slug := resolve.SlugFromToml(); slug != "" {
		return slug, nil
	}
	return "", fmt.Errorf("no egg specified. Usage: hatch restart <slug> (or set slug in .hatch.toml)")
}

func confirmPrompt(prompt string) bool {
	fmt.Printf("%s [y/N] ", prompt)
	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	return strings.TrimSpace(strings.ToLower(answer)) == "y"
}
