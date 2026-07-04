package mcpserver

import (
	"os"
	"strings"
	"testing"
)

// T-016: commands/onboard.md drives the guided /hatch:onboard first-deploy
// flow. This guard doesn't validate Claude Code's own frontmatter parsing
// (out of this repo's reach) — it locks in the content contract: the file
// exists, is non-empty, and references the tools the guided flow drives.
func TestOnboardCommand_ExistsAndReferencesFlow(t *testing.T) {
	data, err := os.ReadFile("../../commands/onboard.md")
	if err != nil {
		t.Fatalf("reading commands/onboard.md: %v", err)
	}

	content := string(data)
	if strings.TrimSpace(content) == "" {
		t.Fatal("commands/onboard.md is empty")
	}

	for _, want := range []string{"get_started", "login", "deploy_app", "get_status"} {
		if !strings.Contains(content, want) {
			t.Errorf("expected commands/onboard.md to reference %q, got:\n%s", want, content)
		}
	}
}
