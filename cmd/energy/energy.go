package energy

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
	"golang.org/x/term"
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
			if slug := resolve.SlugFromToml(); slug != "" {
				return showAppEnergy(client, slug)
			}
			return showAccountEnergy(client)
		},
	}
	return cmd
}

// pctOf returns the percentage of remaining/limit, safe for zero limit.
func pctOf(remaining, limit int) int {
	if limit <= 0 {
		return 100
	}
	return (remaining * 100) / limit
}

// bar renders a 20-char progress bar with color.
func bar(remaining, limit int) string {
	pct := pctOf(remaining, limit)
	filled := (pct * 20) / 100
	if filled > 20 {
		filled = 20
	}
	empty := 20 - filled
	b := strings.Repeat("█", filled) + strings.Repeat("░", empty)
	label := fmt.Sprintf(" %d%%", pct)
	if pct <= 10 {
		return ui.Red(b+label)
	}
	if pct <= 20 {
		return ui.Yellow(b+label)
	}
	return ui.Green(b+label)
}

func showAccountEnergy(client *api.Client) error {
	energy, err := client.GetAccountEnergy()
	if err != nil {
		return fmt.Errorf("getting energy status: %w", err)
	}

	dailyPct := pctOf(energy.DailyRemaining, energy.DailyLimit)
	weeklyPct := pctOf(energy.WeeklyRemaining, energy.WeeklyLimit)

	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  %s\n", ui.Bold("Energy Status"))
	fmt.Fprintf(os.Stderr, "  ─────────────────────────────────────────\n")
	fmt.Fprintf(os.Stderr, "  Daily:   %d/%d min remaining  %s\n",
		energy.DailyRemaining, energy.DailyLimit, bar(energy.DailyRemaining, energy.DailyLimit))
	fmt.Fprintf(os.Stderr, "  Weekly:  %d/%d min remaining  %s\n",
		energy.WeeklyRemaining, energy.WeeklyLimit, bar(energy.WeeklyRemaining, energy.WeeklyLimit))
	fmt.Fprintf(os.Stderr, "  Resets:  %s\n", energy.ResetsAt)
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  Eggs:    %d active, %d sleeping (limit: %d)\n",
		energy.EggsActive, energy.EggsSleeping, energy.EggsLimit)

	if len(energy.AlwaysOnEggs) > 0 {
		fmt.Fprintf(os.Stderr, "  Always-on: %s\n", strings.Join(energy.AlwaysOnEggs, ", "))
	}
	if len(energy.BoostedEggs) > 0 {
		fmt.Fprintf(os.Stderr, "  Boosted:   %s\n", strings.Join(energy.BoostedEggs, ", "))
	}
	fmt.Fprintf(os.Stderr, "\n")

	if dailyPct <= 10 || weeklyPct <= 10 {
		fmt.Fprintf(os.Stderr, "  %s Energy is critically low!\n", ui.Red("!!"))
		fmt.Fprintf(os.Stderr, "  Your free eggs will sleep when energy runs out.\n")
		fmt.Fprintf(os.Stderr, "  Run %s to boost a specific egg.\n\n", ui.Bold("hatch boost <slug>"))
	} else if dailyPct <= 20 || weeklyPct <= 20 {
		fmt.Fprintf(os.Stderr, "  %s Energy is running low.\n", ui.Yellow("!"))
		fmt.Fprintf(os.Stderr, "  Run %s to boost a specific egg.\n\n", ui.Bold("hatch boost <slug>"))
	}
	return nil
}

func showAppEnergy(client *api.Client, slug string) error {
	energy, err := client.GetAppEnergy(slug)
	if err != nil {
		return fmt.Errorf("getting energy for %s: %w", slug, err)
	}

	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  %s (%s)\n", ui.Bold("Energy: "+energy.Slug), energy.Status)
	fmt.Fprintf(os.Stderr, "  ─────────────────────────────────────────\n")
	fmt.Fprintf(os.Stderr, "  Tier:    %s\n", energy.Plan)

	if energy.AlwaysOn {
		fmt.Fprintf(os.Stderr, "  Mode:    %s\n", ui.Green("always-on (unlimited energy)"))
	} else if energy.Boosted {
		fmt.Fprintf(os.Stderr, "  Mode:    %s\n", ui.Blue("boosted (until "+*energy.BoostExpiresAt+")"))
	} else {
		fmt.Fprintf(os.Stderr, "  Mode:    free\n")
	}

	fmt.Fprintf(os.Stderr, "  Daily:   %d/%d min  %s\n",
		energy.DailyRemainingMin, energy.DailyLimitMin,
		bar(energy.DailyRemainingMin, energy.DailyLimitMin))
	fmt.Fprintf(os.Stderr, "  Weekly:  %d/%d min  %s\n",
		energy.WeeklyRemainingMin, energy.WeeklyLimitMin,
		bar(energy.WeeklyRemainingMin, energy.WeeklyLimitMin))
	fmt.Fprintf(os.Stderr, "  Resets:  daily %s, weekly %s\n",
		energy.DailyResetsAt, energy.WeeklyResetsAt)

	if energy.BonusEnergy > 0 {
		fmt.Fprintf(os.Stderr, "  Bonus:   %d min\n", energy.BonusEnergy)
	}
	fmt.Fprintf(os.Stderr, "\n")

	if energy.AlwaysOn || energy.Boosted {
		return nil
	}

	dailyPct := pctOf(energy.DailyRemainingMin, energy.DailyLimitMin)
	weeklyPct := pctOf(energy.WeeklyRemainingMin, energy.WeeklyLimitMin)

	if dailyPct <= 10 || weeklyPct <= 10 {
		fmt.Fprintf(os.Stderr, "  %s Energy is critically low for %s!\n", ui.Red("!!"), slug)
		fmt.Fprintf(os.Stderr, "  Your egg will sleep when energy runs out.\n\n")
		promptBoost(slug)
	} else if dailyPct <= 20 || weeklyPct <= 20 {
		fmt.Fprintf(os.Stderr, "  %s Energy is running low.\n", ui.Yellow("!"))
		fmt.Fprintf(os.Stderr, "  Run %s to keep your egg running.\n\n",
			ui.Bold("hatch boost "+slug))
	}
	return nil
}

// promptBoost interactively offers to boost an egg if running in a terminal.
func promptBoost(slug string) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprintf(os.Stderr, "  Run %s to keep your egg running.\n\n",
			ui.Bold("hatch boost "+slug))
		return
	}

	fmt.Fprintf(os.Stderr, "  Boost pricing:\n")
	fmt.Fprintf(os.Stderr, "    day   24h boost for %s\n", ui.Bold("EUR 1.50"))
	fmt.Fprintf(os.Stderr, "    week  7 days boost for %s\n\n", ui.Bold("EUR 4.00"))
	fmt.Fprintf(os.Stderr, "  Boost this egg? [day/week/N] ")

	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))

	if answer != "day" && answer != "week" {
		return
	}

	token, err := auth.GetToken()
	if err != nil {
		fmt.Fprintf(os.Stderr, "  %s\n", ui.Red("Not logged in — run 'hatch login' first."))
		return
	}

	client := api.NewClient(token)
	result, err := client.BoostCheckout(slug, answer)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  %s %v\n", ui.Red("Boost failed:"), err)
		return
	}

	fmt.Fprintf(os.Stderr, "\n  Opening checkout (EUR %s for %s boost)...\n", result.AmountEur, result.Duration)
	if err := openURL(result.CheckoutURL); err != nil {
		fmt.Fprintf(os.Stderr, "  Could not open browser. Visit:\n  %s\n\n", result.CheckoutURL)
	}
}
