package mcpserver

// SkillMD contains the Hatch platform technical reference for AI agents.
// This is served as an MCP resource at hatch://skill.
const SkillMD = `# Hatch Technical Reference

Deploy flow, runtimes, environment variables, and common issues.
Optimized for AI agents.

## Deploy Flow

1. Build the project locally (npm run build, go build, etc.)
2. Deploy the build output directory:

` + "```" + `bash
hatch deploy --deploy-target <build-dir> --runtime <node|python|go|rust|php|bun|static> --start-command "<cmd>"
` + "```" + `

Or via MCP:

` + "```" + `
deploy_app({ deploy_target: "/path/to/build", runtime: "node", start_command: "node server/index.mjs" })
` + "```" + `

## Runtimes

| Runtime  | Base Image        | For                                    |
|----------|-------------------|----------------------------------------|
| node     | node:20-alpine    | Nuxt, Next, Express, any Node.js app   |
| python   | python:3.12-slim  | FastAPI, Django, Flask, any Python app  |
| go       | alpine:latest     | Pre-compiled Go binaries               |
| rust     | alpine:latest     | Pre-compiled Rust binaries             |
| php      | php:8.3-apache    | Laravel, Symfony, WordPress, any PHP   |
| bun      | oven/bun:1-alpine | Elysia, Hono, any Bun app              |
| static   | nginx:alpine      | Static HTML/CSS/JS (no start command)  |

## What goes in deploy-target

The deploy-target directory should contain everything needed at runtime:

| Project Type | Build Command      | deploy-target | start-command                |
|--------------|--------------------|---------------|------------------------------|
| Nuxt 3       | pnpm build         | .output       | node server/index.mjs        |
| Next.js      | npm run build      | .next         | npx next start               |
| Express      | (none)             | .             | node index.js                |
| FastAPI      | (none)             | .             | uvicorn main:app --host 0.0.0.0 --port 8080 |
| Go           | go build -o dist/  | dist          | ./server                     |
| Rust         | cargo build --release | target/release | ./server                  |
| PHP/Laravel  | composer install   | .             | apache2-foreground           |
| Bun/Elysia   | bun install        | .             | bun run index.ts             |
| Static site  | npm run build      | dist          | (not needed)                 |

IMPORTANT: Include node_modules if your Node.js app needs them at runtime.
The deploy-target contents are extracted to /app/ in the container.

## Environment Variables (auto-injected)

| Variable        | Description                                       |
|-----------------|---------------------------------------------------|
| PORT            | Always 8080. Your app must listen on this port.   |
| DATABASE_URL    | PostgreSQL connection string (if provisioned).    |

## Common Issues

| Error             | Cause                        | Fix                                    |
|-------------------|------------------------------|----------------------------------------|
| App crashed       | Not listening on PORT        | Use process.env.PORT (or equivalent)   |
| Connection refused| Listening on localhost       | Bind to 0.0.0.0 not 127.0.0.1         |
| Exit code 139     | Out of memory                | Reduce memory usage                    |
| Missing module    | node_modules not in artifact | Include node_modules in deploy-target  |

## MCP Tools

| Tool | Description |
|---|---|
| ` + "`deploy_app`" + ` | Deploy a pre-built directory (tar + upload) |
| ` + "`get_platform_info`" + ` | Runtimes, artifact format, platform constraints |
| ` + "`list_apps`" + ` | List all your deployed apps |
| ` + "`add_database`" + ` | Provisions PostgreSQL, injects DATABASE_URL |
| ` + "`add_storage`" + ` | S3-compatible bucket |
| ` + "`get_logs`" + ` | Returns recent application logs |
| ` + "`get_status`" + ` | App running status, URL, region |
| ` + "`set_env`" + ` | Set environment variables |
| ` + "`get_env`" + ` | List all environment variables |
| ` + "`add_domain`" + ` | Custom domain setup with DNS instructions |
| ` + "`get_database_url`" + ` | Get DATABASE_URL for an app |
`

// ClaudeMDContent is the user-facing Hatch deployment guide written by `hatch init`.
const ClaudeMDContent = `## Hatch Deployment

This project deploys to [Hatch](https://gethatch.eu), an EU-hosted PaaS.

### Deploy
Build your project, then deploy the output:
` + "```" + `bash
hatch deploy --deploy-target <build-dir> --runtime <node|python|go|rust|php|bun|static> --start-command "<cmd>"
` + "```" + `

### Runtimes
| Runtime | For | Example start command |
|---------|-----|----------------------|
| node | Nuxt, Next, Express | ` + "`node server/index.mjs`" + ` |
| python | FastAPI, Django, Flask | ` + "`uvicorn main:app --host 0.0.0.0 --port 8080`" + ` |
| go | Go binaries | ` + "`./server`" + ` |
| rust | Rust binaries | ` + "`./server`" + ` |
| php | Laravel, Symfony, WordPress | ` + "`apache2-foreground`" + ` |
| bun | Elysia, Hono, Bun apps | ` + "`bun run index.ts`" + ` |
| static | HTML/CSS/JS | (none needed) |

### Environment Variables (auto-injected)
- ` + "`PORT`" + ` — Always 8080. Your app must listen on this port.
- ` + "`DATABASE_URL`" + ` — PostgreSQL connection string (if provisioned via ` + "`hatch db`" + ` or MCP ` + "`add_database`" + `).

### MCP Tools (via ` + "`hatch mcp`" + `)
Use these tools to manage your deployment: ` + "`deploy_app`" + `, ` + "`get_logs`" + `, ` + "`get_status`" + `, ` + "`restart_app`" + `, ` + "`set_env`" + `, ` + "`add_database`" + `, ` + "`add_domain`" + `.

Run ` + "`hatch mcp`" + ` or configure in ` + "`.claude/settings.json`" + `:
` + "```" + `json
{ "mcpServers": { "hatch": { "command": "hatch", "args": ["mcp"] } } }
` + "```" + `
`
