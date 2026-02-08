package deploy

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/auth"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/ui"
	"github.com/spf13/cobra"
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

// APIClient is the interface for the Hatch API.
type APIClient interface {
	CreateApp(name string) (*api.App, error)
	UploadArtifact(slug string, artifact []byte, framework, startCommand string) error
}

// Deps holds injectable dependencies for testing.
type Deps struct {
	GetToken     func() (string, error)
	GetCwd       func() (string, error)
	NewAPIClient func(token string) APIClient
}

// realAPIClient wraps api.Client to implement APIClient interface.
type realAPIClient struct {
	client *api.Client
}

func (r *realAPIClient) CreateApp(name string) (*api.App, error) {
	return r.client.CreateApp(name)
}

func (r *realAPIClient) UploadArtifact(slug string, artifact []byte, framework, startCommand string) error {
	return r.client.UploadArtifact(slug, bytes.NewReader(artifact), framework, startCommand)
}

func defaultDeps() *Deps {
	return &Deps{
		GetToken: auth.GetToken,
		GetCwd:   getCwd,
		NewAPIClient: func(token string) APIClient {
			return &realAPIClient{client: api.NewClient(token)}
		},
	}
}

var deps = defaultDeps()

var (
	appName      string
	domainName   string
	artifactPath string
	framework    string
	startCommand string
)

func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy your egg to Hatch",
		Long:  "Deploy the current directory as a Hatch egg. Builds locally and uploads the artifact.",
		RunE:  runDeploy,
	}
	cmd.Flags().StringVarP(&appName, "name", "n", "", "custom egg name (defaults to directory name)")
	cmd.Flags().StringVarP(&domainName, "domain", "d", "", "custom domain (e.g. example.com)")
	cmd.Flags().StringVar(&artifactPath, "artifact", "", "path to pre-built tar.gz artifact (skips analyze+build)")
	cmd.Flags().StringVar(&framework, "framework", "", "framework type (static, jekyll, hugo, nuxt, next, node, express)")
	cmd.Flags().StringVar(&startCommand, "start-command", "", "start command for the egg (required for non-static frameworks)")
	return cmd
}

func runDeploy(cmd *cobra.Command, args []string) error {
	// 1. Check auth
	token, err := deps.GetToken()
	if err != nil {
		return fmt.Errorf("checking auth: %w", err)
	}
	if token == "" {
		return fmt.Errorf("not logged in. Run 'hatch login', set HATCH_TOKEN, or use --token")
	}

	// 2. Route to agent mode or auto mode
	if artifactPath != "" {
		return RunArtifactDeploy(ArtifactDeployConfig{
			Token:        token,
			AppName:      appName,
			Domain:       domainName,
			ArtifactPath: artifactPath,
			Framework:    framework,
			StartCommand: startCommand,
		})
	}

	// Auto mode: analyze, build, upload
	return RunLocalDeploy(LocalDeployConfig{
		Token:   token,
		AppName: appName,
		Domain:  domainName,
	})
}

func getCwd() (string, error) {
	return filepath.Abs(".")
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

// resolveApp resolves or creates an app, returning the slug. Writes .hatch.toml on first deploy.
func resolveApp(client APIClient, appSlug, appNameOverride, dir string) (string, error) {
	// If explicit slug provided, use it
	if appSlug != "" {
		return appSlug, nil
	}

	// Check .hatch.toml
	hatchConfig, err := readHatchConfig(dir)
	if err != nil {
		return "", fmt.Errorf("reading .hatch.toml: %w", err)
	}
	if hatchConfig != nil {
		ui.Info(fmt.Sprintf("Deploying to existing egg: %s", hatchConfig.Slug))
		return hatchConfig.Slug, nil
	}

	// Create new app
	name := appNameOverride
	if name == "" {
		if dir == "" || dir == "." {
			cwd, _ := os.Getwd()
			name = filepath.Base(cwd)
		} else {
			name = filepath.Base(dir)
		}
	}

	ui.Info(fmt.Sprintf("Creating new egg: %s", name))
	app, err := client.CreateApp(name)
	if err != nil {
		return "", fmt.Errorf("creating egg: %w", err)
	}
	ui.Success(fmt.Sprintf("Created egg: %s", app.Slug))

	if err := writeHatchConfig(dir, app.Slug, name); err != nil {
		ui.Warn(fmt.Sprintf("Could not write .hatch.toml: %v", err))
	}

	return app.Slug, nil
}
