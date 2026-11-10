package root

// h-oazj: `hatch protect` must be registered on the root command, with an
// `email` subtree for the email-allowlist protection verbs. Mirrors
// TestWebhookCommandRegistered/TestVolumeCommandRegistered.

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestProtectCommandRegistered(t *testing.T) {
	var protect *cobra.Command
	for _, c := range rootCmd.Commands() {
		if c.Name() == "protect" {
			protect = c
			break
		}
	}
	if protect == nil {
		t.Fatal("`hatch protect` command not registered (h-oazj: egg email-allowlist protection)")
	}

	var email *cobra.Command
	for _, sc := range protect.Commands() {
		if sc.Name() == "email" {
			email = sc
			break
		}
	}
	if email == nil {
		t.Fatal("`hatch protect email` subcommand missing")
	}

	sub := map[string]*cobra.Command{}
	for _, sc := range email.Commands() {
		sub[sc.Name()] = sc
	}
	for _, name := range []string{"enable", "disable", "list", "add", "remove"} {
		if sub[name] == nil {
			t.Errorf("`hatch protect email %s` subcommand missing", name)
		}
	}
}
