package deploy

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/EscapeVelocityOperations/hatch-cli/cmd/analyze"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/ui"
)

// LocalDeployConfig holds configuration for local deployment.
type LocalDeployConfig struct {
	Token     string
	AppName   string
	Domain    string
	OutputDir string // Override output directory
}

// RunLocalDeploy performs a local build and uploads the artifact.
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

	// 2. Run the build command
	ui.Info("Building locally...")
	buildCmd := strings.Fields(analysis.BuildCommand)
	if len(buildCmd) == 0 {
		return fmt.Errorf("no build command detected")
	}

	cmd := exec.Command(buildCmd[0], buildCmd[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "NODE_ENV=production")

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	// 3. Determine output directory
	outputDir := analysis.OutputDir
	if cfg.OutputDir != "" {
		outputDir = cfg.OutputDir
	}

	if _, err := os.Stat(outputDir); os.IsNotExist(err) {
		return fmt.Errorf("output directory %s does not exist after build", outputDir)
	}

	// 4. Check/create app
	client := api.NewClient(cfg.Token)

	var slug string
	hatchConfig, err := readHatchConfig()
	if err != nil {
		return fmt.Errorf("reading .hatch.toml: %w", err)
	}

	if hatchConfig != nil {
		slug = hatchConfig.Slug
		ui.Info(fmt.Sprintf("Deploying to existing app: %s", slug))
	} else {
		// Create new app
		name := cfg.AppName
		if name == "" {
			cwd, _ := os.Getwd()
			name = filepath.Base(cwd)
		}
		ui.Info(fmt.Sprintf("Creating new app: %s", name))
		app, err := client.CreateApp(name)
		if err != nil {
			return fmt.Errorf("creating app: %w", err)
		}
		slug = app.Slug
		ui.Success(fmt.Sprintf("Created app: %s", slug))
	}

	// 5. Create tar.gz of output directory
	ui.Info("Creating artifact...")
	artifact, err := createTarGz(outputDir, analysis)
	if err != nil {
		return fmt.Errorf("creating artifact: %w", err)
	}

	ui.Info(fmt.Sprintf("Artifact size: %.2f MB", float64(len(artifact))/1024/1024))

	// 6. Upload artifact
	sp := ui.NewSpinner("Uploading artifact...")
	sp.Start()
	err = uploadArtifact(cfg.Token, slug, artifact, analysis)
	sp.Stop()

	if err != nil {
		return fmt.Errorf("uploading artifact: %w", err)
	}

	ui.Success("Deployed successfully!")
	ui.Info(fmt.Sprintf("App URL: https://%s.hosted.gethatch.eu", slug))

	return nil
}

// createTarGz creates a tar.gz archive of the output directory.
func createTarGz(dir string, analysis *analyze.Analysis) ([]byte, error) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip node_modules in the output (they should be bundled)
		rel, _ := filepath.Rel(dir, path)
		if strings.Contains(rel, "node_modules") && !strings.HasPrefix(rel, ".output") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		header, err := tar.FileInfoHeader(info, "")
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

		if !info.IsDir() {
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

// ArtifactMetadata contains metadata about the uploaded artifact.
type ArtifactMetadata struct {
	Framework    string `json:"framework"`
	BuildCommand string `json:"buildCommand"`
	StartCommand string `json:"startCommand"`
	OutputDir    string `json:"outputDir"`
}

// uploadArtifact uploads the artifact to the Hatch API.
func uploadArtifact(token, slug string, artifact []byte, analysis *analyze.Analysis) error {
	// Build multipart request with artifact and metadata
	url := api.DefaultHost + "/v1/apps/" + slug + "/artifact"

	metadata := ArtifactMetadata{
		Framework:    analysis.Framework,
		BuildCommand: analysis.BuildCommand,
		StartCommand: analysis.StartCommand,
		OutputDir:    analysis.OutputDir,
	}

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return err
	}

	// Create request with artifact body
	req, err := http.NewRequest("POST", url, bytes.NewReader(artifact))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/gzip")
	req.Header.Set("X-Artifact-Metadata", string(metadataJSON))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return nil
}
