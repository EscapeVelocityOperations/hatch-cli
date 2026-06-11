// Package volume implements `hatch volume` — persistent app volumes
// (h-gcf5h): enable, status, disable.
//
// TODO(h-g91eg): implement — stubs so the TDD-red tests (h-poo35) compile.
package volume

import (
	"errors"

	"github.com/spf13/cobra"
)

var errNotImplemented = errors.New("not implemented")

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
	return &Deps{}
}

// NewCmd returns the `hatch volume` command group.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "volume",
		Short: "Manage the app's persistent volume (mounted at /data)",
	}

	enable := &cobra.Command{
		Use:   "enable [slug]",
		Short: "Provision a persistent volume for the app",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			size, _ := cmd.Flags().GetInt("size")
			slug := ""
			if len(args) > 0 {
				slug = args[0]
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
			slug := ""
			if len(args) > 0 {
				slug = args[0]
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
			slug := ""
			if len(args) > 0 {
				slug = args[0]
			}
			return runDisable(slug, now)
		},
	}
	disable.Flags().Bool("now", false, "delete immediately and irreversibly (skips the grace period)")

	cmd.AddCommand(enable, status, disable)
	return cmd
}

func runEnable(slug string, sizeMB int) error {
	return errNotImplemented // TODO(h-g91eg)
}

func runStatus(slug string) error {
	return errNotImplemented // TODO(h-g91eg)
}

func runDisable(slug string, now bool) error {
	return errNotImplemented // TODO(h-g91eg)
}
