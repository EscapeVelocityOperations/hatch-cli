package mcpserver

import (
	"os"
	"strings"
	"testing"
)

// D3: skill_content.go's SkillMD stays the single source of truth
// (go:embed can't reach ../../skills from this package). These tests define
// the generated frontmatter contract and guard against drift between the
// generator's output and the committed skills/hatch/SKILL.md.

func TestGenerateSkillMD_FrontmatterHasTriggerPhrases(t *testing.T) {
	md := GenerateSkillMD()
	if !strings.HasPrefix(md, "---\n") {
		t.Fatal("expected SKILL.md to start with YAML frontmatter")
	}
	for _, want := range []string{"name: hatch", "deploy to hatch", "gethatch.eu"} {
		if !strings.Contains(md, want) {
			t.Errorf("expected SKILL.md frontmatter/description to contain %q, got:\n%s", want, md)
		}
	}
}

func TestGenerateSkillMD_IncludesSkillMDBody(t *testing.T) {
	md := GenerateSkillMD()
	if !strings.Contains(md, SkillMD) {
		t.Error("expected generated SKILL.md to include the full SkillMD body")
	}
}

// TestSkillMDSyncGuard regenerates SKILL.md in memory and byte-compares it
// against the committed file. Drift (SkillMD edited without re-running
// `go generate ./...`) fails this test.
func TestSkillMDSyncGuard(t *testing.T) {
	want := GenerateSkillMD()

	got, err := os.ReadFile("../../skills/hatch/SKILL.md")
	if err != nil {
		t.Fatalf("reading committed skills/hatch/SKILL.md: %v (run `go generate ./...` from internal/mcpserver and commit the result)", err)
	}

	if string(got) != want {
		t.Fatal("skills/hatch/SKILL.md is out of sync with SkillMD — run `go generate ./...` from internal/mcpserver and commit the result")
	}
}
