package mcpserver

// SkillMD contains the Hatch platform technical reference for AI agents.
// This is served as an MCP resource at hatch://skill.
const SkillMD = `# Hatch Technical Reference

Deploy flow, runtime detection, environment variables, per-runtime preparation, and common issues.
This page is optimized for Claude and other AI agents.

## Deploy Flow

Hatch uses a single command to deploy. No GitHub integration or web UI required.

` + "```" + `bash
# 1. Install the CLI (requires Go 1.21+)
go install github.com/EscapeVelocityOperations/hatch-cli@latest

# 2. Authenticate
hatch login

# 3. Deploy (creates app, commits changes, and deploys)
hatch deploy
` + "```" + `

The ` + "`deploy`" + ` command handles everything:
- Initializes git if the directory isn't a repo
- Auto-commits any uncommitted changes
- Creates the app on Hatch (using directory name by default)
- Pushes to the Hatch git remote
- Returns a live URL

Every subsequent ` + "`hatch deploy`" + ` triggers a new deployment.

## MCP Deploy (alternative)

If the Hatch MCP server is configured, you can deploy via MCP tools:

` + "```" + `bash
# Start the MCP server
hatch mcp
` + "```" + `

MCP server config for AI agents:
` + "```" + `json
{
  "mcpServers": {
    "hatch": {
      "command": "hatch",
      "args": ["mcp"]
    }
  }
}
` + "```" + `

Then use:
` + "```" + `
deploy_app({ artifact_path: "/path/to/artifact.tar.gz", framework: "node", start_command: "node index.js" })
` + "```" + `

## Runtime Detection

Hatch auto-detects the runtime based on which manifest file is present in the repository root.

| File Present | Runtime | Start Command |
|---|---|---|
| ` + "`package.json`" + ` | Node.js 20 | ` + "`npm start`" + ` |
| ` + "`requirements.txt`" + ` | Python 3.11 | ` + "`python app.py`" + ` or Procfile |
| ` + "`go.mod`" + ` | Go 1.21 | ` + "`go run .`" + ` |
| ` + "`Cargo.toml`" + ` | Rust | ` + "`cargo run --release`" + ` |
| ` + "`index.html`" + ` (only) | Static | Served via Caddy |

## Environment Variables (auto-injected)

These environment variables are automatically available to every deployed app:

| Variable | Description |
|---|---|
| ` + "`PORT`" + ` | Always ` + "`8080`" + `. Your app must listen on this port. |
| ` + "`DATABASE_URL`" + ` | PostgreSQL connection string, if a database is provisioned. |
| ` + "`REDIS_URL`" + ` | Redis connection string, if Redis is provisioned. |
| ` + "`HATCH_APP_NAME`" + ` | The name of the deployed application. |
| ` + "`HATCH_REGION`" + ` | The deployment region. |

## Preparing Apps Per Runtime

Each runtime requires your app to listen on the ` + "`PORT`" + ` environment variable and bind to ` + "`0.0.0.0`" + `.

### Node.js

` + "```" + `javascript
const port = process.env.PORT || 3000;
app.listen(port, '0.0.0.0', () => {
  console.log(` + "`" + `Listening on port ${port}` + "`" + `);
});
` + "```" + `

### Python

` + "```" + `python
import os

port = int(os.environ.get('PORT', 5000))
app.run(host='0.0.0.0', port=port)
` + "```" + `

### Go

` + "```" + `go
port := os.Getenv("PORT")
if port == "" {
    port = "8080"
}
http.ListenAndServe(":"+port, nil)
` + "```" + `

### Rust

` + "```" + `rust
let port = std::env::var("PORT").unwrap_or_else(|_| "8080".to_string());
let addr = format!("0.0.0.0:{}", port);
// bind to addr
` + "```" + `

## Common Issues

| Error | Cause | Fix |
|---|---|---|
| App crashed | Not listening on PORT | Use ` + "`process.env.PORT`" + ` (or equivalent) |
| Build failed | Missing dependencies | Check manifest file (package.json, requirements.txt, etc.) |
| Connection refused | Listening on localhost | Listen on ` + "`0.0.0.0`" + ` instead of ` + "`127.0.0.1`" + ` |

## CLI Reference

| Command | Description |
|---------|-------------|
| ` + "`hatch deploy`" + ` | Deploy current directory (creates app if needed) |
| ` + "`hatch deploy --name NAME`" + ` | Deploy with custom app name |
| ` + "`hatch deploy --domain DOMAIN`" + ` | Deploy and configure custom domain |
| ` + "`hatch apps`" + ` | List all your apps |
| ` + "`hatch info SLUG`" + ` | Show app details and status |
| ` + "`hatch logs SLUG`" + ` | View app logs |
| ` + "`hatch open SLUG`" + ` | Open app in browser |
| ` + "`hatch restart SLUG`" + ` | Restart the app |
| ` + "`hatch destroy SLUG`" + ` | Delete the app |
| ` + "`hatch env set KEY=VAL`" + ` | Set environment variable |
| ` + "`hatch env unset KEY`" + ` | Remove environment variable |
| ` + "`hatch domain add DOMAIN`" + ` | Add custom domain |
| ` + "`hatch domain list`" + ` | List domains |
| ` + "`hatch domain remove DOMAIN`" + ` | Remove custom domain |
| ` + "`hatch db connect SLUG`" + ` | Open local proxy to app database |
| ` + "`hatch login`" + ` | Authenticate via browser |
| ` + "`hatch logout`" + ` | Clear local credentials |
| ` + "`hatch mcp`" + ` | Start MCP server for AI agents |
| ` + "`hatch version`" + ` | Show CLI version |

## MCP Tools

Available MCP tools when the Hatch MCP server is configured:

| Tool | Description |
|---|---|
| ` + "`deploy_app`" + ` | Build artifact upload + deploy (creates app if needed) |
| ` + "`get_platform_info`" + ` | Supported frameworks, artifact format, platform constraints |
| ` + "`analyze_project`" + ` | Detect framework, build command, output dir for a project |
| ` + "`list_apps`" + ` | List all your deployed apps |
| ` + "`add_database`" + ` | Provisions PostgreSQL, injects DATABASE_URL |
| ` + "`add_storage`" + ` | S3-compatible bucket |
| ` + "`get_logs`" + ` | Returns recent application logs |
| ` + "`get_status`" + ` | App running status, URL, region |
| ` + "`set_env`" + ` | Set environment variables |
| ` + "`get_env`" + ` | List all environment variables |
| ` + "`add_domain`" + ` | Custom domain setup with DNS instructions |
| ` + "`get_database_url`" + ` | Get DATABASE_URL for an app |

## For AI Agents

When a user says "deploy this", guide them through:
1. ` + "`hatch login`" + ` (if not authenticated)
2. ` + "`hatch deploy`" + `
3. Share the live URL

Ensure the app listens on PORT and binds to 0.0.0.0. If deployment fails, read logs with ` + "`hatch logs SLUG`" + `, fix, and deploy again.
`
