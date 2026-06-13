package packs

import "testing"

func TestPacksCmd_Structure(t *testing.T) {
	cmd := NewCmd()
	if cmd.Use != "packs" {
		t.Errorf("Use = %q, want packs", cmd.Use)
	}

	var hasBuy, hasList bool
	for _, c := range cmd.Commands() {
		switch c.Name() {
		case "buy":
			hasBuy = true
		case "list":
			hasList = true
		}
	}
	if !hasBuy {
		t.Error("missing 'buy' subcommand")
	}
	if !hasList {
		t.Error("missing 'list' subcommand")
	}
}
