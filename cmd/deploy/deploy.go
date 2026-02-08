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
	UploadArtifact(slug string, artifact []byte, runtime, startCommand string) error
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

func (r *realAPIClient) UploadArtifact(slug string, artifact []byte, runtime, startCommand string) error {
	return r.client.UploadArtifact(slug, bytes.NewReader(artifact), runtime, startCommand)
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
	deployTarget string
	runtime      string
	startCommand string
)

func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy a pre-built application directory to Hatch",
		Long: `Deploy a pre-built application directory to Hatch.

YOU must build the project first (e.g. npm run build, go build, etc.),
then point --deploy-target at the output directory containing everything
needed to run the app. Hatch wraps it in a thin container and deploys it.

Required flags:
  --deploy-target <dir>    Path to the build output directory
  --runtime <runtime>      Base container image: node, python, go, or static
  --start-command <cmd>    Command to start your app (not needed for static)

Platform constraints:
  - Container runs linux/amd64
  - App must listen on PORT env var (always 8080)
  - App must bind to 0.0.0.0 (not 127.0.0.1 or localhost)
  - Max artifact size: 500 MB

Examples:
  # Node.js (Nuxt)
  cd my-nuxt-app && pnpm build
  hatch deploy --deploy-target .output --runtime node \
    --start-command "node server/index.mjs"

  # Python (FastAPI)
  cd my-api && pip install -r requirements.txt
  hatch deploy --deploy-target . --runtime python \
    --start-command "uvicorn main:app --host 0.0.0.0 --port 8080"

  # Go
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o dist/server .
  hatch deploy --deploy-target dist --runtime go \
    --start-command "./server"

  # Static site
  cd my-site && npm run build
  hatch deploy --deploy-target dist --runtime static`,
		RunE: runDeploy,
	}
	cmd.Flags().StringVarP(&appName, "name", "n", "", "custom egg name (defaults to directory name)")
	cmd.Flags().StringVarP(&domainName, "domain", "d", "", "custom domain (e.g. example.com)")
	cmd.Flags().StringVar(&deployTarget, "deploy-target", "", "path to the build output directory (required)")
	cmd.Flags().StringVar(&runtime, "runtime", "", "base container image: node, python, go, or static (required)")
	cmd.Flags().StringVar(&startCommand, "start-command", "", "command to start the app (required for non-static runtimes)")
	return cmd
}

func runDeploy(cmd *cobra.Command, args []string) error {
	// Validate required flags
	if deployTarget == "" {
		return fmt.Errorf("--deploy-target is required\n\nUsage: hatch deploy --deploy-target <dir> --runtime <runtime> --start-command <cmd>\n\nRun 'hatch deploy --help' for details")
	}
	if runtime == "" {
		return fmt.Errorf("--runtime is required (node, python, go, or static)\n\nRun 'hatch deploy --help' for details")
	}

	// Check auth
	token, err := deps.GetToken()
	if err != nil {
		return fmt.Errorf("checking auth: %w", err)
	}
	if token == "" {
		return fmt.Errorf("not logged in. Run 'hatch login', set HATCH_TOKEN, or use --token")
	}

	return RunArtifactDeploy(ArtifactDeployConfig{
		Token:        token,
		AppName:      appName,
		Domain:       domainName,
		DeployTarget: deployTarget,
		Runtime:      runtime,
		StartCommand: startCommand,
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
