package apps

import (
	"fmt"
	"os"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/auth"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/ui"
	"github.com/spf13/cobra"
)

// Deps holds injectable dependencies for testing.
type Deps struct {
	GetToken func() (string, error)
	ListApps func(token string) ([]api.App, error)
	GetApp   func(token, slug string) (*api.App, error)
}

func defaultDeps() *Deps {
	return &Deps{
		GetToken: auth.GetToken,
		ListApps: func(token string) ([]api.App, error) {
			return api.NewClient(token).ListApps()
		},
		GetApp: func(token, slug string) (*api.App, error) {
			return api.NewClient(token).GetApp(slug)
		},
	}
}

var deps = defaultDeps()

// NewCmd returns the apps command with info subcommand.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "eggs",
		Short: "List your Hatch eggs",
		Long:  "Display a list of all eggs deployed to the Hatch platform.",
		RunE:  runList,
	}
	return cmd
}

// NewInfoCmd returns the info command.
func NewInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info [slug]",
		Short: "Show details for an egg",
		Long:  "Display detailed information about a specific Hatch egg.",
		Args:  cobra.ExactArgs(1),
		RunE:  runInfo,
	}
}

func runList(cmd *cobra.Command, args []string) error {
	token, err := deps.GetToken()
	if err != nil {
		return fmt.Errorf("checking auth: %w", err)
	}
	if token == "" {
		return fmt.Errorf("not logged in. Run 'hatch login', set HATCH_TOKEN, or use --token")
	}

	sp := ui.NewSpinner("Fetching eggs...")
	sp.Start()
	appList, err := deps.ListApps(token)
	sp.Stop()

	if err != nil {
		return fmt.Errorf("fetching eggs: %w", err)
	}

	if len(appList) == 0 {
		ui.Info("No eggs found. Deploy one with 'hatch deploy'.")
		return nil
	}

	table := ui.NewTable(os.Stdout, "SLUG", "NAME", "STATUS", "URL")
	for _, a := range appList {
		url := a.URL
		if url == "" {
			url = "https://" + a.Slug + ".nest.gethatch.eu"
		}
		table.AddRow(a.Slug, a.Name, statusColor(a.Status), url)
	}
	table.Render()
	return nil
}

func runInfo(cmd *cobra.Command, args []string) error {
	token, err := deps.GetToken()
	if err != nil {
		return fmt.Errorf("checking auth: %w", err)
	}
	if token == "" {
		return fmt.Errorf("not logged in. Run 'hatch login', set HATCH_TOKEN, or use --token")
	}

	slug := args[0]

	sp := ui.NewSpinner("Fetching egg details...")
	sp.Start()
	app, err := deps.GetApp(token, slug)
	sp.Stop()

	if err != nil {
		return fmt.Errorf("fetching egg: %w", err)
	}

	fmt.Println()
	fmt.Printf("  %s\n", ui.Bold(app.Name))
	fmt.Printf("  %s %s\n", ui.Dim("Slug:"), app.Slug)
	fmt.Printf("  %s %s\n", ui.Dim("Status:"), statusColor(app.Status))
	fmt.Printf("  %s %s\n", ui.Dim("URL:"), app.URL)
	fmt.Printf("  %s %s\n", ui.Dim("Region:"), app.Region)
	fmt.Printf("  %s %s\n", ui.Dim("Created:"), app.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("  %s %s\n", ui.Dim("Updated:"), app.UpdatedAt.Format("2006-01-02 15:04:05"))
	fmt.Println()

	return nil
}

func statusColor(status string) string {
	switch status {
	case "running":
		return ui.Green(status)
	case "stopped", "crashed":
		return ui.Red(status)
	case "deploying":
		return ui.Yellow(status)
	default:
		return status
	}
}
