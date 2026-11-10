#!/bin/sh
set -e

# Redirect this script's own stdout to stderr for the rest of its run: when
# invoked nested inside the hatch-mcp.sh bootstrap wrapper (D4), stdout is
# inherited straight through to the live MCP JSON-RPC channel — any
# installer banner/progress/prompt text landing there would corrupt the
# protocol handshake for every first-time bootstrap-via-plugin user. This
# covers every plain `echo`/`printf` in the script in one place, rather than
# relying on each one to remember `>&2` individually. It does NOT break
# ask()'s `$(ask ...)` return-value capture below: command substitution
# always captures its own subshell's stdout, independent of this outer
# redirect.
exec >&2

REPO="EscapeVelocityOperations/hatch-cli"
BINARY_NAME="hatch"

# Colors (disabled if not a terminal). Checked against fd 2 (stderr), not
# fd 1: that's the fd info/warn/ok/dim below actually write to.
if [ -t 2 ]; then
    BOLD='\033[1m'
    DIM='\033[2m'
    GREEN='\033[32m'
    YELLOW='\033[33m'
    CYAN='\033[36m'
    RESET='\033[0m'
else
    BOLD='' DIM='' GREEN='' YELLOW='' CYAN='' RESET=''
fi

# All diagnostic/progress output goes to stderr, never stdout: when this
# script runs nested inside the hatch-mcp.sh bootstrap wrapper (D4), stdout
# is inherited straight through to the live MCP JSON-RPC channel — any
# installer chatter landing there would corrupt the protocol handshake for
# every first-time bootstrap-via-plugin user.
info()  { printf "${CYAN}==>${RESET} ${BOLD}%s${RESET}\n" "$1" >&2; }
warn()  { printf "${YELLOW}  ⚠${RESET}  %s\n" "$1" >&2; }
ok()    { printf "${GREEN}  ✓${RESET}  %s\n" "$1" >&2; }
dim()   { printf "${DIM}     %s${RESET}\n" "$1" >&2; }

# Non-interactive detection (curl | sh, or the hatch-mcp.sh bootstrap wrapper,
# has no TTY on stdin — and in the wrapper's case stdin is the live MCP stdio
# stream, not simply closed: an unconditional `read` here would block on it
# forever, or steal a line of live protocol traffic).
INTERACTIVE=false
if [ -t 0 ]; then
    INTERACTIVE=true
fi

# Prompt with default value. Usage: ask "question" "default"
# In non-interactive mode, returns default silently without touching stdin.
ask() {
    if [ "$INTERACTIVE" = false ]; then
        echo "$2"
        return
    fi
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

# Fail-closed sha256 verification (D5): aborts if there is no checksums.txt
# entry for filename, or if it doesn't match bin_path's actual digest.
# Handles both GNU coreutils (sha256sum, Linux) and macOS (shasum -a 256).
verify_checksum() {
    bin_path="$1"
    filename="$2"
    checksums_file="$3"

    expected=$(awk -v f="$filename" '$2 == f { print $1 }' "$checksums_file")
    if [ -z "$expected" ]; then
        echo "Error: no checksum entry for ${filename} in checksums.txt — aborting" >&2
        exit 1
    fi

    if command -v sha256sum >/dev/null 2>&1; then
        actual=$(sha256sum "$bin_path" | awk '{print $1}')
    elif command -v shasum >/dev/null 2>&1; then
        actual=$(shasum -a 256 "$bin_path" | awk '{print $1}')
    else
        echo "Error: sha256sum or shasum required for checksum verification — aborting" >&2
        exit 1
    fi

    if [ "$actual" != "$expected" ]; then
        echo "Error: checksum mismatch for ${filename} (expected ${expected}, got ${actual}) — aborting" >&2
        exit 1
    fi

    ok "Checksum verified"
}

download_binary() {
    FILENAME="${BINARY_NAME}-${OS}-${ARCH}"
    URL="https://github.com/${REPO}/releases/latest/download/${FILENAME}"
    CHECKSUMS_URL="https://github.com/${REPO}/releases/latest/download/checksums.txt"

    info "Downloading ${BINARY_NAME} for ${OS}/${ARCH}..."

    TMPDIR=$(mktemp -d)
    trap 'rm -rf "$TMPDIR"' EXIT

    if command -v curl >/dev/null 2>&1; then
        curl -fsSL -o "${TMPDIR}/${BINARY_NAME}" "$URL"
        curl -fsSL -o "${TMPDIR}/checksums.txt" "$CHECKSUMS_URL"
    elif command -v wget >/dev/null 2>&1; then
        wget -qO "${TMPDIR}/${BINARY_NAME}" "$URL"
        wget -qO "${TMPDIR}/checksums.txt" "$CHECKSUMS_URL"
    else
        echo "Error: curl or wget required" >&2
        exit 1
    fi

    verify_checksum "${TMPDIR}/${BINARY_NAME}" "$FILENAME" "${TMPDIR}/checksums.txt"

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
