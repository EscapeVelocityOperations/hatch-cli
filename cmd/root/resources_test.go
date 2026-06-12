package root

// TDD red — h-ek431 per-runtime resource profiles, CLI surface (bead h-5r7kd).
// Contract under test (spec on h-lcznn): a `hatch resources` command group
// with `show` (current effective limits + overrides) and `set` (per-app
// override, e.g. `hatch resources set --memory 768`), mirroring the API's
// PATCH /v1/apps/{slug}/resources. impl-cli (h-qrt0p) makes this green.

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestResourcesCommandRegistered(t *testing.T) {
	var resources *cobra.Command
	for _, c := range rootCmd.Commands() {
		if c.Name() == "resources" {
			resources = c
			break
		}
	}
	if resources == nil {
		t.Fatal("`hatch resources` command not registered (h-ek431: per-app override of runtime resource profiles)")
	}

	sub := map[string]*cobra.Command{}
	for _, sc := range resources.Commands() {
		sub[sc.Name()] = sc
	}
	if sub["show"] == nil {
		t.Error("`hatch resources show` subcommand missing")
	}
	set := sub["set"]
	if set == nil {
		t.Fatal("`hatch resources set` subcommand missing")
	}
	if set.Flags().Lookup("memory") == nil {
		t.Error("`hatch resources set` missing --memory flag (MB override)")
	}
	if set.Flags().Lookup("cpu") == nil {
		t.Error("`hatch resources set` missing --cpu flag (MHz override)")
	}
}
