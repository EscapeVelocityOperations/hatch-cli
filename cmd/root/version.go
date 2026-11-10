package root

import (
	"fmt"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/version"
	"github.com/spf13/cobra"
)

var (
	// Set via ldflags at build time.
	commit = "none"
	date   = "unknown"
)

// Version returns the current CLI version string.
func Version() string {
	return version.Version()
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of hatch",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("hatch %s (commit: %s, built: %s)\n", version.Version(), commit, date)
	},
}
