// Package resources implements `hatch resources` (h-ek431): per-app overrides
// on top of the runtime resource profiles. `show` prints the app + override
// guidance; `set` overrides CPU/memory via PATCH /v1/apps/{slug}/resources.
package resources

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/auth"
)

// Deps are the injectable dependencies (mock-client pattern).
type Deps struct {
	GetToken     func() (string, error)
	GetApp       func(token, slug string) (*api.App, error)
	SetResources func(token, slug string, memoryMB, cpuMHz *int) (api.AppResources, error)
}

var deps = defaultDeps()

func defaultDeps() *Deps {
	return &Deps{
		GetToken: auth.GetToken,
		GetApp: func(token, slug string) (*api.App, error) {
			return api.NewClient(token).GetApp(slug)
		},
		SetResources: func(token, slug string, memoryMB, cpuMHz *int) (api.AppResources, error) {
			return api.NewClient(token).SetResources(slug, memoryMB, cpuMHz)
		},
	}
}

// NewCmd returns the `hatch resources` command group.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resources",
		Short: "View or override an app's CPU/memory resources",
	}
	cmd.AddCommand(newShowCmd(), newSetCmd())
	return cmd
}

func newShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <slug>",
		Short: "Show an app and its resource-override guidance",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runShow(args[0])
		},
	}
}

func newSetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set <slug>",
		Short: "Override an app's resources, e.g. `hatch resources set my-app --memory 768`",
		Long: "Override an app's CPU/memory on top of its runtime profile.\n" +
			"A flag you omit is cleared (the runtime profile/default applies);\n" +
			"pass both --memory and --cpu to set both.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var memoryMB, cpuMHz *int
			if cmd.Flags().Changed("memory") {
				v, _ := cmd.Flags().GetInt("memory")
				memoryMB = &v
			}
			if cmd.Flags().Changed("cpu") {
				v, _ := cmd.Flags().GetInt("cpu")
				cpuMHz = &v
			}
			if memoryMB == nil && cpuMHz == nil {
				return errors.New("provide --memory and/or --cpu")
			}
			return runSet(args[0], memoryMB, cpuMHz)
		},
	}
	cmd.Flags().Int("memory", 0, "memory override in MB (omit to clear)")
	cmd.Flags().Int("cpu", 0, "cpu override in MHz (omit to clear)")
	return cmd
}

func runShow(slug string) error {
	token, err := deps.GetToken()
	if err != nil || token == "" {
		return errors.New("not logged in — run `hatch login` first")
	}
	app, err := deps.GetApp(token, slug)
	if err != nil {
		return err
	}
	fmt.Printf("App: %s (%s)\n", app.Slug, app.Status)
	fmt.Println("  Resource limits resolve as: app override > runtime profile > default.")
	fmt.Printf("  Override:   hatch resources set %s --memory <MB> --cpu <MHz>\n", slug)
	fmt.Printf("  Live usage: hatch stats %s\n", slug)
	return nil
}

func runSet(slug string, memoryMB, cpuMHz *int) error {
	token, err := deps.GetToken()
	if err != nil || token == "" {
		return errors.New("not logged in — run `hatch login` first")
	}
	res, err := deps.SetResources(token, slug, memoryMB, cpuMHz)
	if err != nil {
		return err
	}
	fmt.Printf("Updated resources for %s:\n", slug)
	fmt.Printf("  memory_mb: %s\n", fmtOverride(res.MemoryMB))
	fmt.Printf("  cpu_mhz:   %s\n", fmtOverride(res.CPUMHz))
	return nil
}

// fmtOverride renders an override value, or "(default)" when cleared.
func fmtOverride(p *int) string {
	if p == nil {
		return "(default)"
	}
	return fmt.Sprintf("%d", *p)
}
