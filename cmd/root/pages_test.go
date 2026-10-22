package root

// h-8iz8: `hatch pages` must be registered on the root command with every
// verb of the plan.

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestPagesCommandRegistered(t *testing.T) {
	var pages *cobra.Command
	for _, c := range rootCmd.Commands() {
		if c.Name() == "pages" {
			pages = c
			break
		}
	}
	if pages == nil {
		t.Fatal("`hatch pages` command not registered (h-8iz8)")
	}
	sub := map[string]bool{}
	for _, sc := range pages.Commands() {
		sub[sc.Name()] = true
	}
	for _, name := range []string{"publish", "list", "versions", "remove"} {
		if !sub[name] {
			t.Errorf("`hatch pages %s` subcommand missing", name)
		}
	}
}
