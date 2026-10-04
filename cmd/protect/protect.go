// Package protect implements `hatch protect` — access protection for eggs.
// email.go is the email-allowlist subtree (h-oazj); password.go is
// password-only protection (h-abmr), wired directly on this parent command.
package protect

import "github.com/spf13/cobra"

// NewCmd returns the `hatch protect` command group. Password protection
// (h-abmr) is a single credential, not a list, so it lives on this parent
// command as flags (--password, --off) rather than its own verb subtree —
// `hatch protect [slug] --password <p>` / `--off` / bare for status.
//
// Args is set explicitly: cobra only rejects unknown positionals on the root
// command, so without it `hatch protect other-app --off` would silently
// ignore other-app and mutate the cwd egg (h-abmr F1).
func NewCmd() *cobra.Command {
	cmd := newProtectCmd()
	cmd.AddCommand(NewEmailCmd())
	return cmd
}

// newProtectCmd builds the parent command without the email subtree, whose
// subcommands are package-level and can only be mounted once per process.
func newProtectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "protect [slug]",
		Short: "Manage access protection for your eggs",
		Long: "Manage access protection for an egg. The egg is the [slug] argument " +
			"if given, otherwise the app in the current directory (.hatch.toml).",
		Args: cobra.MaximumNArgs(1),
		RunE: runProtect,
	}
	cmd.Flags().String("password", "", "Set a password, enabling password protection")
	cmd.Flags().Bool("off", false, "Disable password protection")
	return cmd
}
