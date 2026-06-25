package root

// h-62x9d: `hatch volume` must be registered on root now that the volumes
// backend is live. Mirrors TestResourcesCommandRegistered.

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestVolumeCommandRegistered(t *testing.T) {
	var volume *cobra.Command
	for _, c := range rootCmd.Commands() {
		if c.Name() == "volume" {
			volume = c
			break
		}
	}
	if volume == nil {
		t.Fatal("`hatch volume` command not registered (h-62x9d: ship the persistent-volume CLI verb)")
	}

	sub := map[string]*cobra.Command{}
	for _, sc := range volume.Commands() {
		sub[sc.Name()] = sc
	}
	for _, name := range []string{"enable", "status", "disable"} {
		if sub[name] == nil {
			t.Errorf("`hatch volume %s` subcommand missing", name)
		}
	}
}
