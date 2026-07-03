//go:build ignore

// Command gen_skill regenerates skills/hatch/SKILL.md from GenerateSkillMD
// (see skill_content.go). Run via `go generate ./...` from this directory —
// the go:generate directive lives in skill_content.go.
package main

import (
	"os"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/mcpserver"
)

func main() {
	if err := os.MkdirAll("../../skills/hatch", 0755); err != nil {
		panic(err)
	}
	if err := os.WriteFile("../../skills/hatch/SKILL.md", []byte(mcpserver.GenerateSkillMD()), 0644); err != nil {
		panic(err)
	}
}
