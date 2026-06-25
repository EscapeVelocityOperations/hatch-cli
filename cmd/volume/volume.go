// Package volume implements `hatch volume` — persistent app volumes
// (h-gcf5h): enable, status, disable.
package volume

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/auth"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/ui"
	"github.com/spf13/cobra"
)

// VolumeInfo mirrors the API's volume status payload.
type VolumeInfo struct {
	SizeMB int
	UsedMB int
	Mount  string
	Status string // active | grace_deleting
}

// Deps are the injectable dependencies, following the domain cmd pattern.
type Deps struct {
	GetToken      func() (string, error)
	EnableVolume  func(token, slug string, sizeMB int) error
	GetVolume     func(token, slug string) (VolumeInfo, error)
	DisableVolume func(token, slug string, now bool) error
}

var deps = defaultDeps()

func defaultDeps() *Deps {
	return &Deps{
		GetToken: auth.GetToken,
		EnableVolume: func(token, slug string, sizeMB int) error {
			return api.NewClient(token).EnableVolume(slug, sizeMB)
		},
		GetVolume: func(token, slug string) (VolumeInfo, error) {
			v, err := api.NewClient(token).GetVolume(slug)
			if err != nil {
				return VolumeInfo{}, err
			}
			return VolumeInfo{SizeMB: v.SizeMB, UsedMB: v.UsedMB, Mount: v.Mount, Status: v.Status}, nil
		},
		DisableVolume: func(token, slug string, now bool) error {
			return api.NewClient(token).DisableVolume(slug, now)
		},
	}
}

var appSlug string

// NewCmd returns the `hatch volume` command group.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "volume",
		Short: "Manage the app's persistent volume (mounted at /data)",
	}
	cmd.PersistentFlags().StringVarP(&appSlug, "app", "a", "", "egg slug (auto-detected from .hatch.toml if omitted)")

	enable := &cobra.Command{
		Use:   "enable [slug]",
		Short: "Provision a persistent volume for the app",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			size, _ := cmd.Flags().GetInt("size")
			slug, err := resolveSlug(args)
			if err != nil {
				return err
			}
			return runEnable(slug, size)
		},
	}
	enable.Flags().Int("size", 1024, "volume size in MB")

	status := &cobra.Command{
		Use:   "status [slug]",
		Short: "Show volume size, usage and mount point",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug, err := resolveSlug(args)
			if err != nil {
				return err
			}
			return runStatus(slug)
		},
	}

	disable := &cobra.Command{
		Use:   "disable [slug]",
		Short: "Detach the volume and delete it after a 7-day grace period",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			now, _ := cmd.Flags().GetBool("now")
			slug, err := resolveSlug(args)
			if err != nil {
				return err
			}
			return runDisable(slug, now)
		},
	}
	disable.Flags().Bool("now", false, "delete immediately and irreversibly (skips the grace period)")

	cmd.AddCommand(enable, status, disable)
	return cmd
}

// resolveSlug picks the egg slug: positional arg > --app flag > .hatch.toml.
func resolveSlug(args []string) (string, error) {
	if len(args) > 0 && args[0] != "" {
		return args[0], nil
	}
	if appSlug != "" {
		return appSlug, nil
	}
	var config struct {
		Slug string `toml:"slug"`
		App  struct {
			Slug string `toml:"slug"`
		} `toml:"app"`
	}
	if data, err := os.ReadFile(filepath.Join(".", ".hatch.toml")); err == nil {
		if err := toml.Unmarshal(data, &config); err == nil {
			if config.Slug != "" {
				return config.Slug, nil
			}
			if config.App.Slug != "" {
				return config.App.Slug, nil
			}
		}
	}
	return "", fmt.Errorf("no egg specified: pass a slug, use --app, or run from a directory with .hatch.toml")
}

func authToken() (string, error) {
	token, err := deps.GetToken()
	if err != nil || token == "" {
		return "", fmt.Errorf("not logged in. Run 'hatch login', set HATCH_TOKEN, or use --token")
	}
	return token, nil
}

func runEnable(slug string, sizeMB int) error {
	token, err := authToken()
	if err != nil {
		return err
	}
	if err := deps.EnableVolume(token, slug, sizeMB); err != nil {
		return fmt.Errorf("enabling volume for %s: %w", slug, err)
	}
	ui.Success(fmt.Sprintf("Volume enabled for %s (%d MB, mounted at /data on next deploy)", slug, sizeMB))
	return nil
}

func runStatus(slug string) error {
	token, err := authToken()
	if err != nil {
		return err
	}
	v, err := deps.GetVolume(token, slug)
	if err != nil {
		return fmt.Errorf("fetching volume for %s: %w", slug, err)
	}
	fmt.Printf("Volume for %s\n", slug)
	fmt.Printf("  Size:   %d MB\n", v.SizeMB)
	fmt.Printf("  Used:   %d MB\n", v.UsedMB)
	fmt.Printf("  Mount:  %s\n", v.Mount)
	fmt.Printf("  Status: %s\n", v.Status)
	return nil
}

func runDisable(slug string, now bool) error {
	token, err := authToken()
	if err != nil {
		return err
	}
	if err := deps.DisableVolume(token, slug, now); err != nil {
		return fmt.Errorf("disabling volume for %s: %w", slug, err)
	}
	if now {
		ui.Success(fmt.Sprintf("Volume for %s deleted immediately", slug))
	} else {
		ui.Success(fmt.Sprintf("Volume for %s detached — data is deleted after the 7-day grace period", slug))
	}
	return nil
}
