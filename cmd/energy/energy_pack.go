package energy

// Energy packs (feature h-vlmt8, spec h-avt0u): purchasable non-expiring
// minutes. `hatch energy` shows the pack bucket; `hatch energy buy` opens a
// Stripe checkout (mirrors cmd/boost's flow).

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/auth"
)

// Deps injects auth + API calls for tests (house pattern, see
// cmd/deploy/deploy_test.go).
type Deps struct {
	GetToken           func() (string, error)
	CreatePackCheckout func(token string) (*api.EnergyPackCheckoutResponse, error)
}

func defaultDeps() *Deps {
	return &Deps{
		GetToken: auth.GetToken,
		CreatePackCheckout: func(token string) (*api.EnergyPackCheckoutResponse, error) {
			return api.NewClient(token).EnergyPackCheckout()
		},
	}
}

var deps = defaultDeps()

// writeAccountEnergy renders the account energy summary — all four buckets
// (daily, weekly, bonus via the api response, pack) — to w.
// showAccountEnergy renders through this seam.
func writeAccountEnergy(w io.Writer, energy *api.EnergyStatus) {
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "  Energy Status\n")
	fmt.Fprintf(w, "  ─────────────────────────────\n")
	fmt.Fprintf(w, "  Daily:   %d/%d min remaining\n", energy.DailyRemaining, energy.DailyLimit)
	fmt.Fprintf(w, "  Weekly:  %d/%d min remaining\n", energy.WeeklyRemaining, energy.WeeklyLimit)
	fmt.Fprintf(w, "  Pack:    %d min (never expires)\n", energy.PackRemaining)
	fmt.Fprintf(w, "  Resets:  %s\n", energy.ResetsAt)
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "  Eggs:    %d active, %d sleeping (limit: %d)\n",
		energy.EggsActive, energy.EggsSleeping, energy.EggsLimit)

	if len(energy.AlwaysOnEggs) > 0 {
		fmt.Fprintf(w, "  Always-on: %v\n", energy.AlwaysOnEggs)
	}
	if len(energy.BoostedEggs) > 0 {
		fmt.Fprintf(w, "  Boosted:   %v\n", energy.BoostedEggs)
	}
	fmt.Fprintf(w, "\n")
}

// runBuy executes `hatch energy buy`: creates a pack checkout session and
// prints the URL (mirrors cmd/boost's flow).
func runBuy(cmd *cobra.Command, args []string) error {
	token, err := deps.GetToken()
	if err != nil {
		return fmt.Errorf("not logged in: %w (run 'hatch login' first)", err)
	}

	fmt.Fprintf(os.Stderr, "  Creating energy pack checkout...\n")

	result, err := deps.CreatePackCheckout(token)
	if err != nil {
		return fmt.Errorf("creating energy pack checkout: %w", err)
	}

	fmt.Fprintf(os.Stderr, "  Energy pack: %d min for €%s — minutes never expire.\n", result.Minutes, result.AmountEur)
	fmt.Fprintf(os.Stderr, "  Complete payment in your browser:\n")
	fmt.Fprintf(os.Stderr, "  %s\n\n", result.CheckoutURL)
	fmt.Fprintf(os.Stderr, "  Minutes are credited as soon as payment confirms.\n")
	return nil
}

func newBuyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "buy",
		Short: "Buy an energy pack (1000 min, non-expiring)",
		RunE:  runBuy,
	}
}
