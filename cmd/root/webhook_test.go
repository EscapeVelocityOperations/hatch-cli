package root

// h-4knyl: `hatch webhook` must be registered on the root command now that the
// hatch-api webhook endpoints are live. Mirrors TestResourcesCommandRegistered.

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestWebhookCommandRegistered(t *testing.T) {
	var webhook *cobra.Command
	for _, c := range rootCmd.Commands() {
		if c.Name() == "webhook" {
			webhook = c
			break
		}
	}
	if webhook == nil {
		t.Fatal("`hatch webhook` command not registered (h-4knyl: wire deploy-webhook management to the live API)")
	}

	sub := map[string]*cobra.Command{}
	for _, sc := range webhook.Commands() {
		sub[sc.Name()] = sc
	}
	for _, name := range []string{"add", "list", "rm", "test"} {
		if sub[name] == nil {
			t.Errorf("`hatch webhook %s` subcommand missing", name)
		}
	}
}
