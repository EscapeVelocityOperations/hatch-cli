# hatch-cli

Command-line interface for deploying and managing applications on the [Hatch](https://gethatch.eu) platform.

## Installation

### Homebrew (macOS)

```sh
brew install EscapeVelocityOperations/tap/hatch
```

### Install script (Linux/macOS)

```sh
curl -fsSL https://gethatch.eu/install.sh | sh
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

## Stack

- Go 1.22+
- [Cobra](https://github.com/spf13/cobra) - CLI framework
- [Viper](https://github.com/spf13/viper) - Configuration management
- [goreleaser](https://goreleaser.com) - Cross-platform builds and releases

## Development

```sh
# Build
make build

# Run tests
make test

# Lint
make lint

# Build release snapshot
make release-snapshot
```
