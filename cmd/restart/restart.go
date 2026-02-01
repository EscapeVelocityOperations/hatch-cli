package restart

import (
	"bufio"
	"fmt"
	"os"
	"strings"

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
	RestartApp   func(token, slug string) error
	Confirm      func(prompt string) bool
}

func defaultDeps() *Deps {
	return &Deps{
		GetToken:     auth.GetToken,
		HasRemote:    git.HasRemote,
		GetRemoteURL: git.GetRemoteURL,
		RestartApp: func(token, slug string) error {
			return api.NewClient(token).RestartApp(slug)
		},
		Confirm: confirmPrompt,
	}
}

var deps = defaultDeps()

// NewCmd returns the restart command.
func NewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart [slug]",
		Short: "Restart an application",
		Long:  "Restart a Hatch application. Requires confirmation unless --yes is provided.",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runRestart,
	}
}

func runRestart(cmd *cobra.Command, args []string) error {
	token, err := deps.GetToken()
	if err != nil {
		return fmt.Errorf("checking auth: %w", err)
	}
	if token == "" {
		return fmt.Errorf("not logged in. Run 'hatch login' first")
	}

	slug, err := resolveSlug(args)
	if err != nil {
		return err
	}

	if !deps.Confirm(fmt.Sprintf("Restart %s?", slug)) {
		ui.Info("Cancelled.")
		return nil
	}

	sp := ui.NewSpinner(fmt.Sprintf("Restarting %s...", slug))
	sp.Start()
	err = deps.RestartApp(token, slug)
	sp.Stop()

	if err != nil {
		return fmt.Errorf("restarting app: %w", err)
	}

	ui.Success(fmt.Sprintf("Restarted %s", slug))
	return nil
}

func resolveSlug(args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}
	if !deps.HasRemote("hatch") {
		return "", fmt.Errorf("no app specified and no hatch git remote found. Usage: hatch restart <slug>")
	}
	url, err := deps.GetRemoteURL("hatch")
	if err != nil {
		return "", fmt.Errorf("reading hatch remote: %w", err)
	}
	return api.SlugFromRemote(url)
}

func confirmPrompt(prompt string) bool {
	fmt.Printf("%s [y/N] ", prompt)
	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	return strings.TrimSpace(strings.ToLower(answer)) == "y"
}
