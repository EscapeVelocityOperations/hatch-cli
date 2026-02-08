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
	lines  int
	follow bool
	build  bool
)

// Deps holds injectable dependencies for testing.
type Deps struct {
	GetToken     func() (string, error)
	HasRemote    func(name string) bool
	GetRemoteURL func(name string) (string, error)
	StreamLogs   func(token, slug string, lines int, follow bool, logType string, handler func(string)) error
}

func defaultDeps() *Deps {
	return &Deps{
		GetToken:     auth.GetToken,
		HasRemote:    git.HasRemote,
		GetRemoteURL: git.GetRemoteURL,
		StreamLogs: func(token, slug string, lines int, follow bool, logType string, handler func(string)) error {
			return api.NewClient(token).StreamLogs(slug, lines, follow, logType, handler)
		},
	}
}

var deps = defaultDeps()

// NewCmd returns the logs command.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs [slug]",
		Short: "View egg logs",
		Long:  "Stream logs from a Hatch egg. If no slug is provided, the egg is detected from the current git remote.",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runLogs,
	}
	cmd.Flags().IntVarP(&lines, "lines", "n", 100, "number of recent log lines to show")
	cmd.Flags().BoolVarP(&follow, "follow", "f", true, "follow log output (live tail)")
	cmd.Flags().BoolVar(&build, "build", false, "show build logs instead of runtime logs")
	return cmd
}

func runLogs(cmd *cobra.Command, args []string) error {
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

	logType := ""
	if build {
		logType = "build"
		ui.Info(fmt.Sprintf("Streaming build logs for %s...", ui.Bold(slug)))
	} else {
		ui.Info(fmt.Sprintf("Streaming logs for %s...", ui.Bold(slug)))
	}
	fmt.Println()

	return deps.StreamLogs(token, slug, lines, follow, logType, func(line string) {
		fmt.Println(line)
	})
}

func resolveSlug(args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}
	// Auto-detect from git remote
	if !deps.HasRemote("hatch") {
		return "", fmt.Errorf("no egg specified and no hatch git remote found. Usage: hatch logs <slug>")
	}
	url, err := deps.GetRemoteURL("hatch")
	if err != nil {
		return "", fmt.Errorf("reading hatch remote: %w", err)
	}
	return api.SlugFromRemote(url)
}
