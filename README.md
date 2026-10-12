# hatch-cli

Command-line interface for deploying and managing applications on the [Hatch](https://gethatch.eu) platform.

## Installation

### Claude Code plugin (recommended)

```
/plugin marketplace add EscapeVelocityOperations/hatch-cli
/plugin install hatch@hatch
```

Then just ask Claude to deploy — no separate install or auth step
required. The plugin ships an MCP server wrapper that bootstraps the
`hatch` binary automatically if it isn't already on your machine
(installs to `~/.hatch/bin`, no sudo, never auto-updates after that).
The first tool call that needs auth (e.g. `deploy_app`) will prompt you
to run the `login` tool, which opens a browser for sign-in or account
creation — or call `get_started` any time for an unauthenticated status
check. Run `/hatch:onboard` for a guided walkthrough of your first
deploy.

Windows: the wrapper bootstrap script is POSIX-only — install the CLI
manually (see below) and configure the MCP server by hand (see
[AI Integration](#ai-integration)).

### Install script (Linux/macOS)

```sh
curl -fsSL https://gethatch.eu/install | sh
```

### From source

```sh
go install github.com/EscapeVelocityOperations/hatch-cli/cmd/hatch@latest
```

## Quick Start

```sh
# Authenticate
hatch login

# Deploy from any directory with source code
hatch deploy

# View your apps
hatch apps

# Check logs
hatch logs

# Open in browser
hatch open
```

## Commands

### Authentication

#### `hatch login`

Authenticate with Hatch via browser-based OAuth. Opens your default browser to complete the login flow.

```sh
hatch login
```

#### `hatch logout`

Clear stored authentication credentials.

```sh
hatch logout
```

### Deployment

#### `hatch deploy`

Deploy the current directory to Hatch. Initializes git if needed, commits uncommitted changes, and pushes to the platform.

```sh
# Deploy current directory (app name = directory name)
hatch deploy

# Deploy with a custom app name
hatch deploy --name my-app
```

**Flags:**

| Flag | Short | Description |
|------|-------|-------------|
| `--name` | `-n` | Custom app name (defaults to directory name) |

### App Management

#### `hatch apps`

List all applications deployed to your Hatch account.

```sh
hatch apps
```

**Output:**

```
NAME      STATUS   URL
────────  ───────  ─────────────────────────────
myapp     running  https://myapp.gethatch.eu
api       running  https://api.gethatch.eu
```

#### `hatch info <slug>`

Display detailed information about a specific application.

```sh
hatch info myapp
```

**Output:**

```
  My App
  Slug:    myapp
  Status:  running
  URL:     https://myapp.gethatch.eu
  Region:  eu-west
  Created: 2025-06-15 10:00:00
  Updated: 2025-06-15 12:00:00
```

#### `hatch logs [slug]`

Stream application logs. If no slug is provided, the app is auto-detected from the `hatch` git remote in the current directory.

```sh
# Stream logs for a specific app
hatch logs myapp

# Auto-detect app from git remote
hatch logs

# Show last 50 lines
hatch logs --tail 50

# Follow log output in real time
hatch logs -f
```

**Flags:**

| Flag | Short | Description |
|------|-------|-------------|
| `--tail` | | Number of recent lines to show (default: 100) |
| `--follow` | `-f` | Follow log output continuously |

#### `hatch env`

List environment variables for an application.

```sh
# List env vars (auto-detect app)
hatch env

# List env vars for a specific app
hatch env --app myapp
```

#### `hatch env set KEY=VALUE [KEY=VALUE...]`

Set one or more environment variables.

```sh
hatch env set PORT=8080
hatch env set DB_URL=postgres://localhost NODE_ENV=production --app myapp
```

#### `hatch env unset KEY [KEY...]`

Remove environment variables.

```sh
hatch env unset PORT
hatch env unset DB_URL NODE_ENV --app myapp
```

**Flags (all env subcommands):**

| Flag | Short | Description |
|------|-------|-------------|
| `--app` | `-a` | App slug (auto-detected from git remote if omitted) |

#### `hatch restart [slug]`

Restart an application. Prompts for confirmation before proceeding.

```sh
hatch restart myapp
hatch restart          # auto-detect from git remote
```

#### `hatch destroy [slug]`

Permanently delete an application and all its data. Requires typing the app name to confirm.

```sh
hatch destroy myapp
```

```
! This will permanently delete myapp and all its data.

Type "myapp" to confirm: myapp
✓ Destroyed myapp
```

#### `hatch open [slug]`

Open the application URL in your default browser.

```sh
hatch open myapp
hatch open             # auto-detect from git remote
```

### Protection

#### `hatch protect email enable`

Enable email-allowlist protection (replaces the current lists). Visitors sign in via a magic link sent to an allowed email address or domain.

```sh
hatch protect email enable --email a@b.com --domain corp.com
```

**Flags:**

| Flag | Short | Description |
|------|-------|-------------|
| `--email` | | Exact email address(es) to allow (repeatable) |
| `--domain` | | Email domain(s) to allow, with or without a leading `@` (repeatable) |

#### `hatch protect email disable`

Disable email-allowlist protection.

```sh
hatch protect email disable
```

#### `hatch protect email list`

Show the current email allowlist.

```sh
hatch protect email list
```

#### `hatch protect email add <email-or-@domain>...`

Add email(s) or `@domain`(s) to the allowlist.

```sh
hatch protect email add newuser@corp.com @partner.com
```

#### `hatch protect email remove <email-or-@domain>...`

Remove email(s) or `@domain`(s) from the allowlist.

```sh
hatch protect email remove newuser@corp.com
```

If protection is enabled but the deployment has no mailer configured, `list` and `enable` print a warning — no magic link can be sent, so allowed visitors are silently locked out. See the [email-protection docs](https://gethatch.eu/docs/email-protection) for rate limits and the corresponding MCP tools (`set_email_protection`, `get_email_protection`, `disable_email_protection`).

### AI Integration

The [Claude Code plugin](#claude-code-plugin-recommended) is the
easiest way to use Hatch from Claude — it configures the MCP server for
you, including the no-binary bootstrap wrapper. Use the steps below for
other MCP clients, Windows, or a manual setup.

#### `hatch mcp`

Start a Model Context Protocol (MCP) server over stdio. This exposes Hatch CLI tools for use by AI assistants like Claude.

```sh
hatch mcp
```

Add to your AI assistant's MCP configuration:

```json
{
  "mcpServers": {
    "hatch": {
      "command": "hatch",
      "args": ["mcp"]
    }
  }
}
```

Two tools are agent-actionable without any prior setup: `login` (opens
a browser for sign-in or account creation — never requires a
pre-existing token) and `get_started` (auth status, CLI version, and a
recommended next step; safe to call unauthenticated). Every other tool
mirrors a CLI command above (`list_apps`, `deploy_app`, `get_status`,
`get_logs`, and so on) and returns the same agent-actionable error
naming `login` if you're not authenticated yet.

### Utility

#### `hatch version`

Print the CLI version, commit, and build date.

```sh
hatch version
```

## App Slug Auto-Detection

Commands that accept an optional `[slug]` argument can auto-detect the app from the `hatch` git remote in the current directory. This works when you've previously deployed with `hatch deploy`, which sets up the remote automatically.

Supported commands: `logs`, `restart`, `destroy`, `open`, `env`

## Global Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--config` | | Config file path (default: `~/.hatch/config.json`) |
| `--verbose` | `-v` | Enable verbose output |

## Configuration

Hatch stores configuration in `~/.hatch/config.json`. This file contains your authentication token and is created automatically on `hatch login`.

## Release signing

Darwin release binaries (`hatch-darwin-amd64`, `hatch-darwin-arm64`) are
codesigned with a Developer ID Application certificate and notarized via
`notarytool` when Apple credentials are configured in CI. This gives the
binary a stable code identity that app firewalls (e.g. Little Snitch) can
attribute, and clears Gatekeeper on manual browser downloads.

We notarize a zip of the bare binary, not a stapled `.app`/`.dmg`/`.pkg` —
Apple's `stapler` only staples bundles and installer packages, and switching
distribution formats would break the `install.sh` / MCP bootstrap checksum
contract. Gatekeeper instead resolves the notarization ticket online on
first launch, so a freshly downloaded binary needs network access the first
time it runs.

If the signing secrets aren't configured, the release step falls back to
today's behavior — an unsigned binary, with a `::warning::` noted in the
workflow log. See the secrets contract in
[`docs/plans/h-6ewk-darwin-sign-notarize-release.md`](docs/plans/h-6ewk-darwin-sign-notarize-release.md).

## Stack

- Go 1.25+
- [Cobra](https://github.com/spf13/cobra) - CLI framework
- [Viper](https://github.com/spf13/viper) - Configuration management

## Development

```sh
# Build
make build

# Run tests
make test

# Lint
make lint
```
