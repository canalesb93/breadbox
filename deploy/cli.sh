#!/usr/bin/env bash
# Breadbox CLI Installer
#
# Downloads the `breadbox` CLI (lite build, ~10 MB) for connecting to a
# remote Breadbox server. No Docker, no Postgres, no server install.
#
# Usage:
#   curl -fsSL https://breadbox.sh/cli.sh | bash
#   curl -fsSL https://breadbox.sh/cli.sh | bash -s -- --host=https://breadbox.example.com
#   curl -fsSL https://breadbox.sh/cli.sh | bash -s -- --yes
#
# Install path:
#   - root → /usr/local/bin/breadbox
#   - regular user → $HOME/.local/bin/breadbox
#   Override: INSTALL_BIN=/custom/path bash cli.sh
#
# For installing the full SERVER (Docker + Postgres + dashboard), use
# https://breadbox.sh/install.sh instead.

set -eu

REPO="canalesb93/breadbox"
GITHUB_API="https://api.github.com/repos/${REPO}"

# ---------------------------------------------------------------------------
# Color helpers
# ---------------------------------------------------------------------------
if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
    GREEN=$(printf '\033[0;32m')
    YELLOW=$(printf '\033[1;33m')
    BLUE=$(printf '\033[0;34m')
    RED=$(printf '\033[0;31m')
    BOLD=$(printf '\033[1m')
    DIM=$(printf '\033[2m')
    NC=$(printf '\033[0m')
else
    GREEN='' YELLOW='' BLUE='' RED='' BOLD='' DIM='' NC=''
fi

info()    { printf "${BLUE}::${NC} %s\n" "$*"; }
warn()    { printf "${YELLOW}warning:${NC} %s\n" "$*"; }
success() { printf "${GREEN}ok:${NC} %s\n" "$*"; }
error()   { printf "${RED}error:${NC} %s\n" "$*" >&2; }
die()     { error "$*"; exit 1; }

# ---------------------------------------------------------------------------
# Platform detection (self-contained — no detect.sh dependency)
# ---------------------------------------------------------------------------
detect_os() {
    case "$(uname -s 2>/dev/null)" in
        Linux)  printf "linux"  ;;
        Darwin) printf "darwin" ;;
        *) die "Unsupported OS '$(uname -s)'. Breadbox CLI ships for Linux and macOS." ;;
    esac
}

detect_arch() {
    case "$(uname -m 2>/dev/null)" in
        x86_64|amd64)  printf "amd64" ;;
        aarch64|arm64) printf "arm64" ;;
        *) die "Unsupported arch '$(uname -m)'. Breadbox CLI ships for amd64 and arm64." ;;
    esac
}

OS=$(detect_os)
ARCH=$(detect_arch)

# ---------------------------------------------------------------------------
# Args
# ---------------------------------------------------------------------------
AUTO_YES=0
HOST_ARG=""
VERSION_ARG=""

for arg in "$@"; do
    case "$arg" in
        --yes|-y) AUTO_YES=1 ;;
        --host=*) HOST_ARG="${arg#--host=}" ;;
        --version=*) VERSION_ARG="${arg#--version=}" ;;
        --help|-h)
            printf "Usage: cli.sh [OPTIONS]\n\n"
            printf "Options:\n"
            printf "  --host=URL       Run 'breadbox auth login --host=URL' after install\n"
            printf "  --version=vX.Y.Z Pin to a specific release tag (default: latest)\n"
            printf "  --yes, -y        Non-interactive: install without prompts, skip auth\n"
            printf "  --help, -h       Show this message\n"
            printf "\nEnvironment:\n"
            printf "  INSTALL_BIN      Override the install path (default per-user / root)\n"
            printf "  NO_COLOR         Disable colored output\n"
            exit 0
            ;;
    esac
done

# ---------------------------------------------------------------------------
# TTY-aware read for `curl | bash` prompts
# ---------------------------------------------------------------------------
_bb_read_line() {
    if [ -t 0 ]; then
        read -r ans
        return $?
    fi
    if read -r ans </dev/tty 2>/dev/null; then
        return 0
    fi
    read -r ans 2>/dev/null || return 1
}

prompt_value() {
    question="$1"
    default="${2:-}"
    if [ "${AUTO_YES:-0}" = "1" ]; then
        printf "%s" "$default"
        return
    fi
    if [ -n "$default" ]; then
        printf "%s [%s]: " "$question" "$default" >&2
    else
        printf "%s: " "$question" >&2
    fi
    if ! _bb_read_line; then
        ans=""
    fi
    printf "%s" "${ans:-$default}"
}

# ---------------------------------------------------------------------------
# Banner
# ---------------------------------------------------------------------------
printf "\n"
printf "${BOLD}  Breadbox CLI installer${NC}\n"
printf "${DIM}  https://github.com/${REPO}${NC}\n"
printf "\n"

# ---------------------------------------------------------------------------
# Resolve install path
# ---------------------------------------------------------------------------
if [ -z "${INSTALL_BIN:-}" ]; then
    if [ "$(id -u 2>/dev/null || echo 1000)" = "0" ]; then
        INSTALL_BIN="/usr/local/bin/breadbox"
    else
        INSTALL_BIN="${HOME:-.}/.local/bin/breadbox"
    fi
fi

info "Platform:     ${OS}/${ARCH}"
info "Install path: ${INSTALL_BIN}"
printf "\n"

# ---------------------------------------------------------------------------
# Resolve target version
# ---------------------------------------------------------------------------
if [ -n "$VERSION_ARG" ]; then
    TAG="$VERSION_ARG"
    info "Pinned version: ${TAG}"
else
    info "Fetching latest release tag..."
    if command -v curl >/dev/null 2>&1; then
        TAG=$(curl -fsSL "${GITHUB_API}/releases/latest" 2>/dev/null \
            | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"//;s/".*//')
    elif command -v wget >/dev/null 2>&1; then
        TAG=$(wget -qO- "${GITHUB_API}/releases/latest" 2>/dev/null \
            | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"//;s/".*//')
    else
        die "Neither curl nor wget found. Install one and retry."
    fi
    [ -z "$TAG" ] && die "Could not determine the latest release tag. (Has the first release been cut yet? You can pin a tag explicitly with --version=vX.Y.Z.)"
    success "Latest release: ${TAG}"
fi

# ---------------------------------------------------------------------------
# Download
# ---------------------------------------------------------------------------
ASSET="breadbox-cli-${OS}-${ARCH}"
URL="https://github.com/${REPO}/releases/download/${TAG}/${ASSET}"

mkdir -p "$(dirname "$INSTALL_BIN")"

if [ -f "$INSTALL_BIN" ]; then
    info "Replacing existing binary at ${INSTALL_BIN}"
fi

info "Downloading ${ASSET}..."
if command -v curl >/dev/null 2>&1; then
    if ! curl -fsSL "$URL" -o "${INSTALL_BIN}.tmp"; then
        rm -f "${INSTALL_BIN}.tmp"
        die "Failed to download ${URL}. Check that the release contains ${ASSET}."
    fi
elif command -v wget >/dev/null 2>&1; then
    if ! wget -qO "${INSTALL_BIN}.tmp" "$URL"; then
        rm -f "${INSTALL_BIN}.tmp"
        die "Failed to download ${URL}."
    fi
fi

mv "${INSTALL_BIN}.tmp" "$INSTALL_BIN"
chmod +x "$INSTALL_BIN"
success "Installed: ${INSTALL_BIN}"

# ---------------------------------------------------------------------------
# PATH check
# ---------------------------------------------------------------------------
bindir=$(dirname "$INSTALL_BIN")
if ! printf '%s' ":${PATH}:" | grep -q ":${bindir}:"; then
    printf "\n"
    warn "${bindir} is not on your PATH. Add it to your shell profile:"
    case "${SHELL:-}" in
        */zsh)
            printf "    ${DIM}echo 'export PATH=\"%s:\$PATH\"' >> ~/.zshrc${NC}\n" "$bindir"
            ;;
        */fish)
            printf "    ${DIM}fish_add_path %s${NC}\n" "$bindir"
            ;;
        *)
            printf "    ${DIM}echo 'export PATH=\"%s:\$PATH\"' >> ~/.bashrc${NC}\n" "$bindir"
            ;;
    esac
fi

# ---------------------------------------------------------------------------
# Optional: auth login
# ---------------------------------------------------------------------------
HOST_VALUE="$HOST_ARG"
if [ -z "$HOST_VALUE" ] && [ "${AUTO_YES:-0}" != "1" ]; then
    printf "\n"
    info "Optional: connect to a remote Breadbox now."
    info "Leave blank to skip — you can run \`breadbox auth login --host=...\` later."
    HOST_VALUE=$(prompt_value "Host URL (e.g. https://breadbox.example.com)" "")
fi

if [ -n "$HOST_VALUE" ]; then
    printf "\n"
    info "Running: breadbox auth login --host=${HOST_VALUE}"
    "$INSTALL_BIN" auth login --host="$HOST_VALUE" || warn "auth login did not complete — re-run manually when ready."
fi

# ---------------------------------------------------------------------------
# Done
# ---------------------------------------------------------------------------
printf "\n"
printf "${GREEN}${BOLD}  Breadbox CLI installed.${NC}\n"
printf "\n"
info "Try:    ${BOLD}breadbox doctor${NC}"
info "Catalog: ${BOLD}breadbox --help${NC}"
info "Docs:   https://docs.breadbox.sh"
printf "\n"
