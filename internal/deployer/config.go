package deployer

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

// HatchConfig represents the .hatch.toml config file.
type HatchConfig struct {
	Slug      string `toml:"slug"`
	Name      string `toml:"name"`
	CreatedAt string `toml:"created_at"`
}

// readHatchConfig reads .hatch.toml from the given directory (or cwd if empty).
func readHatchConfig(dir string) (*HatchConfig, error) {
	if dir == "" {
		dir = "."
	}
	path := filepath.Join(dir, ".hatch.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // Not an error, just doesn't exist
		}
		return nil, err
	}

	var config HatchConfig
	if err := toml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing .hatch.toml: %w", err)
	}

	// Support both [app] section format and flat format
	if config.Slug == "" {
		var appConfig struct {
			App HatchConfig `toml:"app"`
		}
		if err := toml.Unmarshal(data, &appConfig); err == nil && appConfig.App.Slug != "" {
			config = appConfig.App
		}
	}

	if config.Slug == "" {
		return nil, fmt.Errorf("invalid .hatch.toml: missing slug")
	}

	return &config, nil
}

// writeHatchConfig writes a .hatch.toml file to persist app identity across deploys.
func writeHatchConfig(dir, slug, name string) error {
	if dir == "" {
		dir = "."
	}
	path := filepath.Join(dir, ".hatch.toml")
	content := fmt.Sprintf("[app]\nslug = %q\nname = %q\ncreated_at = %q\n", slug, name, time.Now().Format(time.RFC3339))
	return os.WriteFile(path, []byte(content), 0644)
}
