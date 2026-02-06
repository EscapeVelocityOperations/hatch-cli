package deploy

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/EscapeVelocityOperations/hatch-cli/cmd/analyze"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
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
		return fmt.Errorf("unknown framework %q (valid: static, jekyll, hugo, nuxt, next, node, express)", cfg.Framework)
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
	ui.Info(fmt.Sprintf("Nugget URL: https://%s.hosted.gethatch.eu", slug))

	// Set custom domain if specified
	if cfg.Domain != "" {
		realClient := api.NewClient(cfg.Token)
		configureDomain(realClient, slug, cfg.Domain)
	}

	return nil
}

// RunLocalDeploy performs auto-mode: analyze, build, upload.
func RunLocalDeploy(cfg LocalDeployConfig) error {
	// 1. Analyze the project
	ui.Info("Analyzing project...")
	analysis, err := analyze.AnalyzeProject(".")
	if err != nil {
		return fmt.Errorf("analyzing project: %w", err)
	}

	if analysis.HasNativeModules {
		ui.Warn("Project has native modules - local build may not work on different architecture")
		ui.Info("Native modules: " + strings.Join(analysis.NativeModules, ", "))
	}

	ui.Info(fmt.Sprintf("Framework: %s", analysis.Framework))
	ui.Info(fmt.Sprintf("Build command: %s", analysis.BuildCommand))
	ui.Info(fmt.Sprintf("Output directory: %s", analysis.OutputDir))

	// 2. Run the build command (skip for static sites)
	buildCmd := strings.Fields(analysis.BuildCommand)
	if len(buildCmd) == 0 {
		ui.Info("Static site detected — no build step needed")
	} else {
		ui.Info("Building locally...")
		cmd := exec.Command(buildCmd[0], buildCmd[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Env = append(os.Environ(), "NODE_ENV=production")

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("build failed: %w", err)
		}
	}

	// 3. Determine output directory
	outputDir := analysis.OutputDir
	if cfg.OutputDir != "" {
		outputDir = cfg.OutputDir
	}

	if _, err := os.Stat(outputDir); os.IsNotExist(err) {
		return fmt.Errorf("output directory %s does not exist after build", outputDir)
	}

	// 4. Resolve app
	client := deps.NewAPIClient(cfg.Token)
	slug, err := resolveApp(client, "", cfg.AppName, ".")
	if err != nil {
		return err
	}

	// 5. Create tar.gz of output directory
	ui.Info("Creating artifact...")
	artifact, err := createTarGz(outputDir)
	if err != nil {
		return fmt.Errorf("creating artifact: %w", err)
	}

	ui.Info(fmt.Sprintf("Artifact size: %.2f MB", float64(len(artifact))/1024/1024))

	// 6. Upload artifact
	sp := ui.NewSpinner("Uploading artifact...")
	sp.Start()
	err = client.UploadArtifact(slug, artifact, analysis.Framework, analysis.StartCommand)
	sp.Stop()

	if err != nil {
		return fmt.Errorf("uploading artifact: %w", err)
	}

	ui.Success("Deployed successfully!")
	ui.Info(fmt.Sprintf("Nugget URL: https://%s.hosted.gethatch.eu", slug))

	// Set custom domain if specified
	if cfg.Domain != "" {
		realClient := api.NewClient(cfg.Token)
		configureDomain(realClient, slug, cfg.Domain)
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

// createTarGz creates a tar.gz archive of the output directory.
func createTarGz(dir string) ([]byte, error) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, _ := filepath.Rel(dir, path)

		// Handle symlinks
		link := ""
		if info.Mode()&os.ModeSymlink != 0 {
			link, err = os.Readlink(path)
			if err != nil {
				return err
			}
		}

		header, err := tar.FileInfoHeader(info, link)
		if err != nil {
			return err
		}

		header.Name = rel
		if info.IsDir() {
			header.Name += "/"
		}

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		// Only copy content for regular files (not dirs or symlinks)
		if info.Mode().IsRegular() {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()
			if _, err := io.Copy(tw, f); err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gw.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
