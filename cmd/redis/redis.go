package redis

import (
	"fmt"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/auth"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/resolve"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/ui"
	"github.com/spf13/cobra"
)

// NewCmd returns the redis command group.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "redis",
		Short: "Redis cache management commands",
	}
	cmd.AddCommand(newAddCmd())
	cmd.AddCommand(newInfoCmd())
	cmd.AddCommand(newRemoveCmd())
	return cmd
}

func resolveSlug(args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}
	if slug := resolve.SlugFromToml(); slug != "" {
		return slug, nil
	}
	return "", fmt.Errorf("no egg specified. Usage: hatch redis <command> <slug>")
}

func newAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add [slug]",
		Short: "Add a Redis cache sidecar to your egg",
		Long: `Provisions an ephemeral Redis cache that runs alongside your egg.

The Redis sidecar shares the same network as your app, accessible at localhost:6379.
REDIS_URL is automatically set in your environment.

Limits: 25 MB memory, 50 max connections, no persistence (data lost on restart).`,
		Args: cobra.MaximumNArgs(1),
		RunE: runAdd,
	}
}

func runAdd(cmd *cobra.Command, args []string) error {
	token, err := auth.GetToken()
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

	ui.Info(fmt.Sprintf("Adding Redis cache to %s...", ui.Bold(slug)))

	client := api.NewClient(token)
	addon, err := client.AddAddon(slug, "redis")
	if err != nil {
		return fmt.Errorf("adding redis: %w", err)
	}

	if addon.Status == "active" {
		ui.Success("Redis cache ready!")
		ui.Info("REDIS_URL has been set automatically.")
		ui.Info("Your app can connect to redis://localhost:6379")
		fmt.Println()
		ui.Info("Limits: 25 MB memory, 50 connections, no persistence")
		ui.Info("Note: Redis data is ephemeral — lost on egg restart.")
		ui.Info("Redeploy to activate: hatch deploy (from your project directory)")
	} else {
		ui.Warn(fmt.Sprintf("Redis status: %s", addon.Status))
	}

	return nil
}

func newInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info [slug]",
		Short: "Show Redis addon status for your egg",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runInfo,
	}
}

func runInfo(cmd *cobra.Command, args []string) error {
	token, err := auth.GetToken()
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

	client := api.NewClient(token)
	redisURL, status, err := client.GetRedisURL(slug)
	if err != nil {
		return fmt.Errorf("no redis addon for %s. Run: hatch redis add %s", slug, slug)
	}

	fmt.Printf("Redis for %s\n", ui.Bold(slug))
	fmt.Printf("  Status:    %s\n", status)
	fmt.Printf("  URL:       %s\n", redisURL)
	fmt.Printf("  Memory:    25 MB max (allkeys-lru eviction)\n")
	fmt.Printf("  Clients:   50 max connections\n")
	fmt.Printf("  Persist:   disabled (ephemeral)\n")

	return nil
}

func newRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove [slug]",
		Short: "Remove Redis cache from your egg (coming soon)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ui.Warn("Redis removal is not yet supported. Contact support if needed.")
			return nil
		},
	}
}
