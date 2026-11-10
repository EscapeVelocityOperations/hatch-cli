package mcpserver

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// deploySlugResolution describes which app a deploy will target and how the
// slug was determined, so callers can surface the provenance to the user.
type deploySlugResolution struct {
	Slug       string // empty means "create a new app"
	Provenance string // human-readable origin of the slug
	TomlName   string // app name recorded in the toml, when the slug came from one
}

// parseHatchTomlApp extracts slug and name from the [app] section of a
// .hatch.toml. Keys in other sections are ignored.
func parseHatchTomlApp(data []byte) (slug, name string) {
	inApp := false
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			inApp = line == "[app]"
			continue
		}
		if !inApp {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.Trim(strings.TrimSpace(parts[1]), "\"")
		switch key {
		case "slug":
			slug = val
		case "name":
			name = val
		}
	}
	return slug, name
}

// resolveDeploySlug determines the target app slug for deploy_app. An explicit
// app parameter always wins. Otherwise the .hatch.toml is read from the
// deploy_target directory — never from the MCP server's working directory,
// which is unrelated to the caller's intent and has retargeted deploys to the
// wrong live app in the past. A name that contradicts the toml is a hard
// error instead of being silently ignored.
func resolveDeploySlug(appSlug, name, deployTarget string) (deploySlugResolution, error) {
	if appSlug != "" {
		return deploySlugResolution{Slug: appSlug, Provenance: "explicit app parameter"}, nil
	}

	tomlPath := filepath.Join(deployTarget, ".hatch.toml")
	data, err := os.ReadFile(tomlPath)
	if err != nil {
		return deploySlugResolution{}, nil
	}

	slug, tomlName := parseHatchTomlApp(data)
	if slug == "" {
		return deploySlugResolution{}, nil
	}
	if name != "" && tomlName != "" && name != tomlName {
		return deploySlugResolution{}, fmt.Errorf(
			"name %q conflicts with app %q pinned by %s — pass an explicit app: slug to deploy there, or remove the name parameter to use the pinned app",
			name, tomlName, tomlPath)
	}
	return deploySlugResolution{
		Slug:       slug,
		Provenance: fmt.Sprintf(".hatch.toml at %s", tomlPath),
		TomlName:   tomlName,
	}, nil
}

// writeHatchToml pins a newly created app in the deployed directory so later
// deploys of the same directory target the same app.
func writeHatchToml(dir, slug, name string) error {
	content := fmt.Sprintf("[app]\nslug = %q\nname = %q\n", slug, name)
	return os.WriteFile(filepath.Join(dir, ".hatch.toml"), []byte(content), 0644)
}
