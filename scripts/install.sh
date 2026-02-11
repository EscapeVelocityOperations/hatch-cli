#!/bin/sh
set -e

REPO="EscapeVelocityOperations/hatch-cli"
BINARY_NAME="hatch"

# Colors (disabled if not a terminal)
if [ -t 1 ]; then
    BOLD='\033[1m'
    DIM='\033[2m'
    GREEN='\033[32m'
    YELLOW='\033[33m'
    CYAN='\033[36m'
    RESET='\033[0m'
else
    BOLD='' DIM='' GREEN='' YELLOW='' CYAN='' RESET=''
fi

info()  { printf "${CYAN}==>${RESET} ${BOLD}%s${RESET}\n" "$1"; }
warn()  { printf "${YELLOW}  ⚠${RESET}  %s\n" "$1"; }
ok()    { printf "${GREEN}  ✓${RESET}  %s\n" "$1"; }
dim()   { printf "${DIM}     %s${RESET}\n" "$1"; }

# Prompt with default value. Usage: ask "question" "default"
ask() {
    printf "${BOLD}%s${RESET} " "$1" >&2
    read -r answer
    echo "${answer:-$2}"
}

detect_platform() {
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
}

choose_install_dir() {
    # If HATCH_INSTALL_DIR is set, use it (non-interactive / CI)
    if [ -n "$HATCH_INSTALL_DIR" ]; then
        INSTALL_DIR="$HATCH_INSTALL_DIR"
        return
    fi

    USER_DIR="$HOME/.local/bin"
    SYSTEM_DIR="/usr/local/bin"

    echo ""
    info "Where would you like to install hatch?"
    echo ""
    echo "  1) ${USER_DIR}  ${DIM}(user-only, no sudo)${RESET}"
    echo "  2) ${SYSTEM_DIR}  ${DIM}(system-wide, requires sudo)${RESET}"
    echo ""
    choice=$(ask "Choice [1]:" "1")

    case "$choice" in
        2) INSTALL_DIR="$SYSTEM_DIR" ;;
        *) INSTALL_DIR="$USER_DIR" ;;
    esac
}

download_binary() {
    FILENAME="${BINARY_NAME}-${OS}-${ARCH}"
    URL="https://github.com/${REPO}/releases/latest/download/${FILENAME}"

    info "Downloading ${BINARY_NAME} for ${OS}/${ARCH}..."

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
}

install_binary() {
    info "Installing to ${INSTALL_DIR}/${BINARY_NAME}"

    # Create install dir if it doesn't exist
    if [ ! -d "$INSTALL_DIR" ]; then
        mkdir -p "$INSTALL_DIR" 2>/dev/null || sudo mkdir -p "$INSTALL_DIR"
    fi

    if [ -w "$INSTALL_DIR" ]; then
        install -m 755 "${TMPDIR}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
    else
        dim "Requires sudo..."
        sudo install -m 755 "${TMPDIR}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
    fi

    ok "Installed ${BINARY_NAME} to ${INSTALL_DIR}/${BINARY_NAME}"
}

check_path() {
    case ":${PATH}:" in
        *":${INSTALL_DIR}:"*) return ;;
    esac

    echo ""
    warn "${INSTALL_DIR} is not in your PATH"

    # Detect shell config file
    SHELL_NAME=$(basename "${SHELL:-/bin/sh}")
    case "$SHELL_NAME" in
        zsh)  SHELL_RC="$HOME/.zshrc" ;;
        bash) SHELL_RC="$HOME/.bashrc" ;;
        *)    SHELL_RC="$HOME/.profile" ;;
    esac

    EXPORT_LINE="export PATH=\"${INSTALL_DIR}:\$PATH\""

    echo ""
    echo "  Add this to ${SHELL_RC}:"
    echo ""
    echo "    ${EXPORT_LINE}"
    echo ""

    add_path=$(ask "Add it now? [Y/n]:" "y")
    case "$add_path" in
        n|N|no|No) dim "Skipped. Remember to add it manually." ;;
        *)
            echo "" >> "$SHELL_RC"
            echo "# Hatch CLI" >> "$SHELL_RC"
            echo "$EXPORT_LINE" >> "$SHELL_RC"
            ok "Added to ${SHELL_RC}"
            dim "Run: source ${SHELL_RC}"
            # Also export for current session so completions work
            export PATH="${INSTALL_DIR}:${PATH}"
            ;;
    esac
}

setup_zsh_completions() {
    # Only offer for zsh users
    SHELL_NAME=$(basename "${SHELL:-/bin/sh}")
    if [ "$SHELL_NAME" != "zsh" ]; then
        return
    fi

    # Check if hatch has completion support
    if ! "${INSTALL_DIR}/${BINARY_NAME}" completion zsh >/dev/null 2>&1; then
        return
    fi

    COMP_DIR="$HOME/.zsh/completions"
    COMP_FILE="${COMP_DIR}/_hatch"
    ZSHRC="$HOME/.zshrc"

    echo ""
    info "Zsh detected — set up shell completions?"
    dim "Enables tab-completion for all hatch commands and flags"
    echo ""
    setup_comp=$(ask "Install zsh completions? [Y/n]:" "y")

    case "$setup_comp" in
        n|N|no|No)
            dim "Skipped."
            return
            ;;
    esac

    # Generate completions
    mkdir -p "$COMP_DIR"
    "${INSTALL_DIR}/${BINARY_NAME}" completion zsh > "$COMP_FILE"
    ok "Completions written to ${COMP_FILE}"

    # Check if fpath already includes our dir
    FPATH_LINE="fpath=(${COMP_DIR} \$fpath)"
    COMPINIT_LINE="autoload -Uz compinit && compinit"

    needs_fpath=true
    needs_compinit=true

    if [ -f "$ZSHRC" ]; then
        if grep -qF "$COMP_DIR" "$ZSHRC" 2>/dev/null; then
            needs_fpath=false
        fi
        if grep -q "compinit" "$ZSHRC" 2>/dev/null; then
            needs_compinit=false
        fi
    fi

    if [ "$needs_fpath" = true ] || [ "$needs_compinit" = true ]; then
        echo ""
        add_zshrc=$(ask "Add completion config to ${ZSHRC}? [Y/n]:" "y")
        case "$add_zshrc" in
            n|N|no|No)
                echo ""
                dim "Add these lines to ${ZSHRC} manually:"
                if [ "$needs_fpath" = true ]; then
                    dim "  ${FPATH_LINE}"
                fi
                if [ "$needs_compinit" = true ]; then
                    dim "  ${COMPINIT_LINE}"
                fi
                return
                ;;
        esac

        echo "" >> "$ZSHRC"
        echo "# Hatch CLI completions" >> "$ZSHRC"
        if [ "$needs_fpath" = true ]; then
            echo "$FPATH_LINE" >> "$ZSHRC"
            ok "Added fpath to ${ZSHRC}"
        fi
        if [ "$needs_compinit" = true ]; then
            echo "$COMPINIT_LINE" >> "$ZSHRC"
            ok "Added compinit to ${ZSHRC}"
        fi
        dim "Run: source ${ZSHRC}"
    else
        ok "Zsh already configured for completions"
    fi
}

main() {
    echo ""
    echo "  ${BOLD}Hatch CLI Installer${RESET}"
    echo ""

    detect_platform
    choose_install_dir
    download_binary
    install_binary
    check_path
    setup_zsh_completions

    echo ""
    ok "$("${INSTALL_DIR}/${BINARY_NAME}" version 2>&1 || echo "${BINARY_NAME} installed")"
    echo ""
    info "Get started: hatch login"
    echo ""
}

main
