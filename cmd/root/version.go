package root

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	// Set via ldflags at build time.
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of hatch",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("hatch %s (commit: %s, built: %s)\n", version, commit, date)
	},
}
