#!/bin/sh
set -e

DOWNLOAD_BASE="https://gethatch.eu/downloads"
INSTALL_DIR="/usr/local/bin"
BINARY_NAME="hatch"

main() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)

    case "$OS" in
        linux)  OS="linux" ;;
        darwin) OS="darwin" ;;
        *)      echo "Error: unsupported OS: $OS" >&2; exit 1 ;;
    esac

    case "$ARCH" in
        x86_64|amd64)  ARCH="amd64" ;;
        aarch64|arm64) ARCH="arm64" ;;
        *)             echo "Error: unsupported architecture: $ARCH" >&2; exit 1 ;;
    esac

    FILENAME="${BINARY_NAME}-${OS}-${ARCH}"
    URL="${DOWNLOAD_BASE}/${FILENAME}"

    echo "Downloading ${BINARY_NAME} for ${OS}/${ARCH}..."

    TMPDIR=$(mktemp -d)
    trap 'rm -rf "$TMPDIR"' EXIT

    if command -v curl >/dev/null 2>&1; then
        curl -fsSL -o "${TMPDIR}/${BINARY_NAME}" "$URL"
    elif command -v wget >/dev/null 2>&1; then
        wget -qO "${TMPDIR}/${BINARY_NAME}" "$URL"
    else
        echo "Error: curl or wget required" >&2
        exit 1
    fi

    chmod +x "${TMPDIR}/${BINARY_NAME}"

    if [ -w "$INSTALL_DIR" ]; then
        mv "${TMPDIR}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
    else
        echo "Installing to ${INSTALL_DIR} (requires sudo)..."
        sudo mv "${TMPDIR}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
    fi

    echo "Installed ${BINARY_NAME} to ${INSTALL_DIR}/${BINARY_NAME}"
    "${INSTALL_DIR}/${BINARY_NAME}" version
}

main
