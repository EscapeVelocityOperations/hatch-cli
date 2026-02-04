package destroy

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
	DeleteApp    func(token, slug string) error
	ReadInput    func(prompt string) (string, error)
}

func defaultDeps() *Deps {
	return &Deps{
		GetToken:     auth.GetToken,
		HasRemote:    git.HasRemote,
		GetRemoteURL: git.GetRemoteURL,
		DeleteApp: func(token, slug string) error {
			return api.NewClient(token).DeleteApp(slug)
		},
		ReadInput: readInput,
	}
}

var deps = defaultDeps()
var yesFlag bool

// NewCmd returns the destroy command.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "destroy [slug]",
		Short: "Permanently delete an application",
		Long:  "Permanently delete a Hatch application. This action cannot be undone. Requires typing the app name to confirm (unless --yes is provided).",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runDestroy,
	}
	cmd.Flags().BoolVarP(&yesFlag, "yes", "y", false, "Skip confirmation prompt")
	return cmd
}

func runDestroy(cmd *cobra.Command, args []string) error {
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

	// Confirmation: either type app name or use --yes flag
	if !yesFlag {
		ui.Warn(fmt.Sprintf("This will permanently delete %s and all its data.", ui.Bold(slug)))
		fmt.Println()

		answer, err := deps.ReadInput(fmt.Sprintf("Type %q to confirm: ", slug))
		if err != nil {
			return fmt.Errorf("reading input: %w", err)
		}
		if strings.TrimSpace(answer) != slug {
			ui.Info("Cancelled. App name did not match.")
			return nil
		}
	}

	sp := ui.NewSpinner(fmt.Sprintf("Destroying %s...", slug))
	sp.Start()
	err = deps.DeleteApp(token, slug)
	sp.Stop()

	if err != nil {
		return fmt.Errorf("deleting app: %w", err)
	}

	ui.Success(fmt.Sprintf("Destroyed %s", slug))
	return nil
}

func resolveSlug(args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}
	if !deps.HasRemote("hatch") {
		return "", fmt.Errorf("no app specified and no hatch git remote found. Usage: hatch destroy <slug>")
	}
	url, err := deps.GetRemoteURL("hatch")
	if err != nil {
		return "", fmt.Errorf("reading hatch remote: %w", err)
	}
	return api.SlugFromRemote(url)
}

func readInput(prompt string) (string, error) {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	return reader.ReadString('\n')
}
