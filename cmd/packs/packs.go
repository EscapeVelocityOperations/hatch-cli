package packs

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/auth"
	"github.com/spf13/cobra"
)

// NewCmd returns the `packs` command group for buying and listing non-expiring
// energy packs.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "packs",
		Short: "Buy and list non-expiring energy packs",
		Long: `Energy packs are a one-time purchase of non-expiring minutes for when you
exceed the free tier but don't want an always-on subscription.

  hatch packs buy [size]   Buy a pack (opens Stripe checkout in your browser)
  hatch packs list         Show your pack balance and purchase history`,
	}
	cmd.AddCommand(newBuyCmd())
	cmd.AddCommand(newListCmd())
	return cmd
}

func newBuyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "buy [size]",
		Short: "Buy an energy pack (opens Stripe checkout in your browser)",
		Long:  "Buy an energy pack. Size defaults to the standard pack. Opens a Stripe checkout page; minutes are credited once payment confirms.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			token, err := auth.GetToken()
			if err != nil {
				return fmt.Errorf("not logged in: %w (run 'hatch login' first)", err)
			}

			size := "standard"
			if len(args) == 1 {
				size = args[0]
			}

			client := api.NewClient(token)

			fmt.Fprintf(os.Stderr, "  Creating energy pack checkout (%s)...\n", size)

			res, err := client.PacksCheckout(size)
			if err != nil {
				return fmt.Errorf("creating pack checkout: %w", err)
			}

			fmt.Fprintf(os.Stderr, "  Opening checkout in browser (%d min for €%s)...\n", res.Minutes, res.AmountEur)

			if err := openBrowser(res.CheckoutURL); err != nil {
				fmt.Fprintf(os.Stderr, "\n  Could not open browser. Visit this URL to complete payment:\n")
				fmt.Fprintf(os.Stderr, "  %s\n\n", res.CheckoutURL)
				return nil
			}

			fmt.Fprintf(os.Stderr, "  Complete payment in your browser. Your minutes are credited once payment confirms.\n")
			return nil
		},
	}
}

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Show your energy pack balance and purchase history",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			token, err := auth.GetToken()
			if err != nil {
				return fmt.Errorf("not logged in: %w (run 'hatch login' first)", err)
			}

			client := api.NewClient(token)

			res, err := client.ListPacks()
			if err != nil {
				return fmt.Errorf("listing packs: %w", err)
			}

			fmt.Fprintf(os.Stderr, "\n")
			fmt.Fprintf(os.Stderr, "  Energy Packs\n")
			fmt.Fprintf(os.Stderr, "  ─────────────────────────────\n")
			fmt.Fprintf(os.Stderr, "  Balance: %d non-expiring minutes\n\n", res.PackMinutes)

			if len(res.Purchases) == 0 {
				fmt.Fprintf(os.Stderr, "  No purchases yet. Run 'hatch packs buy' to add minutes.\n\n")
				return nil
			}

			for _, p := range res.Purchases {
				fmt.Fprintf(os.Stderr, "  %s  %5d min  €%5.2f  %-9s\n",
					p.CreatedAt, p.Minutes, float64(p.AmountCents)/100, p.Status)
			}
			fmt.Fprintf(os.Stderr, "\n")
			return nil
		},
	}
}

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "linux":
		return exec.Command("xdg-open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return fmt.Errorf("unsupported platform")
	}
}
