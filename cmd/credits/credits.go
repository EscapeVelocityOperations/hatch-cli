package credits

import (
	"fmt"
	"os"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/auth"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/resolve"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/ui"
	"github.com/spf13/cobra"
)

func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "credits",
		Short: "Manage boost credits",
		Long: `View and apply admin-granted boost credits to your eggs.

Boost credits are granted by administrators and can be applied to any of your eggs
to give them temporary boost status without payment.`,
		RunE: listCredits,
	}
	cmd.AddCommand(newApplyCmd())
	return cmd
}

func listCredits(cmd *cobra.Command, args []string) error {
	token, err := auth.GetToken()
	if err != nil {
		return fmt.Errorf("not logged in: %w (run 'hatch login' first)", err)
	}

	client := api.NewClient(token)

	sp := ui.NewSpinner("Fetching boost credits...")
	sp.Start()
	credits, err := client.ListBoostCredits()
	sp.Stop()

	if err != nil {
		return fmt.Errorf("fetching boost credits: %w", err)
	}

	if len(credits.Credits) == 0 {
		ui.Info("No boost credits available.")
		return nil
	}

	fmt.Println()
	fmt.Println(ui.Bold("Boost Credits"))
	fmt.Printf("  Day:  %d\n", credits.DayCredits)
	fmt.Printf("  Week: %d\n", credits.WeekCredits)
	fmt.Println()

	table := ui.NewTable(os.Stdout, "ID", "TYPE", "GRANTED")
	for _, c := range credits.Credits {
		table.AddRow(c.ID, c.Type, c.GrantedAt)
	}
	table.Render()

	return nil
}

func newApplyCmd() *cobra.Command {
	var creditType string

	cmd := &cobra.Command{
		Use:   "apply [slug]",
		Short: "Apply a boost credit to an egg",
		Long: `Apply an admin-granted boost credit to an egg.

If no slug is provided, the egg is detected from .hatch.toml.
If you have multiple credits and don't specify --type, the first available credit is used.

Examples:
  hatch credits apply              # Apply to egg in .hatch.toml
  hatch credits apply my-app       # Apply to specific egg
  hatch credits apply --type day   # Apply a day credit (if available)
  hatch credits apply --type week  # Apply a week credit (if available)`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runApply(args, creditType)
		},
	}

	cmd.Flags().StringVar(&creditType, "type", "", "Credit type to apply (day or week)")

	return cmd
}

func runApply(args []string, creditType string) error {
	token, err := auth.GetToken()
	if err != nil {
		return fmt.Errorf("not logged in: %w (run 'hatch login' first)", err)
	}

	// Resolve slug
	var slug string
	if len(args) > 0 {
		slug = args[0]
	} else {
		slug = resolve.SlugFromToml()
		if slug == "" {
			return fmt.Errorf("no egg specified. Provide a slug or add a .hatch.toml.\nUsage: hatch credits apply <slug>")
		}
	}

	client := api.NewClient(token)

	// Fetch available credits
	sp := ui.NewSpinner("Fetching available credits...")
	sp.Start()
	credits, err := client.ListBoostCredits()
	sp.Stop()

	if err != nil {
		return fmt.Errorf("fetching boost credits: %w", err)
	}

	if len(credits.Credits) == 0 {
		return fmt.Errorf("no boost credits available")
	}

	// Select credit to use
	var selectedCredit *api.BoostCredit
	if creditType != "" {
		// Filter by type
		for _, c := range credits.Credits {
			if c.Type == creditType {
				selectedCredit = &c
				break
			}
		}
		if selectedCredit == nil {
			return fmt.Errorf("no %s credits available", creditType)
		}
	} else {
		// Use first available credit
		selectedCredit = &credits.Credits[0]
	}

	// Redeem the credit
	sp = ui.NewSpinner(fmt.Sprintf("Applying %s boost credit to %s...", selectedCredit.Type, slug))
	sp.Start()
	result, err := client.RedeemBoostCredit(selectedCredit.ID, slug)
	sp.Stop()

	if err != nil {
		return fmt.Errorf("applying boost credit: %w", err)
	}

	ui.Success(fmt.Sprintf("Applied %s boost credit to %s", result.Type, result.EggSlug))
	fmt.Printf("  Boosted until: %s\n", result.BoostExpiresAt)
	fmt.Println()

	return nil
}
