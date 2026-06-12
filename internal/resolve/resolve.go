package resolve

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type hatchConfig struct {
	Slug string `toml:"slug"`
}

type appSection struct {
	App hatchConfig `toml:"app"`
}

// SlugFromToml reads the app slug from .hatch.toml in the current directory.
// Returns empty string if file doesn't exist or has no slug.
func SlugFromToml() string {
	return SlugFromDir(".")
}

// SlugFromDir reads the app slug from .hatch.toml in dir. Returns empty
// string if the file doesn't exist or has no slug. Callers that resolve the
// working directory through an injectable dependency (e.g. cmd/webhook) use
// this so tests can point at a temp app dir.
func SlugFromDir(dir string) string {
	path := filepath.Join(dir, ".hatch.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	// Try flat format first: slug = "my-app"
	var cfg hatchConfig
	if err := toml.Unmarshal(data, &cfg); err == nil && cfg.Slug != "" {
		return cfg.Slug
	}

	// Try [app] section format: [app]\nslug = "my-app"
	var appCfg appSection
	if err := toml.Unmarshal(data, &appCfg); err == nil && appCfg.App.Slug != "" {
		return appCfg.App.Slug
	}

	return ""
}
