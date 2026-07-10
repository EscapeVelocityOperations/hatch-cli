// Package protect implements `hatch protect` — access protection for eggs.
// email.go is the email-allowlist subtree (h-oazj); password.go is
// password-only protection (h-abmr), wired directly on this parent command.
package protect

import "github.com/spf13/cobra"

// NewCmd returns the `hatch protect` command group. Password protection
// (h-abmr) is a single credential, not a list, so it lives on this parent
// command as flags (--password, --off) rather than its own verb subtree —
// `hatch protect --password <p>` / `--off` / bare for status.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "protect",
		Short: "Manage access protection for your eggs",
		RunE:  runProtect,
	}
	cmd.Flags().String("password", "", "Set a password, enabling password protection")
	cmd.AddCommand(NewEmailCmd())
	return cmd
}
