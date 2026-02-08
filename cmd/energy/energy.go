package energy

import (
	"fmt"
	"os"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/auth"
	"github.com/spf13/cobra"
)

func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "energy [slug]",
		Short: "Show energy status",
		Long:  "Show energy status for your account or a specific egg.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			token, err := auth.GetToken()
			if err != nil {
				return fmt.Errorf("not logged in: %w (run 'hatch login' first)", err)
			}

			client := api.NewClient(token)

			if len(args) == 1 {
				return showAppEnergy(client, args[0])
			}
			return showAccountEnergy(client)
		},
	}
	return cmd
}

func showAccountEnergy(client *api.Client) error {
	energy, err := client.GetAccountEnergy()
	if err != nil {
		return fmt.Errorf("getting energy status: %w", err)
	}

	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  Energy Status (%s tier)\n", energy.Tier)
	fmt.Fprintf(os.Stderr, "  ─────────────────────────────\n")
	fmt.Fprintf(os.Stderr, "  Daily:   %d/%d min remaining\n", energy.DailyRemaining, energy.DailyLimit)
	fmt.Fprintf(os.Stderr, "  Weekly:  %d/%d min remaining\n", energy.WeeklyRemaining, energy.WeeklyLimit)
	fmt.Fprintf(os.Stderr, "  Resets:  %s\n", energy.ResetsAt)
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  Eggs:    %d active, %d sleeping (limit: %d)\n",
		energy.EggsActive, energy.EggsSleeping, energy.EggsLimit)

	if len(energy.AlwaysOnEggs) > 0 {
		fmt.Fprintf(os.Stderr, "  Always-on: %v\n", energy.AlwaysOnEggs)
	}
	if len(energy.BoostedEggs) > 0 {
		fmt.Fprintf(os.Stderr, "  Boosted:   %v\n", energy.BoostedEggs)
	}
	fmt.Fprintf(os.Stderr, "\n")
	return nil
}

func showAppEnergy(client *api.Client, slug string) error {
	energy, err := client.GetAppEnergy(slug)
	if err != nil {
		return fmt.Errorf("getting energy for %s: %w", slug, err)
	}

	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  Energy: %s (%s)\n", energy.Slug, energy.Status)
	fmt.Fprintf(os.Stderr, "  ─────────────────────────────\n")
	fmt.Fprintf(os.Stderr, "  Plan:    %s\n", energy.Plan)

	if energy.AlwaysOn {
		fmt.Fprintf(os.Stderr, "  Mode:    always-on (unlimited energy)\n")
	} else if energy.Boosted {
		fmt.Fprintf(os.Stderr, "  Mode:    boosted (until %s)\n", *energy.BoostExpiresAt)
	} else {
		fmt.Fprintf(os.Stderr, "  Mode:    free tier\n")
	}

	fmt.Fprintf(os.Stderr, "  Daily:   %d/%d min (%d used)\n",
		energy.DailyRemainingMin, energy.DailyLimitMin, energy.DailyUsedMin)
	fmt.Fprintf(os.Stderr, "  Weekly:  %d/%d min (%d used)\n",
		energy.WeeklyRemainingMin, energy.WeeklyLimitMin, energy.WeeklyUsedMin)
	fmt.Fprintf(os.Stderr, "  Resets:  daily %s, weekly %s\n",
		energy.DailyResetsAt, energy.WeeklyResetsAt)

	if energy.BonusEnergy > 0 {
		fmt.Fprintf(os.Stderr, "  Bonus:   %d min\n", energy.BonusEnergy)
	}
	fmt.Fprintf(os.Stderr, "\n")

	if !energy.AlwaysOn && !energy.Boosted && energy.DailyRemainingMin <= 30 {
		fmt.Fprintf(os.Stderr, "  Tip: Run 'hatch boost %s' to keep your egg running longer.\n\n", energy.Slug)
	}
	return nil
}
