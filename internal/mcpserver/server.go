package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/EscapeVelocityOperations/hatch-cli/cmd/analyze"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/auth"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// NewServer creates the Hatch MCP server with all tools and resources registered.
func NewServer() *server.MCPServer {
	s := server.NewMCPServer(
		"hatch",
		"1.0.0",
		server.WithToolCapabilities(false),
		server.WithResourceCapabilities(true, false),
	)

	// Read operations (get_*)
	s.AddTool(getPlatformInfoTool(), getPlatformInfoHandler)
	s.AddTool(analyzeProjectTool(), analyzeProjectHandler)
	s.AddTool(listAppsTool(), listAppsHandler)
	s.AddTool(getStatusTool(), getStatusHandler)
	s.AddTool(getLogsTool(), getLogsHandler)
	s.AddTool(getEnvTool(), getEnvHandler)
	s.AddTool(getDatabaseURLTool(), getDatabaseURLHandler)

	// Write operations (deploy_*, add_*, set_*)
	s.AddTool(deployAppTool(), deployAppHandler)
	s.AddTool(addDatabaseTool(), addDatabaseHandler)
	s.AddTool(addStorageTool(), addStorageHandler)
	s.AddTool(setEnvTool(), setEnvHandler)
	s.AddTool(addDomainTool(), addDomainHandler)

	// Resources
	s.AddResource(
		mcp.NewResource(
			"hatch://skill",
			"Hatch Platform Technical Reference",
			mcp.WithResourceDescription("Deploy flow, runtime detection, environment variables, and common issues for the Hatch PaaS"),
			mcp.WithMIMEType("text/markdown"),
		),
		skillResourceHandler,
	)

	return s
}

// skillResourceHandler returns the embedded SKILL.md content.
func skillResourceHandler(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      "hatch://skill",
			MIMEType: "text/markdown",
			Text:     SkillMD,
		},
	}, nil
}

// newClient creates an authenticated API client or returns an error result.
func newClient() (*api.Client, error) {
	token, err := auth.GetToken()
	if err != nil {
		return nil, fmt.Errorf("reading auth token: %w", err)
	}
	if token == "" {
		return nil, fmt.Errorf("not logged in - run 'hatch login', set HATCH_TOKEN, or use --token")
	}
	return api.NewClient(token), nil
}

// --- get_platform_info ---

// FrameworkSpec describes platform requirements for a framework.
type FrameworkSpec struct {
	BaseImage           string `json:"base_image"`
	NeedsStartCommand   bool   `json:"needs_start_command"`
	DefaultStartCommand string `json:"default_start_command,omitempty"`
	ExtractionPath      string `json:"extraction_path,omitempty"`
	Description         string `json:"description"`
}

// DeployRequirements is the platform contract returned to agents.
type DeployRequirements struct {
	Platform   PlatformSpec            `json:"platform"`
	Artifact   ArtifactSpec            `json:"artifact"`
	Frameworks map[string]FrameworkSpec `json:"frameworks"`
}

type PlatformSpec struct {
	Arch              string `json:"arch"`
	Port              int    `json:"port"`
	MaxArtifactSizeMB int    `json:"max_artifact_size_mb"`
}

type ArtifactSpec struct {
	Format   string `json:"format"`
	Contents string `json:"contents"`
}

func getPlatformInfoTool() mcp.Tool {
	return mcp.NewTool("get_platform_info",
		mcp.WithDescription("Returns the platform contract: supported frameworks, artifact format, and deployment requirements. Call this first to understand what the platform needs before preparing a deployment."),
	)
}

func getPlatformInfoHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	reqs := DeployRequirements{
		Platform: PlatformSpec{
			Arch:              "linux/amd64",
			Port:              8080,
			MaxArtifactSizeMB: 500,
		},
		Artifact: ArtifactSpec{
			Format:   "tar.gz",
			Contents: "Output directory contents only, no wrapping folder",
		},
		Frameworks: map[string]FrameworkSpec{
			"static": {
				BaseImage:         "nginx:alpine",
				NeedsStartCommand: false,
				Description:       "Static HTML/CSS/JS served by nginx",
			},
			"jekyll": {
				BaseImage:         "nginx:alpine",
				NeedsStartCommand: false,
				Description:       "Pre-built Jekyll site",
			},
			"hugo": {
				BaseImage:         "nginx:alpine",
				NeedsStartCommand: false,
				Description:       "Pre-built Hugo site",
			},
			"nuxt": {
				BaseImage:           "node:20-alpine",
				NeedsStartCommand:   true,
				DefaultStartCommand: "node .output/server/index.mjs",
				ExtractionPath:      ".output",
				Description:         "Nuxt 3 application (SSR or static)",
			},
			"next": {
				BaseImage:           "node:20-alpine",
				NeedsStartCommand:   true,
				DefaultStartCommand: "pnpm start",
				ExtractionPath:      ".next",
				Description:         "Next.js application",
			},
			"node": {
				BaseImage:           "node:20-alpine",
				NeedsStartCommand:   true,
				DefaultStartCommand: "node index.js",
				ExtractionPath:      ".",
				Description:         "Generic Node.js application",
			},
			"express": {
				BaseImage:           "node:20-alpine",
				NeedsStartCommand:   true,
				DefaultStartCommand: "node index.js",
				ExtractionPath:      ".",
				Description:         "Express.js application",
			},
		},
	}

	data, _ := json.MarshalIndent(reqs, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

// --- analyze_project ---

func analyzeProjectTool() mcp.Tool {
	return mcp.NewTool("analyze_project",
		mcp.WithDescription("Analyze a project directory to detect framework, build command, output directory, and native modules. Use this to understand a project before deploying."),
		mcp.WithString("directory",
			mcp.Required(),
			mcp.Description("Absolute path to the project directory to analyze"),
		),
	)
}

func analyzeProjectHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	dir, err := req.RequireString("directory")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return mcp.NewToolResultError(fmt.Sprintf("Directory not found: %s", dir)), nil
	}

	analysis, err := analyze.AnalyzeProject(dir)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Analysis failed: %v", err)), nil
	}

	data, _ := json.MarshalIndent(analysis, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

// --- list_apps ---

func listAppsTool() mcp.Tool {
	return mcp.NewTool("list_apps",
		mcp.WithDescription("List all your deployed apps with their slugs, status, and URLs."),
	)
}

func listAppsHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := newClient()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	apps, err := client.ListApps()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list apps: %v", err)), nil
	}

	if len(apps) == 0 {
		return mcp.NewToolResultText("No apps found."), nil
	}

	data, _ := json.MarshalIndent(apps, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

// --- deploy_app ---

func deployAppTool() mcp.Tool {
	return mcp.NewTool("deploy_app",
		mcp.WithDescription("Upload a pre-built tar.gz artifact to deploy an application. The agent should have already prepared the artifact using get_platform_info to understand the format. Creates a new app if no slug is provided and no .hatch.toml exists."),
		mcp.WithString("artifact_path",
			mcp.Required(),
			mcp.Description("Absolute path to the tar.gz artifact file"),
		),
		mcp.WithString("framework",
			mcp.Required(),
			mcp.Description("Framework type: static, jekyll, hugo, nuxt, next, node, or express"),
		),
		mcp.WithString("start_command",
			mcp.Description("Start command for the app (required for non-static frameworks)"),
		),
		mcp.WithString("app",
			mcp.Description("App slug to deploy to (reads .hatch.toml or creates new app if omitted)"),
		),
		mcp.WithString("name",
			mcp.Description("Custom app name for new apps (defaults to directory name)"),
		),
		mcp.WithString("directory",
			mcp.Description("Project directory for .hatch.toml lookup (defaults to cwd)"),
		),
	)
}

func deployAppHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	artifactPath, err := req.RequireString("artifact_path")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	fw, err := req.RequireString("framework")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	startCmd := req.GetString("start_command", "")
	appSlug := req.GetString("app", "")
	name := req.GetString("name", "")
	dir := req.GetString("directory", "")

	// Validate framework
	validFrameworks := map[string]bool{
		"static": true, "jekyll": true, "hugo": true,
		"nuxt": true, "next": true, "node": true, "express": true,
	}
	if !validFrameworks[fw] {
		return mcp.NewToolResultError(fmt.Sprintf("Unknown framework %q. Valid: static, jekyll, hugo, nuxt, next, node, express", fw)), nil
	}

	// Validate start command for non-static
	staticFrameworks := map[string]bool{"static": true, "jekyll": true, "hugo": true}
	if !staticFrameworks[fw] && startCmd == "" {
		return mcp.NewToolResultError(fmt.Sprintf("start_command is required for framework %q", fw)), nil
	}

	// Read artifact
	artifact, err := os.ReadFile(artifactPath)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Cannot read artifact: %v", err)), nil
	}

	// Auth
	client, err := newClient()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Resolve app slug
	slug := appSlug
	if slug == "" {
		// Check .hatch.toml
		tomlDir := dir
		if tomlDir == "" {
			tomlDir = "."
		}
		tomlPath := filepath.Join(tomlDir, ".hatch.toml")
		if data, err := os.ReadFile(tomlPath); err == nil {
			var cfg struct {
				App struct {
					Slug string `json:"slug" toml:"slug"`
				} `json:"app" toml:"app"`
			}
			// Try [app] section format
			if strings.Contains(string(data), "[app]") {
				lines := strings.Split(string(data), "\n")
				for _, line := range lines {
					line = strings.TrimSpace(line)
					if strings.HasPrefix(line, "slug") {
						parts := strings.SplitN(line, "=", 2)
						if len(parts) == 2 {
							slug = strings.Trim(strings.TrimSpace(parts[1]), "\"")
						}
					}
				}
			}
			_ = cfg // suppress unused
		}
	}

	if slug == "" {
		// Create new app
		appName := name
		if appName == "" {
			if dir != "" {
				appName = filepath.Base(dir)
			} else {
				cwd, _ := os.Getwd()
				appName = filepath.Base(cwd)
			}
		}

		app, err := client.CreateApp(appName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to create app: %v", err)), nil
		}
		slug = app.Slug

		// Write .hatch.toml for future deploys
		if dir != "" {
			tomlPath := filepath.Join(dir, ".hatch.toml")
			content := fmt.Sprintf("[app]\nslug = %q\nname = %q\n", slug, appName)
			_ = os.WriteFile(tomlPath, []byte(content), 0644)
		}
	}

	// Upload
	if err := client.UploadArtifact(slug, bytes.NewReader(artifact), fw, startCmd); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Upload failed: %v", err)), nil
	}

	appURL := fmt.Sprintf("https://%s.hosted.gethatch.eu", slug)
	return mcp.NewToolResultText(fmt.Sprintf("Deployed successfully!\nApp: %s\nURL: %s\nFramework: %s", slug, appURL, fw)), nil
}

// --- add_database ---

func addDatabaseTool() mcp.Tool {
	return mcp.NewTool("add_database",
		mcp.WithDescription("Provision a PostgreSQL database for an app. Sets DATABASE_URL automatically."),
		mcp.WithString("app",
			mcp.Required(),
			mcp.Description("App slug (name) to add the database to"),
		),
	)
}

func addDatabaseHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	slug, err := req.RequireString("app")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	client, err := newClient()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	addon, err := client.AddAddon(slug, "postgresql")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to add database: %v", err)), nil
	}

	result := fmt.Sprintf("PostgreSQL database provisioned for %s.\nStatus: %s", slug, addon.Status)
	if addon.URL != "" {
		result += fmt.Sprintf("\nDATABASE_URL: %s", addon.URL)
	}
	return mcp.NewToolResultText(result), nil
}

// --- add_storage ---

func addStorageTool() mcp.Tool {
	return mcp.NewTool("add_storage",
		mcp.WithDescription("Provision S3-compatible object storage for an app."),
		mcp.WithString("app",
			mcp.Required(),
			mcp.Description("App slug (name) to add storage to"),
		),
	)
}

func addStorageHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	slug, err := req.RequireString("app")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	client, err := newClient()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	addon, err := client.AddAddon(slug, "s3")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to add storage: %v", err)), nil
	}

	result := fmt.Sprintf("S3-compatible storage provisioned for %s.\nStatus: %s", slug, addon.Status)
	if addon.URL != "" {
		result += fmt.Sprintf("\nStorage URL: %s", addon.URL)
	}
	return mcp.NewToolResultText(result), nil
}

// --- get_logs ---

func getLogsTool() mcp.Tool {
	return mcp.NewTool("get_logs",
		mcp.WithDescription("Get recent application logs."),
		mcp.WithString("app",
			mcp.Required(),
			mcp.Description("App slug (name) to get logs for"),
		),
		mcp.WithNumber("lines",
			mcp.Description("Number of recent log lines to return (default 50)"),
		),
	)
}

func getLogsHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	slug, err := req.RequireString("app")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	lines := int(req.GetFloat("lines", 50))
	if lines <= 0 {
		lines = 50
	}

	client, err := newClient()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	logLines, err := client.GetLogs(slug, lines, "")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get logs: %v", err)), nil
	}

	if len(logLines) == 0 {
		return mcp.NewToolResultText("No recent logs found."), nil
	}

	return mcp.NewToolResultText(strings.Join(logLines, "\n")), nil
}

// --- get_status ---

func getStatusTool() mcp.Tool {
	return mcp.NewTool("get_status",
		mcp.WithDescription("Check if an app is running, its URL, and current status."),
		mcp.WithString("app",
			mcp.Required(),
			mcp.Description("App slug (name) to check"),
		),
	)
}

func getStatusHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	slug, err := req.RequireString("app")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	client, err := newClient()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	app, err := client.GetApp(slug)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get app status: %v", err)), nil
	}

	result := fmt.Sprintf("App: %s\nStatus: %s\nURL: %s\nRegion: %s\nCreated: %s\nUpdated: %s",
		app.Name, app.Status, app.URL, app.Region,
		app.CreatedAt.Format("2006-01-02 15:04:05"),
		app.UpdatedAt.Format("2006-01-02 15:04:05"))

	return mcp.NewToolResultText(result), nil
}

// --- set_env ---

func setEnvTool() mcp.Tool {
	return mcp.NewTool("set_env",
		mcp.WithDescription("Set an environment variable on a deployed app."),
		mcp.WithString("app",
			mcp.Required(),
			mcp.Description("App slug (name) to set the variable on"),
		),
		mcp.WithString("key",
			mcp.Required(),
			mcp.Description("Environment variable name"),
		),
		mcp.WithString("value",
			mcp.Required(),
			mcp.Description("Environment variable value"),
		),
	)
}

func setEnvHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	slug, err := req.RequireString("app")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	key, err := req.RequireString("key")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	value, err := req.RequireString("value")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	client, err := newClient()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	if err := client.SetEnvVar(slug, key, value); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to set env var: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Set %s on %s.", key, slug)), nil
}

// --- get_env ---

func getEnvTool() mcp.Tool {
	return mcp.NewTool("get_env",
		mcp.WithDescription("List all environment variables for an app."),
		mcp.WithString("app",
			mcp.Required(),
			mcp.Description("App slug (name) to list env vars for"),
		),
	)
}

func getEnvHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	slug, err := req.RequireString("app")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	client, err := newClient()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	vars, err := client.GetEnvVars(slug)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get env vars: %v", err)), nil
	}

	if len(vars) == 0 {
		return mcp.NewToolResultText("No environment variables set."), nil
	}

	data, _ := json.MarshalIndent(vars, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

// --- add_domain ---

func addDomainTool() mcp.Tool {
	return mcp.NewTool("add_domain",
		mcp.WithDescription("Add a custom domain to an app. Returns DNS instructions."),
		mcp.WithString("app",
			mcp.Required(),
			mcp.Description("App slug (name) to add the domain to"),
		),
		mcp.WithString("domain",
			mcp.Required(),
			mcp.Description("Custom domain name (e.g. example.com)"),
		),
	)
}

func addDomainHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	slug, err := req.RequireString("app")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	domain, err := req.RequireString("domain")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	client, err := newClient()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	d, err := client.AddDomain(slug, domain)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to add domain: %v", err)), nil
	}

	cname := d.CNAME
	if cname == "" {
		cname = slug + ".hosted.gethatch.eu"
	}

	result := fmt.Sprintf(`Domain %s configured for %s.
Status: %s

DNS Setup:
Add a CNAME record pointing %s to %s

  Type   Name    Value
  CNAME  @       %s
  CNAME  www     %s

For apex domains, use ALIAS/ANAME if your provider supports it.
SSL is provisioned automatically via Let's Encrypt once DNS propagates.`,
		domain, slug, d.Status, domain, cname, cname, cname)

	return mcp.NewToolResultText(result), nil
}

// --- get_database_url ---

func getDatabaseURLTool() mcp.Tool {
	return mcp.NewTool("get_database_url",
		mcp.WithDescription("Get the DATABASE_URL for an app's PostgreSQL database."),
		mcp.WithString("app",
			mcp.Required(),
			mcp.Description("App slug (name) to get the database URL for"),
		),
	)
}

func getDatabaseURLHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	slug, err := req.RequireString("app")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	client, err := newClient()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	vars, err := client.GetEnvVars(slug)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get env vars: %v", err)), nil
	}

	for _, v := range vars {
		if v.Key == "DATABASE_URL" {
			return mcp.NewToolResultText(v.Value), nil
		}
	}

	return mcp.NewToolResultError("No DATABASE_URL found. Add a database first with add_database."), nil
}
