package deploy

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/ui"
)

// ArtifactDeployConfig holds configuration for deploy-target mode.
type ArtifactDeployConfig struct {
	Token        string
	AppName      string
	Domain       string
	DeployTarget string
	Runtime      string
	StartCommand string
	AppSlug      string // Explicit slug (optional, reads .hatch.toml if empty)
}

// validRuntimes lists accepted runtime values.
var validRuntimes = map[string]bool{
	"node": true, "python": true, "go": true, "rust": true, "php": true, "bun": true, "static": true,
}

// RunArtifactDeploy deploys a pre-built directory as an artifact.
func RunArtifactDeploy(cfg ArtifactDeployConfig) error {
	// Validate runtime
	if cfg.Runtime == "" {
		return fmt.Errorf("--runtime is required (node, python, go, rust, php, bun, or static)")
	}
	if !validRuntimes[cfg.Runtime] {
		return fmt.Errorf("unknown runtime %q (valid: node, python, go, rust, php, bun, static)", cfg.Runtime)
	}

	// Validate start command for non-static
	if cfg.Runtime != "static" && cfg.StartCommand == "" {
		return fmt.Errorf("--start-command is required for runtime %q", cfg.Runtime)
	}

	// Validate deploy-target directory exists
	targetInfo, err := os.Stat(cfg.DeployTarget)
	if err != nil {
		return fmt.Errorf("deploy-target directory not found: %s", cfg.DeployTarget)
	}
	if !targetInfo.IsDir() {
		return fmt.Errorf("deploy-target must be a directory: %s", cfg.DeployTarget)
	}

	// Validate entrypoint exists (for non-static runtimes)
	if cfg.Runtime != "static" && cfg.StartCommand != "" {
		entrypoint := parseEntrypoint(cfg.StartCommand)
		if entrypoint != "" {
			entrypointPath := filepath.Join(cfg.DeployTarget, entrypoint)
			if _, err := os.Stat(entrypointPath); os.IsNotExist(err) {
				return fmt.Errorf("entrypoint file %q not found in deploy-target %q\n\nThe start-command references %q but that file does not exist in your deploy-target directory.\nCheck that your build output is complete.", entrypoint, cfg.DeployTarget, entrypoint)
			}
		}
	}

	// Create tar.gz from directory
	ui.Info("Creating artifact from " + cfg.DeployTarget)
	artifact, excluded, err := createTarGz(cfg.DeployTarget)
	if err != nil {
		return fmt.Errorf("creating artifact: %w", err)
	}
	if len(excluded) > 0 {
		fmt.Println(ui.Dim("  Excluded: " + strings.Join(excluded, ", ")))
	}
	ui.Info(fmt.Sprintf("Artifact size: %.2f MB", float64(len(artifact))/1024/1024))

	// Resolve app
	client := deps.NewAPIClient(cfg.Token)
	slug, name, err := resolveApp(client, cfg.AppSlug, cfg.AppName, ".")
	if err != nil {
		return err
	}

	// Upload
	sp := ui.NewSpinner("Uploading artifact...")
	sp.Start()
	err = client.UploadArtifact(slug, artifact, cfg.Runtime, cfg.StartCommand)
	sp.Stop()
	if err != nil {
		return fmt.Errorf("uploading artifact: %w", err)
	}

	// Write .hatch.toml only after successful upload
	if name != "" {
		if err := writeHatchConfig(cfg.DeployTarget, slug, name); err != nil {
			ui.Warn(fmt.Sprintf("Could not write .hatch.toml: %v", err))
		}
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

// parseEntrypoint extracts the file path from a start command.
// e.g. "node server/index.mjs" -> "server/index.mjs"
// e.g. "python -m uvicorn main:app" -> "" (skip validation for -m flag)
// e.g. "./server" -> "server"
func parseEntrypoint(cmd string) string {
	parts := strings.Fields(cmd)
	if len(parts) < 2 {
		return ""
	}

	// Skip the interpreter (node, python, etc.)
	// If second arg starts with -, it's a flag — skip validation
	arg := parts[1]
	if strings.HasPrefix(arg, "-") {
		return ""
	}

	// Strip leading ./ for path checking
	return strings.TrimPrefix(arg, "./")
}

// configureDomain adds a custom domain to an app.
func configureDomain(client *api.Client, slug, domainName string) {
	ui.Info(fmt.Sprintf("Configuring custom domain: %s", domainName))
	domain, err := client.AddDomain(slug, domainName)
	if err != nil {
		ui.Warn(fmt.Sprintf("Domain configuration failed: %v", err))
		ui.Info("You can configure it later with: hatch domain add " + domainName)
	} else {
		if domain.Verified {
			ui.Success(fmt.Sprintf("Domain %s configured and verified", domainName))
		} else {
			ui.Success(fmt.Sprintf("Domain %s configured (pending verification)", domainName))
			fmt.Println()
			fmt.Println(ui.Bold("To verify ownership, add this DNS TXT record:"))
			fmt.Printf("  Host:  %s\n", ui.Bold("_hatch-verify."+domainName))
			fmt.Printf("  Value: %s\n", ui.Bold(domain.VerificationToken))
			fmt.Println()
			fmt.Printf("Then run: %s\n", ui.Bold(fmt.Sprintf("hatch domain verify %s --app %s", domainName, slug)))
			fmt.Println()
		}
		if domain.CNAME != "" {
			ui.Info(fmt.Sprintf("CNAME target: %s", domain.CNAME))
		}
	}
}

// createTarGz creates a tar.gz archive of the given directory.
// Returns the archive bytes and a list of excluded dotfile/dotfolder names.
func createTarGz(dir string) ([]byte, []string, error) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("resolving directory: %w", err)
	}

	var excluded []string

	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Never skip the root directory itself (even if deploy-target starts with ".")
		if path == dir {
			return nil
		}

		// Skip all dotfiles and dot-prefixed directories
		if strings.HasPrefix(filepath.Base(path), ".") {
			if info.IsDir() {
				excluded = append(excluded, filepath.Base(path)+"/")
				return filepath.SkipDir
			}
			excluded = append(excluded, filepath.Base(path))
			return nil
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
				return nil
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
		return nil, nil, err
	}

	if err := tw.Close(); err != nil {
		return nil, nil, err
	}
	if err := gw.Close(); err != nil {
		return nil, nil, err
	}

	// Check artifact size
	if buf.Len() > 500*1024*1024 {
		return nil, nil, fmt.Errorf("artifact too large (%.0f MB, max 500 MB)", float64(buf.Len())/1024/1024)
	}

	return buf.Bytes(), excluded, nil
}
