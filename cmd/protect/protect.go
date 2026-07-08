// Package protect implements `hatch protect` — access protection for eggs.
// email.go is the email-allowlist subtree (h-oazj); password protection
// (h-abmr) is not yet built.
package protect

import "github.com/spf13/cobra"

// NewCmd returns the `hatch protect` command group.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "protect",
		Short: "Manage access protection for your eggs",
	}
	cmd.AddCommand(NewEmailCmd())
	return cmd
}
