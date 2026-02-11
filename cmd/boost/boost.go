package boost

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/auth"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/resolve"
	"github.com/spf13/cobra"
)

func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "boost [slug] [day|week]",
		Short: "Boost an egg with extra energy",
		Long: `Boost an egg to keep it running without sleep restrictions.

If no slug is provided, the egg is detected from .hatch.toml or the current git remote.

Pricing:
  day   24 hours of boost for €1.50
  week  7 days of boost for €4

This opens a Stripe checkout page in your browser to complete payment.`,
		Args: cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			token, err := auth.GetToken()
			if err != nil {
				return fmt.Errorf("not logged in: %w (run 'hatch login' first)", err)
			}

			var slug string
			duration := "day"

			switch len(args) {
			case 0:
				// Resolve from .hatch.toml
				slug = resolve.SlugFromToml()
				if slug == "" {
					return fmt.Errorf("no egg specified. Provide a slug or add a .hatch.toml.\nUsage: hatch boost <slug> [day|week]")
				}
			case 1:
				// Could be slug or duration
				if args[0] == "day" || args[0] == "week" {
					slug = resolve.SlugFromToml()
					if slug == "" {
						return fmt.Errorf("no egg specified. Provide a slug or add a .hatch.toml.\nUsage: hatch boost <slug> [day|week]")
					}
					duration = args[0]
				} else {
					slug = args[0]
				}
			case 2:
				slug = args[0]
				duration = args[1]
			}

			if duration != "day" && duration != "week" {
				return fmt.Errorf("invalid duration %q: must be 'day' or 'week'", duration)
			}

			client := api.NewClient(token)

			fmt.Fprintf(os.Stderr, "  Creating boost checkout for %s (%s)...\n", slug, duration)

			result, err := client.BoostCheckout(slug, duration)
			if err != nil {
				return fmt.Errorf("creating boost checkout: %w", err)
			}

			fmt.Fprintf(os.Stderr, "  Opening checkout in browser (€%s for %s boost)...\n",
				result.AmountEur, result.Duration)

			if err := openBrowser(result.CheckoutURL); err != nil {
				// If browser fails, print URL for manual use
				fmt.Fprintf(os.Stderr, "\n  Could not open browser. Visit this URL to complete payment:\n")
				fmt.Fprintf(os.Stderr, "  %s\n\n", result.CheckoutURL)
				return nil
			}

			fmt.Fprintf(os.Stderr, "  Complete payment in your browser. Your egg will be boosted once payment confirms.\n")
			return nil
		},
	}
	return cmd
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
