package deploy

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/deployer"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/ui"
)

// LocalDeployConfig holds configuration for auto-mode deployment (analyze+build+upload).
type LocalDeployConfig struct {
	Token     string
	AppName   string
	Domain    string
	OutputDir string // Override output directory
}

// ArtifactDeployConfig holds configuration for agent-mode deployment (pre-built artifact).
type ArtifactDeployConfig struct {
	Token        string
	AppName      string
	Domain       string
	ArtifactPath string
	Framework    string
	StartCommand string
	AppSlug      string // Explicit slug (optional, reads .hatch.toml if empty)
	Directory    string // Project dir for .hatch.toml lookup
}

// validFrameworks lists accepted framework values.
var validFrameworks = map[string]bool{
	"static": true, "jekyll": true, "hugo": true,
	"nuxt": true, "next": true, "node": true, "express": true,
	"go": true, "python": true, "fastapi": true, "django": true, "flask": true, "rust": true,
}

// staticFrameworks don't need a start command.
var staticFrameworks = map[string]bool{
	"static": true, "jekyll": true, "hugo": true,
}

// RunArtifactDeploy deploys a pre-built artifact (agent mode).
func RunArtifactDeploy(cfg ArtifactDeployConfig) error {
	// Validate framework
	if cfg.Framework == "" {
		return fmt.Errorf("--framework is required when using --artifact")
	}
	if !validFrameworks[cfg.Framework] {
		return fmt.Errorf("unknown framework %q (valid: static, jekyll, hugo, nuxt, next, node, express, go, python, fastapi, django, flask, rust)", cfg.Framework)
	}

	// Validate start command for non-static
	if !staticFrameworks[cfg.Framework] && cfg.StartCommand == "" {
		return fmt.Errorf("--start-command is required for framework %q", cfg.Framework)
	}

	// Read artifact
	artifact, err := os.ReadFile(cfg.ArtifactPath)
	if err != nil {
		return fmt.Errorf("reading artifact: %w", err)
	}
	ui.Info(fmt.Sprintf("Artifact size: %.2f MB", float64(len(artifact))/1024/1024))

	// Resolve app
	client := deps.NewAPIClient(cfg.Token)
	dir := cfg.Directory
	if dir == "" {
		dir = "."
	}
	slug, err := resolveApp(client, cfg.AppSlug, cfg.AppName, dir)
	if err != nil {
		return err
	}

	// Upload
	sp := ui.NewSpinner("Uploading artifact...")
	sp.Start()
	err = client.UploadArtifact(slug, artifact, cfg.Framework, cfg.StartCommand)
	sp.Stop()
	if err != nil {
		return fmt.Errorf("uploading artifact: %w", err)
	}

	ui.Success("Deployed successfully!")
	ui.Info(fmt.Sprintf("Egg URL: https://%s.nest.gethatch.eu", slug))

	// Set custom domain if specified
	if cfg.Domain != "" {
		realClient := api.NewClient(cfg.Token)
		configureDomain(realClient, slug, cfg.Domain)
	}

	return nil
}

// apiClientAdapter adapts the deploy.APIClient interface to deployer.APIClient.
type apiClientAdapter struct {
	client APIClient
}

func (a *apiClientAdapter) CreateApp(name string) (string, error) {
	app, err := a.client.CreateApp(name)
	if err != nil {
		return "", err
	}
	return app.Slug, nil
}

func (a *apiClientAdapter) UploadArtifact(slug string, artifact []byte, framework, startCommand string) error {
	return a.client.UploadArtifact(slug, artifact, framework, startCommand)
}

// RunLocalDeploy performs auto-mode: analyze, build, upload.
func RunLocalDeploy(cfg LocalDeployConfig) error {
	// Create deployer with progress callback for UI updates
	d := deployer.NewDeployer(
		func() (string, error) { return cfg.Token, nil },
		func(token string) deployer.APIClient {
			return &apiClientAdapter{client: deps.NewAPIClient(token)}
		},
	)

	// Set up progress callback to show UI updates
	var currentSpinner *ui.Spinner
	d.Progress = func(stage, message string) {
		// Stop any running spinner
		if currentSpinner != nil {
			currentSpinner.Stop()
			currentSpinner = nil
		}

		// Show info messages for most stages
		if stage == "uploading" {
			currentSpinner = ui.NewSpinner(message)
			currentSpinner.Start()
		} else {
			ui.Info(message)
		}
	}

	// Deploy using the deployer package
	opts := deployer.DeployOptions{
		Directory: ".",
		Name:      cfg.AppName,
		OutputDir: cfg.OutputDir,
	}

	result, err := d.Deploy(context.Background(), opts)

	// Stop spinner if still running
	if currentSpinner != nil {
		currentSpinner.Stop()
	}

	if err != nil {
		return err
	}

	// Show warnings for native modules
	if result.Analysis.HasNativeModules {
		ui.Warn("Project has native modules - local build may not work on different architecture")
		if len(result.Analysis.NativeModules) > 0 {
			ui.Info("Native modules: " + strings.Join(result.Analysis.NativeModules, ", "))
		}
	}

	ui.Success("Deployed successfully!")
	ui.Info(fmt.Sprintf("Egg URL: %s", result.URL))

	// Set custom domain if specified
	if cfg.Domain != "" {
		realClient := api.NewClient(cfg.Token)
		configureDomain(realClient, result.Slug, cfg.Domain)
	}

	return nil
}

// configureDomain adds a custom domain to an app.
func configureDomain(client *api.Client, slug, domainName string) {
	ui.Info(fmt.Sprintf("Configuring custom domain: %s", domainName))
	domain, err := client.AddDomain(slug, domainName)
	if err != nil {
		ui.Warn(fmt.Sprintf("Domain configuration failed: %v", err))
		ui.Info("You can configure it later with: hatch domain add " + domainName)
	} else {
		ui.Success(fmt.Sprintf("Domain %s configured", domainName))
		if domain.CNAME != "" {
			ui.Info(fmt.Sprintf("CNAME target: %s", domain.CNAME))
		}
	}
}
