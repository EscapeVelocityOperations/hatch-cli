package domain

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
	GetToken     func() (string, error)
	ListDomains  func(token, slug string) ([]api.Domain, error)
	AddDomain    func(token, slug, domain string) (*api.Domain, error)
	RemoveDomain func(token, slug, domain string) error
}

func defaultDeps() *Deps {
	return &Deps{
		GetToken: auth.GetToken,
		ListDomains: func(token, slug string) ([]api.Domain, error) {
			return api.NewClient(token).ListDomains(slug)
		},
		AddDomain: func(token, slug, domain string) (*api.Domain, error) {
			return api.NewClient(token).AddDomain(slug, domain)
		},
		RemoveDomain: func(token, slug, domain string) error {
			return api.NewClient(token).RemoveDomain(slug, domain)
		},
	}
}

var deps = defaultDeps()

// NewCmd returns the domain command with subcommands.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "domain",
		Short: "Manage custom domains for your eggs",
		Long: `Add, list, and remove custom domains for Hatch eggs.

After adding a domain, configure your DNS provider with a CNAME record
pointing to your egg's hosted URL:

  Type   Name   Value
  CNAME  @      <slug>.nest.gethatch.eu
  CNAME  www    <slug>.nest.gethatch.eu

Replace <slug> with your egg's slug (shown after "hatch domain add").

For apex domains (e.g. example.com), use an ALIAS or ANAME record if your
DNS provider supports it. Otherwise, use a subdomain (www.example.com).

SSL certificates are provisioned automatically via Let's Encrypt.`,
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newAddCmd())
	cmd.AddCommand(newRemoveCmd())

	return cmd
}

func newListCmd() *cobra.Command {
	var appSlug string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List custom domains",
		Long: `List all custom domains for a egg.

Example:
  hatch domain list --app my-app`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(appSlug)
		},
	}

	cmd.Flags().StringVarP(&appSlug, "app", "a", "", "Egg slug (required)")
	cmd.MarkFlagRequired("app")

	return cmd
}

func newAddCmd() *cobra.Command {
	var appSlug string

	cmd := &cobra.Command{
		Use:   "add <domain>",
		Short: "Add a custom domain",
		Long: `Add a custom domain to a egg.

After adding the domain, create a CNAME record at your DNS provider:

  CNAME  your-domain.com  →  <slug>.nest.gethatch.eu

SSL is provisioned automatically once DNS propagates (usually a few minutes).

Example:
  hatch domain add example.com --app my-app
  hatch domain add www.example.com --app my-app`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAdd(appSlug, args[0])
		},
	}

	cmd.Flags().StringVarP(&appSlug, "app", "a", "", "Egg slug (required)")
	cmd.MarkFlagRequired("app")

	return cmd
}

func newRemoveCmd() *cobra.Command {
	var appSlug string

	cmd := &cobra.Command{
		Use:   "remove <domain>",
		Short: "Remove a custom domain",
		Long: `Remove a custom domain from a egg.

Example:
  hatch domain remove example.com --app my-app`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRemove(appSlug, args[0])
		},
	}

	cmd.Flags().StringVarP(&appSlug, "app", "a", "", "Egg slug (required)")
	cmd.MarkFlagRequired("app")

	return cmd
}

// resolveSlug resolves an app name to its slug by listing apps.
// Returns the slug unchanged if it's already a valid slug or no match found.
func resolveSlug(appSlug string) (string, error) {
	token, err := deps.GetToken()
	if err != nil {
		return "", fmt.Errorf("checking auth: %w", err)
	}
	if token == "" {
		return "", fmt.Errorf("not logged in. Run 'hatch login', set HATCH_TOKEN, or use --token")
	}

	client := api.NewClient(token)
	apps, err := client.ListApps()
	if err == nil {
		// Check if appSlug matches any app name or slug
		for _, app := range apps {
			if app.Name == appSlug || app.Slug == appSlug {
				return app.Slug, nil
			}
		}
	}
	// Fall back to using it directly as slug
	return appSlug, nil
}

func runList(appSlug string) error {
	slug, err := resolveSlug(appSlug)
	if err != nil {
		return err
	}

	token, err := deps.GetToken()
	if err != nil {
		return fmt.Errorf("checking auth: %w", err)
	}
	if token == "" {
		return fmt.Errorf("not logged in. Run 'hatch login', set HATCH_TOKEN, or use --token")
	}

	sp := ui.NewSpinner("Fetching domains...")
	sp.Start()
	domains, err := deps.ListDomains(token, slug)
	sp.Stop()

	if err != nil {
		return fmt.Errorf("fetching domains: %w", err)
	}

	if len(domains) == 0 {
		ui.Info(fmt.Sprintf("No custom domains configured for '%s'.", slug))
		return nil
	}

	table := ui.NewTable(os.Stdout, "DOMAIN", "STATUS", "CNAME")
	for _, d := range domains {
		table.AddRow(d.Domain, statusColor(d.Status), d.CNAME)
	}
	table.Render()
	return nil
}

func runAdd(appSlug, domain string) error {
	slug, err := resolveSlug(appSlug)
	if err != nil {
		return err
	}

	token, err := deps.GetToken()
	if err != nil {
		return fmt.Errorf("checking auth: %w", err)
	}
	if token == "" {
		return fmt.Errorf("not logged in. Run 'hatch login', set HATCH_TOKEN, or use --token")
	}

	sp := ui.NewSpinner("Adding domain...")
	sp.Start()
	d, err := deps.AddDomain(token, slug, domain)
	sp.Stop()

	if err != nil {
		return fmt.Errorf("adding domain: %w", err)
	}

	cname := d.CNAME
	if cname == "" {
		cname = slug + ".nest.gethatch.eu"
	}

	fmt.Println()
	ui.Success(fmt.Sprintf("Domain '%s' added to '%s'", d.Domain, slug))
	fmt.Printf("  %s Create a CNAME record pointing to: %s\n", ui.Dim("→"), ui.Bold(cname))
	fmt.Printf("  %s For apex domains, use ALIAS/ANAME if your provider supports it.\n", ui.Dim("→"))
	fmt.Printf("  %s SSL will be provisioned automatically once DNS propagates.\n", ui.Dim("→"))
	fmt.Println()
	fmt.Printf("  %s Verify DNS with: %s\n", ui.Dim("→"), ui.Bold(fmt.Sprintf("dig +short CNAME %s", domain)))
	fmt.Println()

	return nil
}

func runRemove(appSlug, domain string) error {
	slug, err := resolveSlug(appSlug)
	if err != nil {
		return err
	}

	token, err := deps.GetToken()
	if err != nil {
		return fmt.Errorf("checking auth: %w", err)
	}
	if token == "" {
		return fmt.Errorf("not logged in. Run 'hatch login', set HATCH_TOKEN, or use --token")
	}

	sp := ui.NewSpinner("Removing domain...")
	sp.Start()
	err = deps.RemoveDomain(token, slug, domain)
	sp.Stop()

	if err != nil {
		return fmt.Errorf("removing domain: %w", err)
	}

	ui.Success(fmt.Sprintf("Domain '%s' removed from '%s'", domain, slug))
	return nil
}

func statusColor(status string) string {
	switch status {
	case "active", "verified":
		return ui.Green(status)
	case "pending":
		return ui.Yellow(status)
	case "error", "failed":
		return ui.Red(status)
	default:
		return status
	}
}
