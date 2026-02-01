# hatch-cli

Command-line interface for deploying and managing applications on the Hatch platform. Authenticate via OAuth, deploy with `hatch deploy`, and manage apps from your terminal.

## Architecture

```
┌──────────────────────────────┐
│         hatch CLI            │
│         (Go / Cobra)         │
├──────────────────────────────┤
│  login    → OAuth browser    │
│  deploy   → git push deploy  │
│  apps     → list/info        │
│  logs     → SSE streaming    │
│  env      → key/value mgmt   │
│  open     → browser launch   │
└──────────────┬───────────────┘
               │ HTTPS (Bearer token)
┌──────────────▼───────────────┐
│       Hatch API              │
│       (hatch-api)            │
└──────────────────────────────┘
```

**Authentication** — OAuth browser flow opens the Hatch login page, a local callback server on `localhost:8765` captures the token, and stores it in `~/.hatch/config.json`.

**Deploy** — Initializes a git repo if needed, auto-commits changes, configures a `hatch` remote pointing at the Hatch git server, and force-pushes to trigger a build.

## Tech Stack

- Go 1.22
- Cobra (CLI framework)
- Viper (configuration)
- goreleaser (distribution)

## Project Structure

```
cmd/
  hatch/            Main entrypoint
  root/             Root command, version
  deploy/           Deploy command (git push workflow)
  login/            OAuth browser login + callback server
  logout/           Clear local credentials
internal/
  auth/             OAuth flow: browser, callback, token, state
  config/           Config file management (~/.hatch/config.json)
  git/              Git operations: init, commit, remote, push
  ui/               Terminal UI: colors, spinner, table
scripts/            Build and release scripts
```

## Setup

### Prerequisites

- Go 1.22+
- Git

### Configuration

The CLI stores its config at `~/.hatch/config.json`:

```json
{
  "token": "your-oauth-token",
  "api_host": "https://api.gethatch.eu"
}
```

Configuration is managed automatically by `hatch login` and `hatch logout`.

### Build

```bash
make build      # Build binary to bin/hatch
make install    # Install to $GOPATH/bin
make test       # Run all tests
make lint       # Run golangci-lint
make clean      # Remove build artifacts
```

## Commands

### `hatch login`

Opens the Hatch OAuth page in your browser. A local callback server on `localhost:8765` captures the token and saves it to `~/.hatch/config.json`.

### `hatch logout`

Clears the stored credentials.

### `hatch deploy`

Deploys the current directory to Hatch:
1. Checks for authentication token
2. Initializes git repo if needed
3. Auto-commits any uncommitted changes
4. Configures/updates the `hatch` remote
5. Force-pushes to trigger a build

Flags:
- `--name/-n` — Custom app name (default: directory name)

### `hatch version`

Displays version, commit, and build date.

## Testing

```bash
make test
# or
go test ./...
```

Tests cover deploy command validation, login/logout flows, auth token management, browser/callback handling, config file operations, and git operations (init, remote, push).

## Distribution

Release builds are managed by goreleaser:

```bash
make release           # Full release
make release-snapshot  # Snapshot (no publish)
```
