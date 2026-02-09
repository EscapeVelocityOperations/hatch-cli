package mcpserver

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/auth"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Package-level functions for dependency injection (overridden in tests).
var (
	getTokenFunc = auth.GetToken
	newAPIClient = func(token string) *api.Client { return api.NewClient(token) }
)

// redactError applies token redaction to error messages returned to the MCP client.
func redactError(msg string) string {
	return api.RedactToken(msg)
}

// toolError returns a consistent, redacted error result for MCP tool handlers.
// All error messages should use the format "failed to {action}: {detail}".
func toolError(format string, args ...interface{}) (*mcp.CallToolResult, error) {
	msg := fmt.Sprintf(format, args...)
	return mcp.NewToolResultError(redactError(msg)), nil
}

// validateProjectPath ensures directory paths are safe and not in restricted locations.
func validateProjectPath(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// Block path traversal attempts (check raw input before resolution)
	if strings.Contains(dir, "..") {
		return fmt.Errorf("path traversal detected in %q", dir)
	}

	// Resolve symlinks to prevent blocklist bypass via symlinked paths.
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("resolving path: %w", err)
		}
		resolved = abs
	}

	// Block sensitive directories (check both the absolute and symlink-resolved paths)
	blocked := []string{"/etc", "/root", "/var", "/usr", "/bin", "/sbin", "/sys", "/proc"}
	for _, b := range blocked {
		if strings.HasPrefix(resolved, b+"/") || resolved == b {
			return fmt.Errorf("path %q resolves to restricted directory %q", dir, b)
		}
		if strings.HasPrefix(abs, b+"/") || abs == b {
			return fmt.Errorf("path %q is in a restricted directory", abs)
		}
	}

	return nil
}

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
	s.AddTool(listAppsTool(), listAppsHandler)
	s.AddTool(getStatusTool(), getStatusHandler)
	s.AddTool(getLogsTool(), getLogsHandler)
	s.AddTool(getEnvTool(), getEnvHandler)
	s.AddTool(listEnvVarsTool(), listEnvVarsHandler)
	s.AddTool(getDatabaseURLTool(), getDatabaseURLHandler)
	s.AddTool(getAppDetailsTool(), getAppDetailsHandler)
	s.AddTool(healthCheckTool(), healthCheckHandler)

	// Write operations (deploy_*, add_*, set_*, delete_*, remove_*, restart_*)
	s.AddTool(deployAppTool(), deployAppHandler)
	s.AddTool(addDatabaseTool(), addDatabaseHandler)
	s.AddTool(addStorageTool(), addStorageHandler)
	s.AddTool(setEnvTool(), setEnvHandler)
	s.AddTool(setEnvVarTool(), setEnvVarHandler)
	s.AddTool(bulkSetEnvTool(), bulkSetEnvHandler)
	s.AddTool(deleteEnvTool(), deleteEnvHandler)
	s.AddTool(addDomainTool(), addDomainHandler)
	s.AddTool(listDomainsTool(), listDomainsHandler)
	s.AddTool(removeDomainTool(), removeDomainHandler)
	s.AddTool(restartAppTool(), restartAppHandler)
	s.AddTool(getBuildLogsTool(), getBuildLogsHandler)

	// CRUD operations
	s.AddTool(createAppTool(), createAppHandler)
	s.AddTool(deleteAppTool(), deleteAppHandler)
	s.AddTool(checkAuthTool(), checkAuthHandler)

	// Energy operations
	s.AddTool(checkEnergyTool(), checkEnergyHandler)
	s.AddTool(getAppEnergyTool(), getAppEnergyHandler)
	s.AddTool(boostAppTool(), boostAppHandler)

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

	s.AddResource(
		mcp.NewResource(
			"hatch://tos",
			"Hatch Terms of Service for AI Agents",
			mcp.WithResourceDescription("What can and cannot be hosted on Hatch — forbidden data types, forbidden applications, resource limits, and legal obligations"),
			mcp.WithMIMEType("text/markdown"),
		),
		tosResourceHandler,
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

// tosResourceHandler fetches the current TOS from the hatch-landing API.
func tosResourceHandler(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	tosURL := "https://gethatch.eu/api/tos/agents"

	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Get(tosURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch TOS: %w", err)
	}
	defer resp.Body.Close()

	body := new(bytes.Buffer)
	body.ReadFrom(resp.Body)

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      "hatch://tos",
			MIMEType: "text/markdown",
			Text:     body.String(),
		},
	}, nil
}

// newClient creates an authenticated API client or returns an error.
func newClient() (*api.Client, error) {
	token, err := getTokenFunc()
	if err != nil {
		return nil, fmt.Errorf("failed to read auth token: %w", err)
	}
	if token == "" {
		return nil, fmt.Errorf("not authenticated - run 'hatch login', set HATCH_TOKEN, or use --token")
	}
	return newAPIClient(token), nil
}

// --- get_platform_info ---

// RuntimeSpec describes a runtime available on the platform.
type RuntimeSpec struct {
	BaseImage            string   `json:"base_image"`
	Description          string   `json:"description"`
	StartCommandExamples []string `json:"start_command_examples"`
}

// DeployRequirements is the platform contract returned to agents.
type DeployRequirements struct {
	Platform    PlatformSpec           `json:"platform"`
	DeployTarget DeployTargetSpec      `json:"deploy_target"`
	Runtimes    map[string]RuntimeSpec `json:"runtimes"`
}

type PlatformSpec struct {
	Arch              string `json:"arch"`
	Port              int    `json:"port"`
	MaxArtifactSizeMB int    `json:"max_artifact_size_mb"`
}

type DeployTargetSpec struct {
	Description string `json:"description"`
	Tip         string `json:"tip"`
}

func getPlatformInfoTool() mcp.Tool {
	return mcp.NewTool("get_platform_info",
		mcp.WithDescription("Returns the platform contract: supported runtimes, artifact format, and deployment requirements. Call this first to understand what the platform needs before preparing a deployment."),
	)
}

func getPlatformInfoHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	reqs := DeployRequirements{
		Platform: PlatformSpec{
			Arch:              "linux/amd64",
			Port:              8080,
			MaxArtifactSizeMB: 500,
		},
		DeployTarget: DeployTargetSpec{
			Description: "Directory containing your build output. Entire contents extracted to /app/ in container.",
			Tip:         "Include everything needed at runtime (compiled code, node_modules, static assets). Exclude source code and dev dependencies.",
		},
		Runtimes: map[string]RuntimeSpec{
			"node": {
				BaseImage:   "node:20-alpine",
				Description: "Node.js 20 — for Nuxt, Next, Express, or any Node app",
				StartCommandExamples: []string{
					"node server/index.mjs",
					"node index.js",
					"npx next start",
				},
			},
			"python": {
				BaseImage:   "python:3.12-slim",
				Description: "Python 3.12 — for FastAPI, Django, Flask, or any Python app",
				StartCommandExamples: []string{
					"uvicorn main:app --host 0.0.0.0 --port 8080",
					"gunicorn app:app --bind 0.0.0.0:8080",
					"python main.py",
				},
			},
			"go": {
				BaseImage:   "alpine:latest",
				Description: "Minimal Alpine — for pre-compiled Go or Rust binaries",
				StartCommandExamples: []string{
					"./server",
					"./myapp",
				},
			},
			"static": {
				BaseImage:            "nginx:alpine",
				Description:          "Nginx — for static HTML/CSS/JS sites. No start_command needed.",
				StartCommandExamples: []string{},
			},
		},
	}

	data, _ := json.MarshalIndent(reqs, "", "  ")
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
		return toolError("failed to list apps: %v", err)
	}

	apps, err := client.ListApps()
	if err != nil {
		return toolError("failed to list apps: %v", err)
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
		mcp.WithDescription(`Deploy a pre-built application directory to Hatch.

PREREQUISITE: You must build the project first (npm run build, go build, etc.)
and have the build output in a local directory.

WHAT THIS TOOL DOES:
1. Validates the deploy-target directory exists
2. Validates the start-command entrypoint file exists in deploy-target
3. Creates a tar.gz of the directory contents
4. Uploads to Hatch which wraps it in a thin container image and deploys

CONTAINER BEHAVIOR:
- The ENTIRE contents of deploy_target are extracted to /app/ inside the container
- The start_command runs from /app/ as working directory
- PORT env var is always 8080 — your app must listen on it
- Your app must bind to 0.0.0.0 (not localhost)

RUNTIME IMAGES:
- "node"   → node:20-alpine (for Node.js/Nuxt/Next/Express apps)
- "python" → python:3.12-slim (for Python/FastAPI/Django/Flask apps)
- "go"     → alpine:latest (for pre-compiled Go or Rust binaries)
- "static" → nginx:alpine (serves files via nginx, no start_command needed)

ERROR RECOVERY:
- If deploy fails, use get_logs to read container stderr
- If app crashes, check that it listens on process.env.PORT (or equivalent)
- If connection refused, check it binds to 0.0.0.0 not 127.0.0.1

EXAMPLES:
  Nuxt:    deploy_app({deploy_target: ".output", runtime: "node", start_command: "node server/index.mjs"})
  FastAPI: deploy_app({deploy_target: ".", runtime: "python", start_command: "uvicorn main:app --host 0.0.0.0 --port 8080"})
  Go:      deploy_app({deploy_target: "dist", runtime: "go", start_command: "./server"})
  Static:  deploy_app({deploy_target: "dist", runtime: "static"})`),

		mcp.WithString("deploy_target",
			mcp.Required(),
			mcp.Description("Absolute path to the build output directory. Its ENTIRE contents will be extracted to /app/ in the container."),
		),
		mcp.WithString("runtime",
			mcp.Required(),
			mcp.Description("Base container image: node, python, go, or static"),
		),
		mcp.WithString("start_command",
			mcp.Description("Command to start the app (paths relative to /app/). Required for all runtimes except static."),
		),
		mcp.WithString("app",
			mcp.Description("App slug to deploy to. If omitted, reads .hatch.toml or creates a new app."),
		),
		mcp.WithString("name",
			mcp.Description("App name for new apps (defaults to directory name)"),
		),
		mcp.WithString("domain",
			mcp.Description("Custom domain to configure (e.g. example.com)"),
		),
	)
}

func deployAppHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	deployTarget, err := req.RequireString("deploy_target")
	if err != nil {
		return toolError("failed to deploy app: missing required parameter 'deploy_target'")
	}

	if err := validateProjectPath(deployTarget); err != nil {
		return toolError("failed to deploy app: invalid deploy_target path: %v", err)
	}

	rt, err := req.RequireString("runtime")
	if err != nil {
		return toolError("failed to deploy app: missing required parameter 'runtime'")
	}

	startCmd := req.GetString("start_command", "")
	appSlug := req.GetString("app", "")
	name := req.GetString("name", "")

	// Validate runtime
	validRuntimes := map[string]bool{
		"node": true, "python": true, "go": true, "static": true,
	}
	if !validRuntimes[rt] {
		return toolError("failed to deploy app: unknown runtime %q (valid: node, python, go, static)", rt)
	}

	// Validate start command for non-static
	if rt != "static" && startCmd == "" {
		return toolError("failed to deploy app: start_command is required for runtime %q", rt)
	}

	// Validate deploy-target directory exists
	info, err := os.Stat(deployTarget)
	if err != nil || !info.IsDir() {
		return toolError("failed to deploy app: deploy_target directory not found: %s", deployTarget)
	}

	// Read and tar the directory
	artifact, err := createMCPTarGz(deployTarget)
	if err != nil {
		return toolError("failed to deploy app: creating artifact: %v", err)
	}

	// Auth
	client, err := newClient()
	if err != nil {
		return toolError("failed to deploy app: %v", err)
	}

	// Resolve app slug
	slug := appSlug
	if slug == "" {
		tomlPath := filepath.Join(".", ".hatch.toml")
		if data, err := os.ReadFile(tomlPath); err == nil {
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
		}
	}

	if slug == "" {
		appName := name
		if appName == "" {
			cwd, _ := os.Getwd()
			appName = filepath.Base(cwd)
		}

		app, err := client.CreateApp(appName)
		if err != nil {
			return toolError("failed to deploy app: %v", err)
		}
		slug = app.Slug

		tomlPath := filepath.Join(".", ".hatch.toml")
		content := fmt.Sprintf("[app]\nslug = %q\nname = %q\n", slug, appName)
		_ = os.WriteFile(tomlPath, []byte(content), 0644)
	}

	// Upload
	if err := client.UploadArtifact(slug, bytes.NewReader(artifact), rt, startCmd); err != nil {
		return toolError("failed to deploy app: upload failed: %v", err)
	}

	appURL := fmt.Sprintf("https://%s.nest.gethatch.eu", slug)
	return mcp.NewToolResultText(fmt.Sprintf("Deployed successfully!\nApp: %s\nURL: %s\nRuntime: %s", slug, appURL, rt)), nil
}

// createMCPTarGz creates a tar.gz from a directory for MCP deploy_app tool.
func createMCPTarGz(dir string) ([]byte, error) {
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, _ := filepath.Rel(dir, path)

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
	tw.Close()
	gzw.Close()

	if buf.Len() > 500*1024*1024 {
		return nil, fmt.Errorf("artifact too large (%.0f MB, max 500 MB)", float64(buf.Len())/1024/1024)
	}

	return buf.Bytes(), nil
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
		return toolError("failed to add database: missing required parameter 'app'")
	}

	client, err := newClient()
	if err != nil {
		return toolError("failed to add database: %v", err)
	}

	addon, err := client.AddAddon(slug, "postgresql")
	if err != nil {
		return toolError("failed to add database: %v", err)
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
		return toolError("failed to add storage: missing required parameter 'app'")
	}

	client, err := newClient()
	if err != nil {
		return toolError("failed to add storage: %v", err)
	}

	addon, err := client.AddAddon(slug, "s3")
	if err != nil {
		return toolError("failed to add storage: %v", err)
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
		return toolError("failed to get logs: missing required parameter 'app'")
	}

	lines := int(req.GetFloat("lines", 50))
	if lines <= 0 {
		lines = 50
	}

	client, err := newClient()
	if err != nil {
		return toolError("failed to get logs: %v", err)
	}

	logLines, err := client.GetLogs(slug, lines, "")
	if err != nil {
		return toolError("failed to get logs: %v", err)
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
		return toolError("failed to get app status: missing required parameter 'app'")
	}

	client, err := newClient()
	if err != nil {
		return toolError("failed to get app status: %v", err)
	}

	app, err := client.GetApp(slug)
	if err != nil {
		return toolError("failed to get app status: %v", err)
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
		return toolError("failed to set env var: missing required parameter 'app'")
	}
	key, err := req.RequireString("key")
	if err != nil {
		return toolError("failed to set env var: missing required parameter 'key'")
	}
	value, err := req.RequireString("value")
	if err != nil {
		return toolError("failed to set env var: missing required parameter 'value'")
	}

	client, err := newClient()
	if err != nil {
		return toolError("failed to set env var: %v", err)
	}

	if err := client.SetEnvVar(slug, key, value); err != nil {
		return toolError("failed to set env var: %v", err)
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
		return toolError("failed to get env vars: missing required parameter 'app'")
	}

	client, err := newClient()
	if err != nil {
		return toolError("failed to get env vars: %v", err)
	}

	vars, err := client.GetEnvVars(slug)
	if err != nil {
		return toolError("failed to get env vars: %v", err)
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
		return toolError("failed to add domain: missing required parameter 'app'")
	}
	domain, err := req.RequireString("domain")
	if err != nil {
		return toolError("failed to add domain: missing required parameter 'domain'")
	}

	// Validate domain format to reject path traversal and injection characters
	if err := api.ValidateDomain(domain); err != nil {
		return toolError("failed to add domain: %v", err)
	}

	client, err := newClient()
	if err != nil {
		return toolError("failed to add domain: %v", err)
	}

	d, err := client.AddDomain(slug, domain)
	if err != nil {
		return toolError("failed to add domain: %v", err)
	}

	cname := d.CNAME
	if cname == "" {
		cname = slug + ".nest.gethatch.eu"
	}

	result := fmt.Sprintf(`Domain %s configured for %s.
Status: %s

DNS Setup:
Add a CNAME record pointing %s to %s

  Type   Name    Value
  CNAME  @       %s
  CNAME  www     %s

For apex domains (e.g. example.com without www), CNAME records are not
allowed by the DNS spec. Use ALIAS or ANAME if your provider supports it
(Cloudflare, Route 53, DNSimple). Otherwise use a www subdomain.

SSL is provisioned automatically via Let's Encrypt once DNS propagates.

To verify DNS is configured correctly, run:
  dig +short CNAME %s
Expected: %s (or similar)
Or for apex domains with A records:
  dig +short A %s
The A record should resolve to the Hatch server IP.

Tell the user to configure DNS, then re-run the dig command to confirm.`,
		domain, slug, d.Status, domain, cname, cname, cname,
		domain, cname, domain)

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
		return toolError("failed to get database url: missing required parameter 'app'")
	}

	client, err := newClient()
	if err != nil {
		return toolError("failed to get database url: %v", err)
	}

	vars, err := client.GetEnvVars(slug)
	if err != nil {
		return toolError("failed to get database url: %v", err)
	}

	for _, v := range vars {
		if v.Key == "DATABASE_URL" {
			return mcp.NewToolResultText(v.Value), nil
		}
	}

	return toolError("failed to get database url: no DATABASE_URL found, add a database first with add_database")
}

// --- restart_app ---

func restartAppTool() mcp.Tool {
	return mcp.NewTool("restart_app",
		mcp.WithDescription("Restart an app. Use after changing environment variables."),
		mcp.WithString("app",
			mcp.Required(),
			mcp.Description("App slug (name) to restart"),
		),
	)
}

func restartAppHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	slug, err := req.RequireString("app")
	if err != nil {
		return toolError("failed to restart app: missing required parameter 'app'")
	}

	client, err := newClient()
	if err != nil {
		return toolError("failed to restart app: %v", err)
	}

	if err := client.RestartApp(slug); err != nil {
		return toolError("failed to restart app: %v", err)
	}

	return mcp.NewToolResultText(fmt.Sprintf("App '%s' restarted successfully", slug)), nil
}

// --- delete_env ---

func deleteEnvTool() mcp.Tool {
	return mcp.NewTool("delete_env",
		mcp.WithDescription("Remove an environment variable from an app"),
		mcp.WithString("app",
			mcp.Required(),
			mcp.Description("App slug (name) to remove the variable from"),
		),
		mcp.WithString("key",
			mcp.Required(),
			mcp.Description("Environment variable name to remove"),
		),
	)
}

func deleteEnvHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	slug, err := req.RequireString("app")
	if err != nil {
		return toolError("failed to delete env var: missing required parameter 'app'")
	}
	key, err := req.RequireString("key")
	if err != nil {
		return toolError("failed to delete env var: missing required parameter 'key'")
	}

	// Validate env key to prevent URL path injection
	if err := api.ValidateEnvKey(key); err != nil {
		return toolError("failed to delete env var: %v", err)
	}

	client, err := newClient()
	if err != nil {
		return toolError("failed to delete env var: %v", err)
	}

	if err := client.UnsetEnvVar(slug, key); err != nil {
		return toolError("failed to delete env var: %v", err)
	}

	return mcp.NewToolResultText(fmt.Sprintf("Environment variable '%s' removed from '%s'", key, slug)), nil
}

// --- list_domains ---

func listDomainsTool() mcp.Tool {
	return mcp.NewTool("list_domains",
		mcp.WithDescription("List custom domains configured for an app"),
		mcp.WithString("app",
			mcp.Required(),
			mcp.Description("App slug (name) to list domains for"),
		),
	)
}

func listDomainsHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	slug, err := req.RequireString("app")
	if err != nil {
		return toolError("failed to list domains: missing required parameter 'app'")
	}

	client, err := newClient()
	if err != nil {
		return toolError("failed to list domains: %v", err)
	}

	domains, err := client.ListDomains(slug)
	if err != nil {
		return toolError("failed to list domains: %v", err)
	}

	if len(domains) == 0 {
		return mcp.NewToolResultText("No custom domains configured."), nil
	}

	data, _ := json.MarshalIndent(domains, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

// --- remove_domain ---

func removeDomainTool() mcp.Tool {
	return mcp.NewTool("remove_domain",
		mcp.WithDescription("Remove a custom domain from an app"),
		mcp.WithString("app",
			mcp.Required(),
			mcp.Description("App slug (name) to remove the domain from"),
		),
		mcp.WithString("domain",
			mcp.Required(),
			mcp.Description("Custom domain name to remove"),
		),
	)
}

func removeDomainHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	slug, err := req.RequireString("app")
	if err != nil {
		return toolError("failed to remove domain: missing required parameter 'app'")
	}
	domain, err := req.RequireString("domain")
	if err != nil {
		return toolError("failed to remove domain: missing required parameter 'domain'")
	}

	// Validate domain to prevent URL path injection
	if err := api.ValidateDomain(domain); err != nil {
		return toolError("failed to remove domain: %v", err)
	}

	client, err := newClient()
	if err != nil {
		return toolError("failed to remove domain: %v", err)
	}

	if err := client.RemoveDomain(slug, domain); err != nil {
		return toolError("failed to remove domain: %v", err)
	}

	return mcp.NewToolResultText(fmt.Sprintf("Domain '%s' removed from '%s'", domain, slug)), nil
}

// --- get_build_logs ---

func getBuildLogsTool() mcp.Tool {
	return mcp.NewTool("get_build_logs",
		mcp.WithDescription("Get build logs for an app. Use to diagnose deploy failures."),
		mcp.WithString("app",
			mcp.Required(),
			mcp.Description("App slug (name) to get build logs for"),
		),
		mcp.WithNumber("lines",
			mcp.Description("Number of recent build log lines to return (default 100)"),
		),
	)
}

func getBuildLogsHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	slug, err := req.RequireString("app")
	if err != nil {
		return toolError("failed to get build logs: missing required parameter 'app'")
	}

	lines := int(req.GetFloat("lines", 100))
	if lines <= 0 {
		lines = 100
	}

	client, err := newClient()
	if err != nil {
		return toolError("failed to get build logs: %v", err)
	}

	logLines, err := client.GetLogs(slug, lines, "build")
	if err != nil {
		return toolError("failed to get build logs: %v", err)
	}

	if len(logLines) == 0 {
		return mcp.NewToolResultText("No build logs found."), nil
	}

	return mcp.NewToolResultText(strings.Join(logLines, "\n")), nil
}

// --- create_app ---

func createAppTool() mcp.Tool {
	return mcp.NewTool("create_app",
		mcp.WithDescription("Create a new app without deploying. Returns the app slug and URL. Use when you need to configure env vars before first deploy."),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("App name"),
		),
	)
}

func createAppHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, err := req.RequireString("name")
	if err != nil {
		return toolError("failed to create app: missing required parameter 'name'")
	}

	client, err := newClient()
	if err != nil {
		return toolError("failed to create app: %v", err)
	}

	app, err := client.CreateApp(name)
	if err != nil {
		return toolError("failed to create app: %v", err)
	}

	result := map[string]string{
		"slug": app.Slug,
		"name": app.Name,
		"url":  fmt.Sprintf("https://%s.nest.gethatch.eu", app.Slug),
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

// --- delete_app ---

func deleteAppTool() mcp.Tool {
	return mcp.NewTool("delete_app",
		mcp.WithDescription("Permanently delete an app and all its resources. This action cannot be undone."),
		mcp.WithString("app",
			mcp.Required(),
			mcp.Description("App slug to delete"),
		),
		mcp.WithBoolean("confirm",
			mcp.Required(),
			mcp.Description("Must be true to confirm deletion"),
		),
	)
}

func deleteAppHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	slug, err := req.RequireString("app")
	if err != nil {
		return toolError("failed to delete app: missing required parameter 'app'")
	}

	confirm := req.GetBool("confirm", false)
	if !confirm {
		return toolError("failed to delete app: confirm must be set to true")
	}

	client, err := newClient()
	if err != nil {
		return toolError("failed to delete app: %v", err)
	}

	if err := client.DeleteApp(slug); err != nil {
		return toolError("failed to delete app: %v", err)
	}

	return mcp.NewToolResultText(fmt.Sprintf("App '%s' has been permanently deleted", slug)), nil
}

// --- check_auth ---

func checkAuthTool() mcp.Tool {
	return mcp.NewTool("check_auth",
		mcp.WithDescription("Check if authentication is configured. Use before other operations to verify credentials."),
	)
}

func checkAuthHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	token, err := getTokenFunc()
	if err != nil {
		return toolError("failed to check auth: %v", err)
	}

	if token == "" {
		return toolError("failed to check auth: not authenticated - run 'hatch login', set HATCH_TOKEN, or use --token")
	}

	// Determine token source
	var source string
	if os.Getenv("HATCH_TOKEN") != "" {
		source = "HATCH_TOKEN env"
	} else {
		source = "config file"
	}

	return mcp.NewToolResultText(fmt.Sprintf("Authenticated. Token source: %s", source)), nil
}

// --- get_app_details ---

func getAppDetailsTool() mcp.Tool {
	return mcp.NewTool("get_app_details",
		mcp.WithDescription("Get detailed app information including deployment history, framework, and domains"),
		mcp.WithString("app",
			mcp.Required(),
			mcp.Description("App slug (name) to get details for"),
		),
	)
}

func getAppDetailsHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	slug, err := req.RequireString("app")
	if err != nil {
		return toolError("failed to get app details: missing required parameter 'app'")
	}

	client, err := newClient()
	if err != nil {
		return toolError("failed to get app details: %v", err)
	}

	app, err := client.GetApp(slug)
	if err != nil {
		return toolError("failed to get app details: %v", err)
	}

	data, _ := json.MarshalIndent(app, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

// --- health_check ---

func healthCheckTool() mcp.Tool {
	return mcp.NewTool("health_check",
		mcp.WithDescription("Check if an app is responding by hitting its public URL. Returns HTTP status and response time."),
		mcp.WithString("app",
			mcp.Required(),
			mcp.Description("App slug (name) to health check"),
		),
	)
}

func healthCheckHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	slug, err := req.RequireString("app")
	if err != nil {
		return toolError("failed to health check: missing required parameter 'app'")
	}

	// Validate slug to prevent SSRF
	if err := api.ValidateSlug(slug); err != nil {
		return toolError("failed to health check: %v", err)
	}

	// Create a separate HTTP client with short timeout for health checks
	healthClient := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return nil // Follow redirects
		},
	}

	url := fmt.Sprintf("https://%s.nest.gethatch.eu", slug)
	start := time.Now()

	resp, err := healthClient.Get(url)
	elapsed := time.Since(start)

	if err != nil {
		result := map[string]interface{}{
			"url":     url,
			"error":   redactError(err.Error()),
			"healthy": false,
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
	defer resp.Body.Close()

	result := map[string]interface{}{
		"url":              url,
		"status_code":      resp.StatusCode,
		"response_time_ms": elapsed.Milliseconds(),
		"healthy":          resp.StatusCode >= 200 && resp.StatusCode < 500,
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

// --- set_env_var ---

func setEnvVarTool() mcp.Tool {
	return mcp.NewTool("set_env_var",
		mcp.WithDescription("Set a single environment variable on an app. Alias for set_env with clearer naming."),
		mcp.WithString("slug",
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

func setEnvVarHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	slug, err := req.RequireString("slug")
	if err != nil {
		return toolError("failed to set env var: missing required parameter 'slug'")
	}
	key, err := req.RequireString("key")
	if err != nil {
		return toolError("failed to set env var: missing required parameter 'key'")
	}
	value, err := req.RequireString("value")
	if err != nil {
		return toolError("failed to set env var: missing required parameter 'value'")
	}

	client, err := newClient()
	if err != nil {
		return toolError("failed to set env var: %v", err)
	}

	if err := client.SetEnvVar(slug, key, value); err != nil {
		return toolError("failed to set env var: %v", err)
	}

	return mcp.NewToolResultText(fmt.Sprintf("Set %s on %s.", key, slug)), nil
}

// --- list_env_vars ---

func listEnvVarsTool() mcp.Tool {
	return mcp.NewTool("list_env_vars",
		mcp.WithDescription("List all environment variables for an app. Alias for get_env with clearer naming."),
		mcp.WithString("slug",
			mcp.Required(),
			mcp.Description("App slug (name) to list env vars for"),
		),
	)
}

func listEnvVarsHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	slug, err := req.RequireString("slug")
	if err != nil {
		return toolError("failed to list env vars: missing required parameter 'slug'")
	}

	client, err := newClient()
	if err != nil {
		return toolError("failed to list env vars: %v", err)
	}

	vars, err := client.GetEnvVars(slug)
	if err != nil {
		return toolError("failed to list env vars: %v", err)
	}

	if len(vars) == 0 {
		return mcp.NewToolResultText("No environment variables set."), nil
	}

	data, _ := json.MarshalIndent(vars, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

// --- bulk_set_env ---

func bulkSetEnvTool() mcp.Tool {
	return mcp.NewTool("bulk_set_env",
		mcp.WithDescription("Set multiple environment variables at once. More efficient than calling set_env repeatedly."),
		mcp.WithString("app",
			mcp.Required(),
			mcp.Description("App slug (name) to set variables on"),
		),
	)
}

func bulkSetEnvHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	slug, err := req.RequireString("app")
	if err != nil {
		return toolError("failed to bulk set env vars: missing required parameter 'app'")
	}

	args := req.GetArguments()
	varsParam, ok := args["vars"]
	if !ok {
		return toolError("failed to bulk set env vars: missing required parameter 'vars'")
	}

	varsMap, ok := varsParam.(map[string]interface{})
	if !ok {
		return toolError("failed to bulk set env vars: 'vars' must be an object with string values")
	}

	if len(varsMap) == 0 {
		return toolError("failed to bulk set env vars: 'vars' cannot be empty")
	}

	client, err := newClient()
	if err != nil {
		return toolError("failed to bulk set env vars: %v", err)
	}

	var successKeys []string
	var errors []string

	for key, value := range varsMap {
		valueStr, ok := value.(string)
		if !ok {
			errors = append(errors, fmt.Sprintf("%s: value must be a string", key))
			continue
		}

		if err := client.SetEnvVar(slug, key, valueStr); err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", key, err))
		} else {
			successKeys = append(successKeys, key)
		}
	}

	// Build result message
	var result strings.Builder
	if len(successKeys) > 0 {
		result.WriteString(fmt.Sprintf("Set %d environment variables on '%s': %s",
			len(successKeys), slug, strings.Join(successKeys, ", ")))
	}

	if len(errors) > 0 {
		if result.Len() > 0 {
			result.WriteString("\n\nErrors:\n")
		}
		result.WriteString(redactError(strings.Join(errors, "\n")))
	}

	if len(successKeys) == 0 && len(errors) > 0 {
		return mcp.NewToolResultError(redactError(result.String())), nil
	}

	return mcp.NewToolResultText(result.String()), nil
}

// --- check_energy ---

func checkEnergyTool() mcp.Tool {
	return mcp.NewTool("check_energy",
		mcp.WithDescription("Get account-level energy status: daily/weekly limits, remaining energy, active eggs, and always-on/boosted eggs."),
	)
}

func checkEnergyHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := newClient()
	if err != nil {
		return toolError("failed to check energy: %v", err)
	}

	energy, err := client.GetAccountEnergy()
	if err != nil {
		return toolError("failed to check energy: %v", err)
	}

	result := fmt.Sprintf("Tier: %s\nDaily Energy: %d / %d min remaining\nWeekly Energy: %d / %d min remaining\nResets At: %s\nActive Eggs: %d\nSleeping Eggs: %d\nEggs Limit: %d",
		energy.Tier,
		energy.DailyRemaining, energy.DailyLimit,
		energy.WeeklyRemaining, energy.WeeklyLimit,
		energy.ResetsAt,
		energy.EggsActive, energy.EggsSleeping, energy.EggsLimit)

	if len(energy.AlwaysOnEggs) > 0 {
		result += fmt.Sprintf("\nAlways-On Eggs: %s", strings.Join(energy.AlwaysOnEggs, ", "))
	}
	if len(energy.BoostedEggs) > 0 {
		result += fmt.Sprintf("\nBoosted Eggs: %s", strings.Join(energy.BoostedEggs, ", "))
	}

	return mcp.NewToolResultText(result), nil
}

// --- get_app_energy ---

func getAppEnergyTool() mcp.Tool {
	return mcp.NewTool("get_app_energy",
		mcp.WithDescription("Get energy status for a specific app: daily/weekly usage, limits, boost status, and reset times."),
		mcp.WithString("app",
			mcp.Required(),
			mcp.Description("App slug (name) to check energy for"),
		),
	)
}

func getAppEnergyHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	slug, err := req.RequireString("app")
	if err != nil {
		return toolError("failed to get app energy: missing required parameter 'app'")
	}

	client, err := newClient()
	if err != nil {
		return toolError("failed to get app energy: %v", err)
	}

	energy, err := client.GetAppEnergy(slug)
	if err != nil {
		return toolError("failed to get app energy: %v", err)
	}

	result := fmt.Sprintf("App: %s\nStatus: %s\nPlan: %s\nAlways On: %v\nBoosted: %v\n"+
		"Daily: %d / %d min used (%d remaining)\nWeekly: %d / %d min used (%d remaining)\n"+
		"Daily Resets At: %s\nWeekly Resets At: %s",
		energy.Slug, energy.Status, energy.Plan,
		energy.AlwaysOn, energy.Boosted,
		energy.DailyUsedMin, energy.DailyLimitMin, energy.DailyRemainingMin,
		energy.WeeklyUsedMin, energy.WeeklyLimitMin, energy.WeeklyRemainingMin,
		energy.DailyResetsAt, energy.WeeklyResetsAt)

	if energy.BoostExpiresAt != nil {
		result += fmt.Sprintf("\nBoost Expires At: %s", *energy.BoostExpiresAt)
	}
	if energy.BonusEnergy > 0 {
		result += fmt.Sprintf("\nBonus Energy: %d min", energy.BonusEnergy)
	}

	return mcp.NewToolResultText(result), nil
}

// --- boost_app ---

func boostAppTool() mcp.Tool {
	return mcp.NewTool("boost_app",
		mcp.WithDescription("Get a Stripe checkout URL to boost an app's energy. Returns a payment link — the user must complete payment in their browser."),
		mcp.WithString("app",
			mcp.Required(),
			mcp.Description("App slug (name) to boost"),
		),
		mcp.WithString("duration",
			mcp.Description("Boost duration: 'day' for 24 hours (€1) or 'week' for 7 days (€5). Defaults to 'day'."),
		),
	)
}

func boostAppHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	slug, err := req.RequireString("app")
	if err != nil {
		return toolError("failed to boost app: missing required parameter 'app'")
	}

	duration := "day"
	args := req.GetArguments()
	if d, ok := args["duration"].(string); ok && d != "" {
		duration = d
	}

	client, err := newClient()
	if err != nil {
		return toolError("failed to boost app: %v", err)
	}

	checkout, err := client.BoostCheckout(slug, duration)
	if err != nil {
		return toolError("failed to boost app: %v", err)
	}

	result := fmt.Sprintf("Boost checkout created for %s:\nDuration: %s\nPrice: %s\nCheckout URL: %s\n\nOpen this URL in your browser to complete payment.",
		slug, checkout.Duration, checkout.AmountEur, checkout.CheckoutURL)

	return mcp.NewToolResultText(result), nil
}
