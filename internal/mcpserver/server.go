package mcpserver

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/auth"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// NewServer creates the Hatch MCP server with all tools registered.
func NewServer() *server.MCPServer {
	s := server.NewMCPServer(
		"hatch",
		"1.0.0",
		server.WithToolCapabilities(false),
	)

	s.AddTool(deployAppTool(), deployAppHandler)
	s.AddTool(addDatabaseTool(), addDatabaseHandler)
	s.AddTool(addStorageTool(), addStorageHandler)
	s.AddTool(viewLogsTool(), viewLogsHandler)
	s.AddTool(checkStatusTool(), checkStatusHandler)
	s.AddTool(setSecretTool(), setSecretHandler)
	s.AddTool(connectDomainTool(), connectDomainHandler)
	s.AddTool(getDatabaseURLTool(), getDatabaseURLHandler)

	return s
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

// --- deploy_app ---

func deployAppTool() mcp.Tool {
	return mcp.NewTool("deploy_app",
		mcp.WithDescription("Deploy an application directory to Hatch. Initializes git if needed, commits changes, and pushes to Hatch. Returns the live URL."),
		mcp.WithString("directory",
			mcp.Required(),
			mcp.Description("Absolute path to the application directory to deploy"),
		),
		mcp.WithString("name",
			mcp.Description("Custom app name (defaults to directory name)"),
		),
	)
}

func deployAppHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	dir, err := req.RequireString("directory")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	name := req.GetString("name", "")
	if name == "" {
		name = filepath.Base(dir)
	}

	token, err := auth.GetToken()
	if err != nil || token == "" {
		return mcp.NewToolResultError("Not logged in. Run 'hatch login', set HATCH_TOKEN, or use --token."), nil
	}

	// Ensure directory exists
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return mcp.NewToolResultError(fmt.Sprintf("Directory not found: %s", dir)), nil
	}

	// Initialize git if needed
	if err := runGit(dir, "rev-parse", "--git-dir"); err != nil {
		if err := runGit(dir, "init"); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("git init failed: %v", err)), nil
		}
	}

	// Stage and commit any changes
	_ = runGit(dir, "add", "-A")
	// Commit (ignore error if nothing to commit)
	_ = runGit(dir, "commit", "-m", "Deploy via hatch")

	// Set up hatch remote (token as username for Basic Auth)
	remoteURL := fmt.Sprintf("https://%s:x@git.gethatch.eu/deploy/%s.git", token, name)
	if err := runGit(dir, "remote", "get-url", "hatch"); err != nil {
		_ = runGit(dir, "remote", "add", "hatch", remoteURL)
	} else {
		_ = runGit(dir, "remote", "set-url", "hatch", remoteURL)
	}

	// Get current branch
	branch, err := gitOutput(dir, "branch", "--show-current")
	if err != nil || branch == "" {
		branch = "main"
	}

	// Push
	output, err := gitOutput(dir, "push", "--force", "hatch", branch+":main")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Deploy failed: %s", output)), nil
	}

	appURL := fmt.Sprintf("https://%s.gethatch.eu", name)
	return mcp.NewToolResultText(fmt.Sprintf("Deployed successfully!\nApp URL: %s\nApp name: %s", appURL, name)), nil
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

// --- view_logs ---

func viewLogsTool() mcp.Tool {
	return mcp.NewTool("view_logs",
		mcp.WithDescription("View recent application logs."),
		mcp.WithString("app",
			mcp.Required(),
			mcp.Description("App slug (name) to view logs for"),
		),
		mcp.WithNumber("lines",
			mcp.Description("Number of recent log lines to return (default 50)"),
		),
	)
}

func viewLogsHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

// --- check_status ---

func checkStatusTool() mcp.Tool {
	return mcp.NewTool("check_status",
		mcp.WithDescription("Check if an app is running, its URL, and current status."),
		mcp.WithString("app",
			mcp.Required(),
			mcp.Description("App slug (name) to check"),
		),
	)
}

func checkStatusHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

// --- set_secret ---

func setSecretTool() mcp.Tool {
	return mcp.NewTool("set_secret",
		mcp.WithDescription("Set an environment variable (secret) on a deployed app."),
		mcp.WithString("app",
			mcp.Required(),
			mcp.Description("App slug (name) to set the secret on"),
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

func setSecretHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
		return mcp.NewToolResultError(fmt.Sprintf("Failed to set secret: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Set %s on %s.", key, slug)), nil
}

// --- connect_domain ---

func connectDomainTool() mcp.Tool {
	return mcp.NewTool("connect_domain",
		mcp.WithDescription("Connect a custom domain to an app. Returns DNS instructions."),
		mcp.WithString("app",
			mcp.Required(),
			mcp.Description("App slug (name) to connect the domain to"),
		),
		mcp.WithString("domain",
			mcp.Required(),
			mcp.Description("Custom domain name (e.g. example.com)"),
		),
	)
}

func connectDomainHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
		return mcp.NewToolResultError(fmt.Sprintf("Failed to connect domain: %v", err)), nil
	}

	cname := d.CNAME
	if cname == "" {
		cname = slug + ".gethatch.eu"
	}

	result := fmt.Sprintf("Domain %s configured for %s.\nStatus: %s\n\nDNS Setup:\nAdd a CNAME record pointing %s to %s",
		domain, slug, d.Status, domain, cname)

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

// --- git helpers ---

func runGit(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
