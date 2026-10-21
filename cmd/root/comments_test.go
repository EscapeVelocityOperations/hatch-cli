package root

// h-gldp.3: `hatch comments` must be registered on the root command with
// every verb of the plan (h-40lj § CLI).

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestCommentsCommandRegistered(t *testing.T) {
	var comments *cobra.Command
	for _, c := range rootCmd.Commands() {
		if c.Name() == "comments" {
			comments = c
			break
		}
	}
	if comments == nil {
		t.Fatal("`hatch comments` command not registered (h-gldp.3)")
	}

	sub := map[string]*cobra.Command{}
	for _, sc := range comments.Commands() {
		sub[sc.Name()] = sc
	}
	for _, name := range []string{"enable", "disable", "status", "list", "show", "reply",
		"resolve", "reopen", "delete", "export", "purge"} {
		if sub[name] == nil {
			t.Errorf("`hatch comments %s` subcommand missing", name)
		}
	}
}
