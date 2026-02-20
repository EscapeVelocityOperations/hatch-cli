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
	"github.com/EscapeVelocityOperations/hatch-cli/internal/ignore"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/ui"
	"golang.org/x/term"
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
	// Inform interactive users that Hatch is designed for AI agents
	if term.IsTerminal(int(os.Stdout.Fd())) {
		ui.Info("Hatch is designed for AI agents. Manual usage is supported, but your agent should handle deployment in production.")
	}

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

	// Check if deploy target looks like a source directory (not build output)
	if err := checkSourceDirectory(cfg.DeployTarget, cfg.Runtime); err != nil {
		return err
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

// isSourceDirectory checks if the deploy target looks like a project root
// (not a build output directory).
func isSourceDirectory(dir string) bool {
	has := func(name string) bool {
		_, err := os.Stat(filepath.Join(dir, name))
		return err == nil
	}
	// Node/Bun project root (has package.json + node_modules)
	if has("package.json") && has("node_modules") {
		return true
	}
	// Go project root
	if has("go.mod") {
		return true
	}
	// Python project root
	if has("requirements.txt") || has("pyproject.toml") || has("Pipfile") {
		return true
	}
	return false
}

// checkSourceDirectory warns or errors when deploying from a source directory.
// Static and PHP runtimes require a .hatchignore; others get a warning.
func checkSourceDirectory(dir, rt string) error {
	if !isSourceDirectory(dir) {
		return nil
	}

	hatchignorePath := filepath.Join(dir, ".hatchignore")
	_, err := os.Stat(hatchignorePath)
	hasIgnore := err == nil

	if (rt == "static" || rt == "php") && !hasIgnore {
		return fmt.Errorf("--runtime %s requires a .hatchignore file when deploying from a project directory.\n\n"+
			"Create one to control which files are included in the artifact:\n\n"+
			"  # .hatchignore — files/dirs to exclude from deploy\n"+
			"  node_modules/\n"+
			"  src/\n"+
			"  *.md\n"+
			"  tests/\n\n"+
			"Or generate one with: hatch init-ignore --runtime %s\n\n"+
			"Then run 'hatch deploy' again.", rt, rt)
	}

	if !hasIgnore {
		ui.Warn("deploy-target looks like a project root, not a build output directory.")
		ui.Info("Consider creating a .hatchignore or pointing --deploy-target at your build output.")
		ui.Info("Generate one with: hatch init-ignore")
	}
	return nil
}

// createTarGz creates a tar.gz archive of the given directory.
// Uses .hatchignore patterns if present, otherwise applies built-in defaults.
// Returns the archive bytes and a list of excluded file/folder names.
func createTarGz(dir string) ([]byte, []string, error) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("resolving directory: %w", err)
	}

	// Load .hatchignore or use defaults
	matcher, err := ignore.LoadFile(filepath.Join(dir, ".hatchignore"))
	if err != nil {
		if os.IsNotExist(err) {
			matcher = ignore.DefaultMatcher()
		} else {
			return nil, nil, fmt.Errorf("reading .hatchignore: %w", err)
		}
	}

	var excluded []string
	excludedSet := make(map[string]bool)

	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Never skip the root directory itself (even if deploy-target starts with ".")
		if path == dir {
			return nil
		}

		rel, _ := filepath.Rel(dir, path)

		if matcher.ShouldExclude(rel, info.IsDir()) {
			label := rel
			if info.IsDir() {
				label += "/"
				if !excludedSet[label] {
					excludedSet[label] = true
					excluded = append(excluded, label)
				}
				return filepath.SkipDir
			}
			if !excludedSet[label] {
				excludedSet[label] = true
				excluded = append(excluded, label)
			}
			return nil
		}

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
