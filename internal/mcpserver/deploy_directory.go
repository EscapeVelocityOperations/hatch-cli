package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/auth"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/deployer"
	"github.com/mark3labs/mcp-go/mcp"
)

// mcpAPIClientAdapter adapts api.Client to deployer.APIClient interface.
type mcpAPIClientAdapter struct {
	client *api.Client
}

func (a *mcpAPIClientAdapter) CreateApp(name string) (string, error) {
	app, err := a.client.CreateApp(name)
	if err != nil {
		return "", err
	}
	return app.Slug, nil
}

func (a *mcpAPIClientAdapter) UploadArtifact(slug string, artifact []byte, framework, startCommand string) error {
	return a.client.UploadArtifact(slug, bytes.NewReader(artifact), framework, startCommand)
}

func deployDirectoryTool() mcp.Tool {
	return mcp.NewTool("deploy_directory",
		mcp.WithDescription("Deploy a project directory. Analyzes the project, builds it, creates an artifact, and uploads. One-step deploy."),
		mcp.WithString("directory",
			mcp.Required(),
			mcp.Description("Absolute path to the project directory to deploy"),
		),
		mcp.WithString("name",
			mcp.Description("App name (defaults to directory name)"),
		),
		mcp.WithString("framework",
			mcp.Description("Framework override (static, jekyll, hugo, nuxt, next, node, express)"),
		),
		mcp.WithString("build_command",
			mcp.Description("Build command override"),
		),
		mcp.WithString("start_command",
			mcp.Description("Start command override"),
		),
	)
}

func deployDirectoryHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	dir, err := req.RequireString("directory")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	if err := validateProjectPath(dir); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return mcp.NewToolResultError(fmt.Sprintf("Directory not found: %s", dir)), nil
	}

	// Get optional parameters
	name := req.GetString("name", "")
	framework := req.GetString("framework", "")
	buildCmd := req.GetString("build_command", "")
	startCmd := req.GetString("start_command", "")

	// Create deployer with API client factory
	d := deployer.NewDeployer(
		auth.GetToken,
		func(token string) deployer.APIClient {
			return &mcpAPIClientAdapter{client: api.NewClient(token)}
		},
	)

	// Set up options
	opts := deployer.DeployOptions{
		Directory: dir,
		Name:      name,
		Framework: framework,
		BuildCmd:  buildCmd,
		StartCmd:  startCmd,
	}

	// Execute deployment
	result, err := d.Deploy(ctx, opts)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Deployment failed: %v", err)), nil
	}

	// Return JSON result
	response := map[string]string{
		"slug":      result.Slug,
		"url":       result.URL,
		"framework": result.Framework,
	}

	data, _ := json.MarshalIndent(response, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}
