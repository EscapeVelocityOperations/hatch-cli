package deployer

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/EscapeVelocityOperations/hatch-cli/cmd/analyze"
)

// DeployOptions holds configuration for a deployment.
type DeployOptions struct {
	Directory   string // Project directory to deploy (defaults to ".")
	Name        string // Optional app name (defaults to directory name)
	Framework   string // Optional framework override
	BuildCmd    string // Optional build command override
	StartCmd    string // Optional start command override
	OutputDir   string // Optional output directory override
	BuildEnv    []string // Additional environment variables for build
}

// DeployResult holds the result of a successful deployment.
type DeployResult struct {
	Slug      string // App slug
	URL       string // Deployed app URL
	Framework string // Detected or specified framework
	Analysis  *analyze.Analysis // Full analysis result
}

// ProgressFunc is a callback for reporting progress during deployment.
// stage examples: "analyzing", "building", "creating_artifact", "uploading"
// message examples: "Analyzing project...", "Building locally...", "Uploading artifact..."
type ProgressFunc func(stage, message string)

// APIClient defines the interface for interacting with the Hatch API.
type APIClient interface {
	CreateApp(name string) (slug string, err error)
	UploadArtifact(slug string, artifact []byte, framework, startCommand string) error
}

// Deployer orchestrates the deployment pipeline.
type Deployer struct {
	// GetToken retrieves the authentication token.
	GetToken func() (string, error)

	// NewAPIClient creates an API client with the given token.
	NewAPIClient func(token string) APIClient

	// Progress is an optional callback for progress updates.
	Progress ProgressFunc

	// AllowedBuildCommands lists acceptable build tools (for validation).
	AllowedBuildCommands map[string]bool
}

// NewDeployer creates a Deployer with default configuration.
func NewDeployer(getToken func() (string, error), newAPIClient func(token string) APIClient) *Deployer {
	return &Deployer{
		GetToken:     getToken,
		NewAPIClient: newAPIClient,
		AllowedBuildCommands: map[string]bool{
			"npm": true, "npx": true, "pnpm": true, "yarn": true, "bun": true,
			"make": true, "go": true, "cargo": true, "bundle": true,
			"hugo": true, "jekyll": true,
		},
	}
}

// Deploy executes the full deployment pipeline: analyze → build → tar → upload.
func (d *Deployer) Deploy(ctx context.Context, opts DeployOptions) (*DeployResult, error) {
	// Set defaults
	if opts.Directory == "" {
		opts.Directory = "."
	}

	// 1. Analyze the project
	d.progress("analyzing", "Analyzing project...")
	analysis, err := analyze.AnalyzeProject(opts.Directory)
	if err != nil {
		return nil, fmt.Errorf("analyzing project: %w", err)
	}

	// Apply overrides
	if opts.Framework != "" {
		analysis.Framework = opts.Framework
	}
	if opts.BuildCmd != "" {
		analysis.BuildCommand = opts.BuildCmd
	}
	if opts.StartCmd != "" {
		analysis.StartCommand = opts.StartCmd
	}
	if opts.OutputDir != "" {
		analysis.OutputDir = opts.OutputDir
	}

	// 2. Run the build command (skip for static sites)
	buildCmd := strings.Fields(analysis.BuildCommand)
	if len(buildCmd) == 0 {
		d.progress("building", "Static site detected — no build step needed")
	} else {
		// Validate build command against allowlist.
		// This is a security boundary: the build_command parameter comes from MCP tool
		// input (potentially agent-controlled). Only known-safe build tools are permitted.
		if d.AllowedBuildCommands != nil && !d.AllowedBuildCommands[buildCmd[0]] {
			return nil, fmt.Errorf("build command %q is not in the allowed list (permitted: npm, npx, pnpm, yarn, bun, make, go, cargo, bundle, hugo, jekyll)", buildCmd[0])
		}

		d.progress("building", "Building locally...")
		cmd := exec.CommandContext(ctx, buildCmd[0], buildCmd[1:]...)
		cmd.Dir = opts.Directory
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		// Set build environment
		cmd.Env = append(os.Environ(), "NODE_ENV=production")
		if len(opts.BuildEnv) > 0 {
			cmd.Env = append(cmd.Env, opts.BuildEnv...)
		}

		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("build failed: %w", err)
		}
	}

	// 3. Verify output directory exists
	outputPath := filepath.Join(opts.Directory, analysis.OutputDir)
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("output directory %s does not exist after build", analysis.OutputDir)
	}

	// 4. Get authentication
	token, err := d.GetToken()
	if err != nil {
		return nil, fmt.Errorf("getting token: %w", err)
	}
	if token == "" {
		return nil, fmt.Errorf("not authenticated")
	}

	client := d.NewAPIClient(token)

	// 5. Resolve or create app
	slug, err := d.resolveApp(client, opts.Name, opts.Directory)
	if err != nil {
		return nil, err
	}

	// 6. Create tar.gz artifact
	d.progress("creating_artifact", "Creating artifact...")
	artifact, err := createTarGz(outputPath)
	if err != nil {
		return nil, fmt.Errorf("creating artifact: %w", err)
	}

	d.progress("creating_artifact", fmt.Sprintf("Artifact size: %.2f MB", float64(len(artifact))/1024/1024))

	// 7. Upload artifact
	d.progress("uploading", "Uploading artifact...")
	err = client.UploadArtifact(slug, artifact, analysis.Framework, analysis.StartCommand)
	if err != nil {
		return nil, fmt.Errorf("uploading artifact: %w", err)
	}

	// 8. Return result
	result := &DeployResult{
		Slug:      slug,
		URL:       fmt.Sprintf("https://%s.hosted.gethatch.eu", slug),
		Framework: analysis.Framework,
		Analysis:  analysis,
	}

	return result, nil
}

// progress calls the progress callback if set.
func (d *Deployer) progress(stage, message string) {
	if d.Progress != nil {
		d.Progress(stage, message)
	}
}

// resolveApp resolves or creates an app, returning the slug.
func (d *Deployer) resolveApp(client APIClient, nameOverride, dir string) (string, error) {
	// Check for .hatch.toml
	hatchConfig, err := readHatchConfig(dir)
	if err != nil {
		return "", fmt.Errorf("reading .hatch.toml: %w", err)
	}
	if hatchConfig != nil {
		d.progress("resolving_app", fmt.Sprintf("Deploying to existing nugget: %s", hatchConfig.Slug))
		return hatchConfig.Slug, nil
	}

	// Create new app
	name := nameOverride
	if name == "" {
		absDir, _ := filepath.Abs(dir)
		name = filepath.Base(absDir)
	}

	d.progress("resolving_app", fmt.Sprintf("Creating new nugget: %s", name))
	slug, err := client.CreateApp(name)
	if err != nil {
		return "", fmt.Errorf("creating nugget: %w", err)
	}

	// Write .hatch.toml for future deploys
	if err := writeHatchConfig(dir, slug, name); err != nil {
		d.progress("resolving_app", fmt.Sprintf("Warning: could not write .hatch.toml: %v", err))
	}

	return slug, nil
}

// createTarGz creates a tar.gz archive of the given directory.
func createTarGz(dir string) ([]byte, error) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolving directory: %w", err)
	}

	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
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

			// Validate symlink doesn't escape output directory
			target := link
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(path), target)
			}
			target, err = filepath.Abs(target)
			if err != nil {
				return err
			}

			// Skip symlinks that point outside the output directory
			if !strings.HasPrefix(target, absDir+string(filepath.Separator)) && target != absDir {
				return nil // Skip this symlink
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

	// Check artifact size
	if buf.Len() > 500*1024*1024 {
		return nil, fmt.Errorf("artifact too large (%.0f MB, max 500 MB)", float64(buf.Len())/1024/1024)
	}

	return buf.Bytes(), nil
}
