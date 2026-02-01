#!/bin/sh
set -e

REPO="EscapeVelocityOperations/hatch-cli"
BINARY="hatch"
INSTALL_DIR="/usr/local/bin"

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
    x86_64|amd64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

case "$OS" in
    linux|darwin) ;;
    *) echo "Unsupported OS: $OS"; exit 1 ;;
esac

LATEST=$(curl -sL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"v?([^"]+)".*/\1/')

if [ -z "$LATEST" ]; then
    echo "Error: Could not determine latest version"
    exit 1
fi

URL="https://github.com/${REPO}/releases/download/v${LATEST}/${BINARY}_${OS}_${ARCH}.tar.gz"

TMPDIR=$(mktemp -d)
trap "rm -rf $TMPDIR" EXIT

echo "Downloading hatch v${LATEST} for ${OS}/${ARCH}..."
curl -sL "$URL" | tar xz -C "$TMPDIR"

echo "Installing to ${INSTALL_DIR}/${BINARY}..."
sudo install -m 755 "${TMPDIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"

echo "hatch v${LATEST} installed successfully"
