// Package scripts also guards the plugin manifests (.claude-plugin/plugin.json,
// .claude-plugin/marketplace.json) against drifting from the wrapper script
// they must reference (T-020): version lockstep across both manifests, and
// the mcpServers.hatch.command pointing at an existing executable file.
package scripts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const pluginRootPlaceholder = "${CLAUDE_PLUGIN_ROOT}/"

type pluginManifest struct {
	Version    string `json:"version"`
	MCPServers map[string]struct {
		Command string            `json:"command"`
		Args    []string          `json:"args"`
		Env     map[string]string `json:"env"`
	} `json:"mcpServers"`
}

type marketplaceManifest struct {
	Version string `json:"version"`
	Plugins []struct {
		Version string `json:"version"`
	} `json:"plugins"`
}

func readPluginManifest(t *testing.T) pluginManifest {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", ".claude-plugin", "plugin.json"))
	if err != nil {
		t.Fatalf("reading plugin.json: %v", err)
	}
	var m pluginManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("parsing plugin.json: %v", err)
	}
	return m
}

func readMarketplaceManifest(t *testing.T) marketplaceManifest {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", ".claude-plugin", "marketplace.json"))
	if err != nil {
		t.Fatalf("reading marketplace.json: %v", err)
	}
	var m marketplaceManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("parsing marketplace.json: %v", err)
	}
	return m
}

// TestManifestVersionLockstep guards plugin.json's version against both of
// marketplace.json's version fields (top-level + plugins[0]) drifting apart.
func TestManifestVersionLockstep(t *testing.T) {
	plugin := readPluginManifest(t)
	marketplace := readMarketplaceManifest(t)

	if len(marketplace.Plugins) == 0 {
		t.Fatalf("marketplace.json has no plugins entries")
	}

	if plugin.Version != marketplace.Version {
		t.Errorf("plugin.json version %q != marketplace.json top-level version %q", plugin.Version, marketplace.Version)
	}
	if plugin.Version != marketplace.Plugins[0].Version {
		t.Errorf("plugin.json version %q != marketplace.json plugins[0].version %q", plugin.Version, marketplace.Plugins[0].Version)
	}
}

// TestPluginManifest_CommandUsesWrapper guards plugin.json's hatch MCP server
// command against pointing anywhere but the bootstrap wrapper (D4), and
// against the empty HATCH_TOKEN env var placeholder T-020 removes.
func TestPluginManifest_CommandUsesWrapper(t *testing.T) {
	plugin := readPluginManifest(t)

	hatch, ok := plugin.MCPServers["hatch"]
	if !ok {
		t.Fatalf("plugin.json mcpServers has no %q entry", "hatch")
	}

	if !strings.HasPrefix(hatch.Command, pluginRootPlaceholder) {
		t.Fatalf("mcpServers.hatch.command = %q, want prefix %q", hatch.Command, pluginRootPlaceholder)
	}

	if _, present := hatch.Env["HATCH_TOKEN"]; present {
		t.Errorf("mcpServers.hatch.env still declares HATCH_TOKEN (should be removed, T-020)")
	}

	relPath := strings.TrimPrefix(hatch.Command, pluginRootPlaceholder)
	// Plugin root == repo root (marketplace.json plugins[0].source == "./");
	// this test package lives one directory below repo root.
	resolved := filepath.Join("..", relPath)
	info, err := os.Stat(resolved)
	if err != nil {
		t.Fatalf("mcpServers.hatch.command resolves to %s, which does not exist: %v", resolved, err)
	}
	if info.Mode()&0111 == 0 {
		t.Errorf("wrapper script %s is not executable (mode %s)", resolved, info.Mode())
	}
}
