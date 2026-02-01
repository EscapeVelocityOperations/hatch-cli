package logs

import (
	"fmt"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/auth"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/git"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/ui"
	"github.com/spf13/cobra"
)

var (
	tail   int
	follow bool
)

// Deps holds injectable dependencies for testing.
type Deps struct {
	GetToken     func() (string, error)
	HasRemote    func(name string) bool
	GetRemoteURL func(name string) (string, error)
	StreamLogs   func(token, slug string, tail int, follow bool, handler func(string)) error
}

func defaultDeps() *Deps {
	return &Deps{
		GetToken:     auth.GetToken,
		HasRemote:    git.HasRemote,
		GetRemoteURL: git.GetRemoteURL,
		StreamLogs: func(token, slug string, tail int, follow bool, handler func(string)) error {
			return api.NewClient(token).StreamLogs(slug, tail, follow, handler)
		},
	}
}

var deps = defaultDeps()

// NewCmd returns the logs command.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs [slug]",
		Short: "View application logs",
		Long:  "Stream logs from a Hatch application. If no slug is provided, the app is detected from the current git remote.",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runLogs,
	}
	cmd.Flags().IntVar(&tail, "tail", 100, "number of recent log lines to show")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "follow log output")
	return cmd
}

func runLogs(cmd *cobra.Command, args []string) error {
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

	ui.Info(fmt.Sprintf("Streaming logs for %s...", ui.Bold(slug)))
	fmt.Println()

	return deps.StreamLogs(token, slug, tail, follow, func(line string) {
		fmt.Println(line)
	})
}

func resolveSlug(args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}
	// Auto-detect from git remote
	if !deps.HasRemote("hatch") {
		return "", fmt.Errorf("no app specified and no hatch git remote found. Usage: hatch logs <slug>")
	}
	url, err := deps.GetRemoteURL("hatch")
	if err != nil {
		return "", fmt.Errorf("reading hatch remote: %w", err)
	}
	return api.SlugFromRemote(url)
}
