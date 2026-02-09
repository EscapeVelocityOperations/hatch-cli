package initcmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/mcpserver"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/ui"
	"github.com/spf13/cobra"
)

var (
	claudeMDOnly bool
	mcpOnly      bool
	force        bool
)

func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Bootstrap a project for Hatch deployment + AI agent integration",
		Long: `Sets up your project for deploying to Hatch and configures AI agent integration.

This command:
  1. Writes a "Hatch Deployment" section to CLAUDE.md (creates if missing)
  2. Adds the Hatch MCP server to .claude/settings.json

Use --claude-md-only or --mcp-only to run just one step.`,
		RunE: runInit,
	}

	cmd.Flags().BoolVar(&claudeMDOnly, "claude-md-only", false, "only write CLAUDE.md, skip MCP config")
	cmd.Flags().BoolVar(&mcpOnly, "mcp-only", false, "only configure MCP server, skip CLAUDE.md")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing Hatch section in CLAUDE.md")

	return cmd
}

const hatchSectionMarker = "## Hatch Deployment"

func runInit(cmd *cobra.Command, args []string) error {
	wroteClaudeMD := false
	wroteMCP := false

	if !mcpOnly {
		var err error
		wroteClaudeMD, err = writeClaudeMD()
		if err != nil {
			return err
		}
	}

	if !claudeMDOnly {
		var err error
		wroteMCP, err = writeMCPConfig()
		if err != nil {
			return err
		}
	}

	fmt.Println()
	if wroteClaudeMD {
		ui.Success("Added Hatch deployment guide to CLAUDE.md")
	}
	if wroteMCP {
		ui.Success("Added Hatch MCP server to .claude/settings.json")
	}

	fmt.Println()
	fmt.Println("  Next steps:")
	fmt.Println("    1. Build your project")
	fmt.Println("    2. Run " + ui.Bold("hatch deploy"))
	fmt.Println()

	return nil
}

func writeClaudeMD() (bool, error) {
	const filename = "CLAUDE.md"

	existing, err := os.ReadFile(filename)
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("reading %s: %w", filename, err)
	}

	content := string(existing)

	if strings.Contains(content, hatchSectionMarker) {
		if !force {
			ui.Warn("CLAUDE.md already contains a Hatch section. Use --force to overwrite.")
			return false, nil
		}
		// Remove existing section: from marker to next ## or EOF
		content = removeHatchSection(content)
	}

	// Append hatch section
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	if len(content) > 0 && !strings.HasSuffix(content, "\n\n") {
		content += "\n"
	}
	content += mcpserver.ClaudeMDContent

	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		return false, fmt.Errorf("writing %s: %w", filename, err)
	}

	return true, nil
}

func removeHatchSection(content string) string {
	idx := strings.Index(content, hatchSectionMarker)
	if idx < 0 {
		return content
	}

	before := content[:idx]

	// Find the next ## heading after the marker
	rest := content[idx+len(hatchSectionMarker):]
	nextHeading := strings.Index(rest, "\n## ")
	if nextHeading >= 0 {
		return before + rest[nextHeading+1:]
	}

	// No next heading — section goes to EOF
	return strings.TrimRight(before, "\n") + "\n"
}

func writeMCPConfig() (bool, error) {
	settingsDir := filepath.Join(".claude")
	settingsPath := filepath.Join(settingsDir, "settings.json")

	// Read existing settings or start fresh
	var settings map[string]any

	data, err := os.ReadFile(settingsPath)
	if err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return false, fmt.Errorf("parsing %s: %w", settingsPath, err)
		}
	} else if os.IsNotExist(err) {
		settings = make(map[string]any)
	} else {
		return false, fmt.Errorf("reading %s: %w", settingsPath, err)
	}

	// Get or create mcpServers
	mcpServers, _ := settings["mcpServers"].(map[string]any)
	if mcpServers == nil {
		mcpServers = make(map[string]any)
	}

	// Check if hatch already configured
	if _, exists := mcpServers["hatch"]; exists {
		ui.Info("Hatch MCP server already configured in .claude/settings.json")
		return false, nil
	}

	mcpServers["hatch"] = map[string]any{
		"command": "hatch",
		"args":    []string{"mcp"},
	}
	settings["mcpServers"] = mcpServers

	// Ensure directory exists
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		return false, fmt.Errorf("creating %s: %w", settingsDir, err)
	}

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return false, fmt.Errorf("marshaling settings: %w", err)
	}

	if err := os.WriteFile(settingsPath, append(out, '\n'), 0644); err != nil {
		return false, fmt.Errorf("writing %s: %w", settingsPath, err)
	}

	return true, nil
}
