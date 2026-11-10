#!/bin/sh
set -eu

# Bootstrap wrapper for the Hatch Claude Code plugin's MCP server (D4,
# docs/plans/h-y2g6-adr0023-zero-friction-onboarding.md in the portharbour
# repo). Resolves an existing `hatch` binary, or bootstraps one from the
# pinned install URL, then execs `hatch mcp`.
#
# stdout is reserved for the MCP stdio protocol once `hatch mcp` execs —
# every diagnostic line in this script goes to stderr.

INSTALL_URL="https://gethatch.eu/install"
INSTALL_DIR="$HOME/.hatch/bin"
# Overridable only for tests (an isolated fixture dir); production always
# resolves to the real /usr/local/bin.
SYSTEM_BIN_DIR="${HATCH_SYSTEM_BIN_DIR:-/usr/local/bin}"

hatch_bin=""

if command -v hatch >/dev/null 2>&1; then
    hatch_bin=$(command -v hatch)
elif [ -x "$INSTALL_DIR/hatch" ]; then
    hatch_bin="$INSTALL_DIR/hatch"
elif [ -x "$HOME/.local/bin/hatch" ]; then
    hatch_bin="$HOME/.local/bin/hatch"
elif [ -x "$SYSTEM_BIN_DIR/hatch" ]; then
    hatch_bin="$SYSTEM_BIN_DIR/hatch"
fi

if [ -z "$hatch_bin" ]; then
    echo "hatch-mcp.sh: hatch binary not found, bootstrapping from $INSTALL_URL" >&2

    tmp_installer=$(mktemp)
    trap 'rm -f "$tmp_installer"' EXIT

    # Downloaded to a file and run as a separate step (rather than a naked
    # `curl | sh` pipe) so a curl failure is caught directly: POSIX sh has
    # no `pipefail`, so a failed curl piped into `sh` would otherwise run an
    # empty script and report success.
    if ! curl -fsSL "$INSTALL_URL" -o "$tmp_installer"; then
        echo "hatch-mcp.sh: failed to download installer from $INSTALL_URL — aborting (no fallback mirrors)" >&2
        exit 1
    fi

    if ! HATCH_INSTALL_DIR="$INSTALL_DIR" sh "$tmp_installer"; then
        echo "hatch-mcp.sh: installer failed — aborting" >&2
        exit 1
    fi

    hatch_bin="$INSTALL_DIR/hatch"
    if [ ! -x "$hatch_bin" ]; then
        echo "hatch-mcp.sh: bootstrap completed but $hatch_bin is not executable — aborting" >&2
        exit 1
    fi

    echo "hatch-mcp.sh: installed hatch to $hatch_bin" >&2
    "$hatch_bin" version >&2 || true
fi

exec "$hatch_bin" mcp
