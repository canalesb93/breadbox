#!/usr/bin/env bash
set -eu

# Breadbox Install Script
# Usage:
#   curl -sSL https://raw.githubusercontent.com/canalesb93/breadbox/main/deploy/install.sh | bash
#   bash install.sh [--uninstall] [--yes] [--domain=...] [--install-docker]
#   bash install.sh [--version=vX.Y.Z] [--no-start]
#
# INSTALL_DIR convention:
#   - System install  (running as root or EUID 0) → /opt/breadbox
#   - User install    (default)                   → $HOME/.breadbox
# Override with: INSTALL_DIR=/custom/path bash install.sh
#
# Caddy (HTTPS reverse proxy) is gated behind the `caddy` compose profile.
# It is only started when a DOMAIN is configured, so localhost-only installs
# never bind ports 80/443.

REPO="canalesb93/breadbox"
GITHUB_RAW="https://raw.githubusercontent.com/${REPO}"
GITHUB_API="https://api.github.com/repos/${REPO}"
COMPOSE_FILE="docker-compose.prod.yml"

# Load platform detection. When install.sh is piped into `bash`, the local
# detect.sh file does not exist — fetch it into a temp location and source
# from there so the one-liner path works too.
_bb_load_detect() {
    # Try the local copy first (git checkout / manual install).
    script_dir=""
    if [ -n "${BASH_SOURCE:-}" ]; then
        script_dir=$(cd "$(dirname "${BASH_SOURCE:-$0}")" 2>/dev/null && pwd || echo "")
    fi
    if [ -n "$script_dir" ] && [ -r "$script_dir/detect.sh" ]; then
        # shellcheck disable=SC1090,SC1091
        . "$script_dir/detect.sh"
        return
    fi
    # Fetch from GitHub. Use a temp file; silently skip if we can't.
    _detect_tmp="${TMPDIR:-/tmp}/breadbox-detect.$$.sh"
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL "${GITHUB_RAW}/main/deploy/detect.sh" -o "$_detect_tmp" 2>/dev/null || return 0
    elif command -v wget >/dev/null 2>&1; then
        wget -qO "$_detect_tmp" "${GITHUB_RAW}/main/deploy/detect.sh" 2>/dev/null || return 0
    else
        return 0
    fi
    if [ -r "$_detect_tmp" ]; then
        # shellcheck disable=SC1090
        . "$_detect_tmp"
        rm -f "$_detect_tmp"
    fi
}
_bb_load_detect
# Populate BB_* if detect.sh is available; otherwise tolerate absence.
if command -v bb_detect_all >/dev/null 2>&1; then
    bb_detect_all
fi
: "${BB_OS:=unknown}"
: "${BB_ARCH:=unknown}"
: "${BB_DISTRO:=unknown}"
: "${BB_DISTRO_VERSION:=unknown}"
: "${BB_PKG_MANAGER:=none}"
: "${BB_INIT_SYSTEM:=none}"

# Pick a consistent default INSTALL_DIR based on privilege.
if [ -z "${INSTALL_DIR:-}" ]; then
    if [ "$(id -u 2>/dev/null || echo 1000)" = "0" ]; then
        INSTALL_DIR="/opt/breadbox"
    else
        INSTALL_DIR="${HOME:-.}/.breadbox"
    fi
fi

# ---------------------------------------------------------------------------
# Color helpers (disabled when not a terminal or NO_COLOR is set)
# ---------------------------------------------------------------------------
if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
    # Pre-interpret the escapes via $(printf ...) so each variable holds
    # an actual ESC byte rather than the literal text "\033". Avoids a
    # subtle bug where `printf "...\\_\\${NC}\n"` would consume the
    # trailing `\\` as a backslash literal and leak `\033[0m` as visible
    # text — exactly what happened on the bottom-right of the ASCII
    # banner.
    RED=$(printf '\033[0;31m')
    GREEN=$(printf '\033[0;32m')
    YELLOW=$(printf '\033[1;33m')
    BLUE=$(printf '\033[0;34m')
    BOLD=$(printf '\033[1m')
    DIM=$(printf '\033[2m')
    NC=$(printf '\033[0m')
else
    RED='' GREEN='' YELLOW='' BLUE='' BOLD='' DIM='' NC=''
fi

info()    { printf "${BLUE}::${NC} %s\n" "$*"; }
warn()    { printf "${YELLOW}warning:${NC} %s\n" "$*"; }
error()   { printf "${RED}error:${NC} %s\n" "$*" >&2; }
success() { printf "${GREEN}ok:${NC} %s\n" "$*"; }

die() { error "$*"; exit 1; }

# ---------------------------------------------------------------------------
# Banner
# ---------------------------------------------------------------------------
banner() {
    printf "\n"
    printf "${BOLD}  ____                      _ _               ${NC}\n"
    printf "${BOLD} | __ ) _ __ ___  __ _  __| | |__   _____  __${NC}\n"
    printf "${BOLD} |  _ \\| '__/ _ \\/ _\` |/ _\` | '_ \\ / _ \\ \\/ /${NC}\n"
    printf "${BOLD} | |_) | | |  __/ (_| | (_| | |_) | (_) >  < ${NC}\n"
    printf "${BOLD} |____/|_|  \\___|\\__,_|\\__,_|_.__/ \\___/_/\\_\\${NC}\n"
    printf "\n"
    printf "${DIM}  Self-hosted financial data aggregation${NC}\n"
    printf "${DIM}  https://github.com/${REPO}${NC}\n"
    printf "\n"
}

# ---------------------------------------------------------------------------
# Uninstall
# ---------------------------------------------------------------------------
do_uninstall() {
    banner
    info "Uninstalling Breadbox from ${INSTALL_DIR}"
    printf "\n"

    if [ ! -d "$INSTALL_DIR" ]; then
        die "No installation found at ${INSTALL_DIR}"
    fi

    cd "$INSTALL_DIR"

    # Stop containers if compose file exists
    if [ -f "$COMPOSE_FILE" ]; then
        info "Stopping containers..."
        # Pass --profile caddy so the Caddy service (if it was started) is
        # included in the stop. Profiles not in use are silently ignored.
        docker compose --profile caddy -f "$COMPOSE_FILE" down 2>/dev/null \
            || docker compose -f "$COMPOSE_FILE" down 2>/dev/null \
            || true
    fi

    printf "\n"
    printf "${YELLOW}This will remove the following files:${NC}\n"
    printf "  ${INSTALL_DIR}/${COMPOSE_FILE}\n"
    printf "  ${INSTALL_DIR}/Caddyfile\n"
    printf "  ${INSTALL_DIR}/.env\n"
    printf "  ${INSTALL_DIR}/.breadbox-version\n"
    printf "\n"
    printf "${YELLOW}Docker volumes (postgres, breadbox data, and caddy if used) are NOT removed.${NC}\n"
    printf "To remove all breadbox volumes (DESTROYS data):\n"
    printf "  docker volume rm \$(docker volume ls -q | grep breadbox)\n"
    printf "\n"

    # When invoked via `curl | bash -s -- --uninstall`, stdin is the pipe.
    # _bb_read_line falls back to /dev/tty so the prompt actually works.
    # If neither is available (cron, etc.), require --yes to avoid silently
    # destroying state.
    if [ "${AUTO_YES:-0}" != "1" ]; then
        printf "Continue? [y/N] "
        if ! _bb_read_line; then
            error "No terminal available for confirmation. Re-run with --yes to skip the prompt."
            exit 1
        fi
        case "$ans" in
            [yY]|[yY][eE][sS]) ;;
            *) info "Uninstall cancelled."; exit 0 ;;
        esac
    fi

    rm -f "$INSTALL_DIR/$COMPOSE_FILE"
    rm -f "$INSTALL_DIR/Caddyfile"
    rm -f "$INSTALL_DIR/.env"
    rm -f "$INSTALL_DIR/.breadbox-version"

    # Remove directory if empty
    rmdir "$INSTALL_DIR" 2>/dev/null || true

    printf "\n"
    success "Breadbox uninstalled."
    info "Database volume preserved. Remove manually if needed (see above)."
    exit 0
}

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------
check_command() {
    command -v "$1" >/dev/null 2>&1
}

# Helper: read one line into $ans. Tries the right source for each
# invocation pattern; returns 1 only when nothing produces a line.
#
# Preference order:
#   1. Stdin when it's a TTY — interactive `bash install.sh`
#   2. /dev/tty — `curl | bash` case (stdin is the drained install pipe,
#      but the controlling terminal is still available)
#   3. Stdin as a last resort — heredoc / here-string testing patterns
#      and headless ssh with piped input
#
# Per-read redirection avoids dash's quirk where `exec 3</dev/tty` exits
# the shell on open failure even inside a conditional.
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

# Prompt for a yes/no answer. Returns 0 for yes, 1 for no.
# Respects AUTO_YES (treats default as the answer when --yes is set).
prompt_yn() {
    question="$1"
    default="${2:-n}"

    if [ "${AUTO_YES:-0}" = "1" ]; then
        [ "$default" = "y" ] && return 0
        return 1
    fi

    if [ "$default" = "y" ]; then
        printf "%s [Y/n] " "$question"
    else
        printf "%s [y/N] " "$question"
    fi
    if ! _bb_read_line; then
        # No readable source (headless cron / -T ssh). Fall back to default.
        ans="$default"
    fi
    ans=${ans:-$default}
    case "$ans" in
        [yY]|[yY][eE][sS]) return 0 ;;
        *) return 1 ;;
    esac
}

# Prompt for a free-text value with a default.
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
        ans="$default"
    fi
    printf "%s" "${ans:-$default}"
}

# Detect the host's external HTTPS URL when running on a known VM
# platform. Prints the URL on stdout, or nothing if no platform was
# recognized.
#
# Currently supports exe.dev only. PaaS hosts like Fly.io / Railway /
# Render deploy via Dockerfile builds at the edge, not by running
# install.sh interactively — env-var-based detection for those was
# both unreachable in practice AND created a misdetection bug when
# stale FLY_APP_NAME etc. lingered in a developer's shell. PaaS port
# coupling is handled in the binary instead (12-factor $PORT
# fallback in internal/config/load.go).
#
# Adding another VM platform here: one if-block — probe whatever
# metadata service or env var it exposes, print the public URL.
detect_external_url() {
    # exe.dev: bounded HTTP probe to the link-local metadata service.
    if check_command curl; then
        meta=$(curl -fsS --max-time 2 http://169.254.169.254/ 2>/dev/null)
        if [ -n "$meta" ]; then
            # Metadata: {"name": "<vmname>", "source_ip": "..."}.
            # Prefer python3 for robust JSON parsing; fall back to a
            # grep+sed path so the function works on python-less hosts.
            vmname=""
            if check_command python3; then
                vmname=$(printf "%s" "$meta" | python3 -c \
                    'import json,sys; d=json.load(sys.stdin); print(d.get("name",""))' \
                    2>/dev/null)
            fi
            if [ -z "$vmname" ]; then
                vmname=$(printf "%s" "$meta" \
                    | grep -oE '"name"[[:space:]]*:[[:space:]]*"[^"]+"' \
                    | head -1 \
                    | sed 's/.*"name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
            fi
            if [ -n "$vmname" ]; then
                printf "https://%s.exe.xyz" "$vmname"
                return
            fi
        fi
    fi
}

# Fetch the latest release tag from GitHub API.
# Falls back to "latest" if the API call fails.
get_latest_tag() {
    tag=""
    if check_command curl; then
        tag=$(curl -fsSL "${GITHUB_API}/releases/latest" 2>/dev/null \
            | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"//;s/".*//')
    elif check_command wget; then
        tag=$(wget -qO- "${GITHUB_API}/releases/latest" 2>/dev/null \
            | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"//;s/".*//')
    fi
    printf "%s" "${tag:-latest}"
}

# Download a file. Tries curl, falls back to wget.
download() {
    url="$1"
    dest="$2"
    if check_command curl; then
        curl -fsSL "$url" -o "$dest"
    elif check_command wget; then
        wget -qO "$dest" "$url"
    else
        die "Neither curl nor wget found. Install one and retry."
    fi
}

# Try to install Docker via https://get.docker.com.
# Only Linux. Best-effort — if it fails, surface the error.
try_install_docker() {
    case "$BB_OS" in
        linux)
            info "Installing Docker via https://get.docker.com (detected: ${BB_DISTRO} ${BB_DISTRO_VERSION}, ${BB_ARCH}) ..."
            if check_command curl; then
                curl -fsSL https://get.docker.com | sh
            elif check_command wget; then
                wget -qO- https://get.docker.com | sh
            else
                die "Cannot install Docker: neither curl nor wget is available."
            fi

            # Add the invoking user to the docker group for non-sudo use,
            # when we are running via sudo.
            if [ -n "${SUDO_USER:-}" ]; then
                usermod -aG docker "$SUDO_USER" 2>/dev/null || true
                warn "Added ${SUDO_USER} to the 'docker' group. Log out and back in for this to take effect."
            fi
            ;;
        darwin)
            die "Automatic Docker install is not supported on macOS. Install Docker Desktop from https://docs.docker.com/desktop/install/mac-install/ and re-run this script."
            ;;
        *)
            die "Automatic Docker install is not supported on ${BB_OS}. Install Docker manually from https://docs.docker.com/get-docker/ and re-run this script."
            ;;
    esac
}

# Install openssl via the detected package manager if available. Skipped on
# systems where we don't know the package manager — the user can install it
# manually and re-run.
try_install_openssl() {
    case "$BB_PKG_MANAGER" in
        none)
            die "openssl is not installed and no supported package manager was detected. Install openssl manually and re-run this script."
            ;;
    esac
    info "Installing openssl via ${BB_PKG_MANAGER}..."
    if ! bb_pkg_install openssl; then
        die "Failed to install openssl via ${BB_PKG_MANAGER}. Install it manually and re-run."
    fi
}

# ---------------------------------------------------------------------------
# Daemon registration (opt-in)
# ---------------------------------------------------------------------------

register_daemon_systemd() {
    unit_template_url="${GITHUB_RAW}/main/deploy/daemon/breadbox.service.tmpl"
    unit_dest="/etc/systemd/system/breadbox.service"

    if [ "$(id -u)" != "0" ]; then
        warn "Registering a systemd unit requires root. To enable it, re-run as:"
        warn "  curl -fsSL https://breadbox.sh/install.sh | sudo bash -s -- --register-daemon"
        warn "(Note: 'sudo curl … | bash' applies sudo to curl only, not bash —"
        warn "the script then runs as your user. Put sudo in front of bash.)"
        return 1
    fi

    tmp_unit="${TMPDIR:-/tmp}/breadbox.service.$$"
    # Prefer local file if present (git checkout install).
    if [ -r "$(dirname "$0")/daemon/breadbox.service.tmpl" ]; then
        cp "$(dirname "$0")/daemon/breadbox.service.tmpl" "$tmp_unit"
    else
        download "$unit_template_url" "$tmp_unit" || { warn "Could not fetch systemd unit template"; return 1; }
    fi

    compose_cmd="docker compose ${CADDY_PROFILE} -f ${INSTALL_DIR}/${COMPOSE_FILE}"
    sed -e "s|__INSTALL_DIR__|${INSTALL_DIR}|g" \
        -e "s|__COMPOSE_CMD__|${compose_cmd}|g" \
        "$tmp_unit" > "$unit_dest"
    rm -f "$tmp_unit"

    systemctl daemon-reload
    systemctl enable breadbox.service >/dev/null 2>&1 || true
    success "Registered systemd unit: ${unit_dest} (enabled at boot)"
    info "Start: systemctl start breadbox    Stop: systemctl stop breadbox"
}

register_daemon_launchd() {
    plist_template_url="${GITHUB_RAW}/main/deploy/daemon/sh.breadbox.plist.tmpl"
    # User-level LaunchAgent — does not need root.
    plist_dest="${HOME}/Library/LaunchAgents/sh.breadbox.plist"
    mkdir -p "${HOME}/Library/LaunchAgents"

    tmp_plist="${TMPDIR:-/tmp}/sh.breadbox.plist.$$"
    if [ -r "$(dirname "$0")/daemon/sh.breadbox.plist.tmpl" ]; then
        cp "$(dirname "$0")/daemon/sh.breadbox.plist.tmpl" "$tmp_plist"
    else
        download "$plist_template_url" "$tmp_plist" || { warn "Could not fetch launchd template"; return 1; }
    fi

    compose_cmd="docker compose ${CADDY_PROFILE} -f ${INSTALL_DIR}/${COMPOSE_FILE}"
    sed -e "s|__INSTALL_DIR__|${INSTALL_DIR}|g" \
        -e "s|__COMPOSE_CMD__|${compose_cmd}|g" \
        "$tmp_plist" > "$plist_dest"
    rm -f "$tmp_plist"

    # bootout is the modern replacement for `launchctl unload`; tolerate
    # "not currently loaded" errors.
    launchctl bootout "gui/$(id -u)" "$plist_dest" >/dev/null 2>&1 || true
    if launchctl bootstrap "gui/$(id -u)" "$plist_dest" 2>/dev/null; then
        success "Registered launchd agent: ${plist_dest}"
    else
        warn "launchctl bootstrap failed. The plist is in place at ${plist_dest};"
        warn "run 'launchctl bootstrap gui/\$(id -u) ${plist_dest}' from a login shell to finish."
    fi
}

register_daemon() {
    case "$BB_INIT_SYSTEM" in
        systemd) register_daemon_systemd ;;
        launchd) register_daemon_launchd ;;
        *)
            warn "No supported init system detected (BB_INIT_SYSTEM=${BB_INIT_SYSTEM}). Skipping daemon registration."
            warn "Breadbox will still start now via 'docker compose up', but won't be re-started automatically on boot."
            ;;
    esac
}

# ---------------------------------------------------------------------------
# Parse arguments
# ---------------------------------------------------------------------------
AUTO_YES=0
INSTALL_DOCKER=0
REGISTER_DAEMON=0
NO_REGISTER_DAEMON=0
NO_START=0
PURGE_VOLUMES=0
DOMAIN_ARG=""
PORT_ARG=""
VERSION_ARG=""

for arg in "$@"; do
    case "$arg" in
        --uninstall) do_uninstall ;;
        --yes|-y) AUTO_YES=1 ;;
        --install-docker) INSTALL_DOCKER=1 ;;
        --register-daemon) REGISTER_DAEMON=1 ;;
        --no-register-daemon) NO_REGISTER_DAEMON=1 ;;
        --no-start) NO_START=1 ;;
        --purge-volumes) PURGE_VOLUMES=1 ;;
        --domain=*) DOMAIN_ARG="${arg#--domain=}" ;;
        --port=*) PORT_ARG="${arg#--port=}" ;;
        --version=*) VERSION_ARG="${arg#--version=}" ;;
        --help|-h)
            printf "Usage: install.sh [OPTIONS]\n\n"
            printf "Options:\n"
            printf "  --uninstall            Stop containers and remove installed files\n"
            printf "  --yes, -y              Skip interactive prompts; accept defaults\n"
            printf "  --install-docker       Install Docker automatically (Linux only)\n"
            printf "  --domain=HOST          Configure the install for HTTPS at HOST (enables Caddy)\n"
            printf "  --port=N               HTTP port to listen on (default: 8080)\n"
            printf "  --version=vX.Y.Z       Pin to a specific release tag (default: latest GitHub release)\n"
            printf "  --no-start             Write the install but don't 'docker compose up' it\n"
            printf "  --purge-volumes        Drop existing postgres/breadbox-data volumes before install\n"
            printf "                         (DESTRUCTIVE — wipes prior data; use when re-installing fresh)\n"
            printf "  --register-daemon      Register launchd (macOS) or systemd (Linux) unit\n"
            printf "  --no-register-daemon   Skip daemon registration (no boot-time autostart)\n"
            printf "  --help, -h             Show this help message\n"
            printf "\nEnvironment:\n"
            printf "  INSTALL_DIR   Installation directory\n"
            printf "                Default (root):        /opt/breadbox\n"
            printf "                Default (regular user): \$HOME/.breadbox\n"
            printf "  NO_COLOR      Disable colored output\n"
            exit 0
            ;;
    esac
done

# ---------------------------------------------------------------------------
# Main install
# ---------------------------------------------------------------------------
banner

# --- Pre-flight checks ---

info "Platform: ${BB_OS}/${BB_ARCH} (${BB_DISTRO} ${BB_DISTRO_VERSION}, pkg=${BB_PKG_MANAGER}, init=${BB_INIT_SYSTEM})"
info "Install directory: ${INSTALL_DIR}"
info "Checking prerequisites..."

# Hard-stop on unsupported OS.
case "$BB_OS" in
    linux|darwin) ;;
    *)
        die "Unsupported OS '${BB_OS}' (uname -s = $(uname -s 2>/dev/null || echo '?')). Supported: Linux, macOS."
        ;;
esac

# Warn on exotic architectures. We only ship amd64 + arm64 images.
case "$BB_ARCH" in
    amd64|arm64) ;;
    *)
        warn "Detected arch '${BB_ARCH}'. Official images are built for amd64 and arm64."
        warn "Installation may fail or run under emulation."
        ;;
esac

# Docker
if ! check_command docker; then
    warn "Docker is not installed."
    if [ "$INSTALL_DOCKER" = "1" ] || prompt_yn "Install Docker now via https://get.docker.com?" "n"; then
        try_install_docker
        check_command docker || die "Docker install did not make 'docker' available on PATH."
    else
        die "Docker is required. Install it from https://docs.docker.com/get-docker/ and re-run this script."
    fi
fi
success "Docker found"

# Docker Compose (v2 plugin)
if ! docker compose version >/dev/null 2>&1; then
    die "Docker Compose plugin not found. Install it from https://docs.docker.com/compose/install/ and re-run this script."
fi
success "Docker Compose found"

# Docker daemon running
if ! docker info >/dev/null 2>&1; then
    die "Docker daemon is not running. Start it and re-run this script."
fi
success "Docker daemon is running"

# openssl (for key generation)
if ! check_command openssl; then
    warn "openssl is not installed (needed for encryption key + db password generation)."
    if [ "$AUTO_YES" = "1" ] || prompt_yn "Install openssl via ${BB_PKG_MANAGER}?" "y"; then
        try_install_openssl
        check_command openssl || die "openssl still not available after install."
    else
        die "openssl is required. Install it and re-run this script."
    fi
fi
success "openssl found"

# --- Detect external URL (when on a recognized VM platform) ---
EXTERNAL_URL=$(detect_external_url)
if [ -n "$EXTERNAL_URL" ]; then
    success "Detected platform — public URL: ${EXTERNAL_URL}"
fi

printf "\n"

# --- Resolve version ---

if [ -n "$VERSION_ARG" ]; then
    info "Using pinned version: ${VERSION_ARG}"
    TAG="$VERSION_ARG"
    IMAGE_TAG="$VERSION_ARG"
else
    info "Fetching latest release..."
    TAG=$(get_latest_tag)
    if [ "$TAG" = "latest" ]; then
        # Pre-v0.1.0 there's no GitHub Release yet, so this fires for
        # every install. Phrase as neutral info rather than a warning
        # — the rolling :latest image is the right behavior in this
        # state.
        info "No release tagged yet — using :latest image (rolling)."
        IMAGE_TAG="latest"
    else
        success "Latest release: ${TAG}"
        IMAGE_TAG="$TAG"
    fi
fi

printf "\n"

# --- Check for existing installation ---

if [ -f "${INSTALL_DIR}/.env" ]; then
    warn "Existing .env at ${INSTALL_DIR}/.env — preserving it."
    info "${DIM}To reset, run --uninstall first. To change values, edit .env directly.${NC}"
    if [ -n "$DOMAIN_ARG" ] || [ -n "$PORT_ARG" ]; then
        warn "${DIM}--domain / --port ignored on reinstall; edit .env or --uninstall first.${NC}"
    fi
    printf "\n"
    ENV_EXISTS=1
else
    ENV_EXISTS=0
fi

# --- Domain prompt ---

DOMAIN_VALUE="$DOMAIN_ARG"
if [ -z "$DOMAIN_VALUE" ] && [ "$ENV_EXISTS" = "0" ]; then
    printf "\n"
    if [ -n "$EXTERNAL_URL" ]; then
        info "Public URL detected: ${EXTERNAL_URL} (HTTPS via the platform)."
        info "${DIM}Leave the domain prompt blank to use it. More: https://docs.breadbox.sh/install${NC}"
    else
        info "Public domain for HTTPS via Caddy (blank = localhost / behind external proxy)."
        info "${DIM}More: https://docs.breadbox.sh/install${NC}"
    fi
    DOMAIN_VALUE=$(prompt_value "Public domain" "")
fi

if [ -n "$DOMAIN_VALUE" ]; then
    info "Configuring for domain: ${DOMAIN_VALUE}"
    CADDY_PROFILE="--profile caddy"
else
    info "Localhost-only install (no HTTPS, no Caddy)"
    CADDY_PROFILE=""
fi

# --- Port prompt ---
#
# Most users want 8080 — but some platforms (exe.dev, some cloud PaaS,
# certain reverse proxies) route to a specific port number that isn't
# 8080. Prompting up-front catches those cases before the install
# completes and the user discovers "service unavailable" later.
PORT_VALUE="$PORT_ARG"
# 12-factor / PaaS convention: Heroku, Fly.io, Railway, Render, Cloud Run
# all inject PORT into the runtime environment. When the user runs the
# installer on a host where PORT is already exported, default to it
# instead of forcing them to discover and translate it. --port=N still
# wins; this only kicks in when --port wasn't given.
if [ -z "$PORT_VALUE" ] && [ -n "${PORT:-}" ]; then
    PORT_VALUE="$PORT"
    info "Detected PORT=${PORT} in the environment (12-factor convention) — using as default."
fi
if [ -z "$PORT_VALUE" ] && [ "$ENV_EXISTS" = "0" ]; then
    printf "\n"
    info "HTTP port (default 8080 — change if 8080 is taken or your proxy needs another)."
    PORT_VALUE=$(prompt_value "Port" "8080")
fi
PORT_VALUE="${PORT_VALUE:-8080}"
# Sanity-check: digits-only, 1-65535.
case "$PORT_VALUE" in
    ''|*[!0-9]*) die "--port must be a number, got: ${PORT_VALUE}" ;;
esac
if [ "$PORT_VALUE" -lt 1 ] || [ "$PORT_VALUE" -gt 65535 ]; then
    die "--port must be 1-65535, got: ${PORT_VALUE}"
fi
if [ "$PORT_VALUE" != "8080" ]; then
    info "Listening on port: ${PORT_VALUE}"
fi

printf "\n"

# --- Create install directory ---

mkdir -p "$INSTALL_DIR"

# --- Download deployment files ---

info "Downloading deployment files from ${TAG}..."

DOWNLOAD_REF="$TAG"
if [ "$TAG" = "latest" ]; then
    DOWNLOAD_REF="main"
fi

download "${GITHUB_RAW}/${DOWNLOAD_REF}/deploy/docker-compose.prod.yml" \
    "${INSTALL_DIR}/${COMPOSE_FILE}"
success "docker-compose.prod.yml"

download "${GITHUB_RAW}/${DOWNLOAD_REF}/deploy/Caddyfile" \
    "${INSTALL_DIR}/Caddyfile"
success "Caddyfile"

printf "\n"

# --- Pin image tag in compose file ---

if [ "$IMAGE_TAG" != "latest" ]; then
    # Replace :latest with the pinned tag in the downloaded compose file.
    # Use a temp file instead of sed -i for POSIX portability (BSD vs GNU sed).
    tmpfile="${INSTALL_DIR}/${COMPOSE_FILE}.tmp"
    sed "s|ghcr.io/${REPO}:latest|ghcr.io/${REPO}:${IMAGE_TAG}|g" \
        "${INSTALL_DIR}/${COMPOSE_FILE}" > "$tmpfile"
    mv "$tmpfile" "${INSTALL_DIR}/${COMPOSE_FILE}"
    info "Pinned image to ghcr.io/${REPO}:${IMAGE_TAG}"
fi

# Record the pinned tag for traceability and for any user-side scripting
# that wants to know which release this dir was installed against.
# "latest" signals the user picked rolling updates.
printf "%s\n" "$IMAGE_TAG" > "${INSTALL_DIR}/.breadbox-version"

# --- Generate .env ---

if [ "$ENV_EXISTS" -eq 0 ]; then
    info "Generating secrets..."

    ENCRYPTION_KEY=$(openssl rand -hex 32)
    POSTGRES_PASSWORD=$(openssl rand -hex 24)

    # Emit DOMAIN= (commented when not set) so users can flip it later without
    # hand-editing adjacent lines.
    if [ -n "$DOMAIN_VALUE" ]; then
        DOMAIN_LINE="DOMAIN=${DOMAIN_VALUE}"
    else
        DOMAIN_LINE="# DOMAIN=breadbox.example.com"
    fi

    cat > "${INSTALL_DIR}/.env" <<ENVEOF
# Breadbox Configuration
# Generated by install.sh on $(date -u +"%Y-%m-%dT%H:%M:%SZ")
# Docs: https://github.com/${REPO}/blob/main/deploy/README.md

# --- Database ---
DATABASE_URL=postgres://breadbox:${POSTGRES_PASSWORD}@db:5432/breadbox?sslmode=disable
POSTGRES_USER=breadbox
POSTGRES_PASSWORD=${POSTGRES_PASSWORD}
POSTGRES_DB=breadbox

# --- Security ---
ENCRYPTION_KEY=${ENCRYPTION_KEY}

# --- Server ---
SERVER_PORT=${PORT_VALUE}
ENVIRONMENT=docker

# --- Domain (for Caddy HTTPS) ---
# Uncomment to enable automatic TLS. Also re-run the install with the
# --domain flag or start the caddy profile manually:
#   docker compose --profile caddy -f ${COMPOSE_FILE} up -d
${DOMAIN_LINE}

# --- Plaid (optional, configure via dashboard at /providers) ---
# PLAID_CLIENT_ID=
# PLAID_SECRET=
# PLAID_ENV=sandbox

# --- Teller (optional, configure via dashboard at /providers) ---
# TELLER_APP_ID=
# TELLER_CERT_PATH=
# TELLER_KEY_PATH=
# TELLER_ENV=sandbox
# TELLER_WEBHOOK_SECRET=
ENVEOF

    chmod 600 "${INSTALL_DIR}/.env"
    success "Generated .env with fresh secrets"
else
    info "Using existing .env (not overwritten)"
fi

printf "\n"

# --- Detect existing postgres volume (cross-install footgun) ---
#
# When a user uninstalls and reinstalls, --uninstall preserves the
# postgres_data volume by design (protect user data). But this install
# generates a fresh POSTGRES_PASSWORD that doesn't match what's baked into
# the existing volume's pg_hba.conf, so breadbox can't authenticate and
# every migration attempt fails with "password authentication failed".
#
# Detect the leftover volume and force the user to choose: drop it (lose
# data, get a clean install) or abort and recover the old password from
# their previous .env backup.
PROJECT_NAME=$(basename "$INSTALL_DIR")
EXISTING_PG_VOLUME=$(docker volume ls --format '{{.Name}}' 2>/dev/null \
    | grep -E "^${PROJECT_NAME}_postgres_data$" | head -1 || true)

if [ "$ENV_EXISTS" = "0" ] && [ -n "$EXISTING_PG_VOLUME" ]; then
    warn "Existing postgres volume detected: ${EXISTING_PG_VOLUME}"
    warn "It was preserved by a previous --uninstall. The new .env has a"
    warn "fresh POSTGRES_PASSWORD that will NOT match the volume's stored"
    warn "credentials — breadbox would fail to authenticate."
    printf "\n"
    if [ "$PURGE_VOLUMES" = "1" ] || prompt_yn "Drop existing volumes and start fresh? (DESTROYS all prior data)" "n"; then
        info "Removing existing volumes..."
        # breadbox_transcripts / breadbox_backups are the pre-BB_DATA_DIR
        # volume names — list them too so --purge-volumes wipes old installs
        # cleanly. Fresh installs only have breadbox_data; the docker
        # volume rm calls for the legacy names will no-op via 2>/dev/null.
        for vol in "${PROJECT_NAME}_postgres_data" "${PROJECT_NAME}_breadbox_data" "${PROJECT_NAME}_breadbox_transcripts" "${PROJECT_NAME}_breadbox_backups"; do
            docker volume rm "$vol" 2>/dev/null && success "Removed $vol" || true
        done
        printf "\n"
    else
        # Remove the freshly-generated .env so the next install.sh run starts
        # from ENV_EXISTS=0 and re-prompts (otherwise the stale fresh-secrets
        # .env would silently keep failing auth).
        rm -f "${INSTALL_DIR}/.env"
        die "Aborted. To preserve the existing data, restore your previous .env (or its POSTGRES_PASSWORD) to ${INSTALL_DIR}/.env and re-run. To start fresh, re-run with --purge-volumes."
    fi
fi

# --- Skip start if requested ---

cd "$INSTALL_DIR"

if [ "$NO_START" = "1" ]; then
    success "Install written. Skipping 'docker compose up' (--no-start)."
    info "Review and start manually:"
    info "  cd ${INSTALL_DIR}"
    if [ -n "$CADDY_PROFILE" ]; then
        info "  docker compose --profile caddy -f ${COMPOSE_FILE} up -d"
    else
        info "  docker compose -f ${COMPOSE_FILE} up -d"
    fi
    exit 0
fi

# --- Pre-flight port check ---
#
# Catch "port already in use" up-front instead of letting the user wait 60s
# for the health probe to time out. Uses `ss` (universally available on
# systemd hosts via iproute2) — falls back to lsof if ss is missing.
# Compose's port-mapping clash error is also clear, but we want to refuse
# *before* generating volumes / pulling images / starting containers.
check_port_in_use() {
    port="$1"
    if command -v ss >/dev/null 2>&1; then
        ss -ltn "sport = :${port}" 2>/dev/null | tail -n +2 | grep -q .
    elif command -v lsof >/dev/null 2>&1; then
        lsof -iTCP:"${port}" -sTCP:LISTEN >/dev/null 2>&1
    else
        # No tool available — skip the check. Compose will surface the
        # error if there's a real conflict.
        return 1
    fi
}

if check_port_in_use "$PORT_VALUE"; then
    error "Port ${PORT_VALUE} is already in use on this host."
    error "Pick a different port with: --port=N (or stop the conflicting service first)."
    error "Check what's listening:"
    if command -v ss >/dev/null 2>&1; then
        error "  ss -ltnp 'sport = :${PORT_VALUE}'"
    else
        error "  lsof -iTCP:${PORT_VALUE} -sTCP:LISTEN"
    fi
    exit 1
fi

# --- Start services ---

info "Starting Breadbox..."
# Intentionally unquoted: CADDY_PROFILE is either empty or "--profile caddy"
# and we want the empty case to contribute no argument.
# shellcheck disable=SC2086
if ! docker compose $CADDY_PROFILE -f "$COMPOSE_FILE" up -d; then
    error "docker compose up failed. Last 20 lines of compose logs:"
    # Redact DSN-style credentials before dumping to the terminal — see the
    # health-failure block below for the same precaution.
    # shellcheck disable=SC2086
    docker compose $CADDY_PROFILE -f "$COMPOSE_FILE" logs --tail 20 2>&1 \
        | sed -E 's#(://[^:@/]+:)[^@]+@#\1REDACTED@#g' \
        || true
    exit 1
fi

printf "\n"

# --- Wait for healthy ---

# Pull SERVER_PORT from .env so a non-default port still gets probed correctly.
PORT=$(grep -E '^SERVER_PORT=' "${INSTALL_DIR}/.env" 2>/dev/null | head -1 | cut -d= -f2)
PORT="${PORT:-8080}"

info "Waiting for Breadbox to start..."
healthy=0
i=0
while [ "$i" -lt 60 ]; do
    if curl -sf "http://localhost:${PORT}/health/ready" >/dev/null 2>&1; then
        healthy=1
        break
    fi
    sleep 1
    i=$((i + 1))
done

printf "\n"

if [ "$healthy" -eq 1 ]; then
    # Optional daemon registration
    if [ "$NO_REGISTER_DAEMON" = "1" ]; then
        :  # user opted out
    elif [ "$REGISTER_DAEMON" = "1" ] \
        || { [ "$BB_INIT_SYSTEM" != "none" ] \
             && prompt_yn "Register a ${BB_INIT_SYSTEM} unit so Breadbox restarts on boot?" "n"; }; then
        printf "\n"
        register_daemon
    fi

    # Platform-reachability check. Breadbox is healthy locally on
    # ${PORT}. When a platform-supplied external URL exists, verify
    # the platform's edge proxy is routing to us.
    #
    # We use HTTP-status classification rather than curl's --fail
    # because a private VM (auth gate, 4xx) IS routing — the bug we
    # want to catch is "edge proxy points at the wrong port", which
    # surfaces as connection failure (000) or upstream-error (5xx).
    # Any 2xx/3xx/4xx means the platform is reaching Breadbox.
    PLATFORM_REACHABLE=1
    if [ -n "$EXTERNAL_URL" ]; then
        status=$(curl -sL --max-time 5 -o /dev/null -w '%{http_code}' \
            "${EXTERNAL_URL}/health/ready" 2>/dev/null)
        if [ -z "$status" ] || [ "$status" = "000" ] || [ "$status" -ge 500 ] 2>/dev/null; then
            PLATFORM_REACHABLE=0
        fi
    fi

    # Compute the URL we'll headline. Priority: user-supplied --domain >
    # detected platform external URL > localhost. The middle case
    # closes the loop on VM platforms (exe.dev) where the script can
    # derive the real public URL — no more "or visit your public URL"
    # guessing.
    if [ -n "$DOMAIN_VALUE" ]; then
        SETUP_URL="https://${DOMAIN_VALUE}/setup"
    elif [ -n "$EXTERNAL_URL" ]; then
        SETUP_URL="${EXTERNAL_URL}/setup"
    else
        SETUP_URL="http://localhost:${PORT}/setup"
    fi

    # Compose-command shortcuts for the footer hints. Both variants are
    # computed up front to avoid repeating long literal commands later.
    COMPOSE_CMD="docker compose -f ${COMPOSE_FILE}"
    CADDY_COMPOSE_CMD="docker compose --profile caddy -f ${COMPOSE_FILE}"
    if [ -n "$CADDY_PROFILE" ]; then
        COMPOSE_CMD="$CADDY_COMPOSE_CMD"
    fi

    printf "${GREEN}${BOLD}"
    printf "  =========================================\n"
    printf "    Breadbox is running!\n"
    printf "  =========================================\n"
    printf "${NC}\n"

    # Headline: the one thing the user needs to do RIGHT NOW.
    printf "  ${BOLD}→ Continue setup:${NC} ${BOLD}${SETUP_URL}${NC}\n"
    # Only show the "or your public URL" hint when we genuinely don't
    # know the public URL (no --domain and no platform detection).
    if [ -z "$DOMAIN_VALUE" ] && [ -z "$EXTERNAL_URL" ]; then
        printf "    ${DIM}(or your public URL if this host is behind a reverse proxy on port ${PORT})${NC}\n"
    fi
    # Platform-reachability warning. Breadbox is healthy locally; if
    # the public URL doesn't reach us, the bug is on the platform side
    # (proxy port mismatch is the canonical cause). Don't fail the
    # install — just surface the remediation.
    if [ "$PLATFORM_REACHABLE" -eq 0 ]; then
        printf "\n"
        warn "${EXTERNAL_URL} isn't routing to port ${PORT} yet — check your platform's proxy."
        case "$EXTERNAL_URL" in
            *.exe.xyz)
                vmname=$(printf "%s" "$EXTERNAL_URL" | sed 's|https://||;s|\.exe\.xyz||')
                warn "  Fix: ssh exe.dev share port ${vmname} ${PORT}"
                ;;
        esac
    fi
    printf "\n"

    # Quiet operational essentials — the rest lives in docs. Setup
    # wizard guides the user from /setup; bank/agent/backups setup
    # surfaces in the app itself (/getting-started).
    printf "  ${DIM}Config:    ${INSTALL_DIR}/.env${NC}\n"
    printf "  ${DIM}Logs:      cd ${INSTALL_DIR} && ${COMPOSE_CMD} logs -f${NC}\n"
    printf "  ${DIM}Uninstall: curl -fsSL https://breadbox.sh/install.sh | bash -s -- --uninstall${NC}\n"
    printf "  ${DIM}Docs:      https://docs.breadbox.sh/install${NC}\n"
    printf "\n"
else
    error "Breadbox did not become healthy within 60 seconds."
    error ""
    error "Common causes:"
    error "  1. Port ${PORT} is already used by another process on this host."
    error "     Re-run with --port=N to pick a different one, or stop the"
    error "     conflicting service. Check with:"
    error "       ss -ltnp 'sport = :${PORT}'    (or: lsof -iTCP:${PORT} -sTCP:LISTEN)"
    error "  2. Postgres is still initializing (rare on first install — wait"
    error "     a few seconds and retry from the install dir, or check the"
    error "     db logs: docker compose -f ${COMPOSE_FILE} logs db)"
    error "  3. The breadbox container hit a startup error (most likely the"
    error "     migration failed). See the last 30 lines:"
    error ""
    # Redact credentials embedded in DSN-style URLs before dumping logs to
    # the user's terminal. goose/pgx errors sometimes include the DSN, and
    # POSTGRES_PASSWORD was generated for this install just a moment ago —
    # we don't want it landing in scrollback / CI captures / shell history.
    # shellcheck disable=SC2086
    docker compose $CADDY_PROFILE -f "$COMPOSE_FILE" logs --tail 30 breadbox 2>&1 \
        | sed -E 's#(://[^:@/]+:)[^@]+@#\1REDACTED@#g' \
        || true
    error ""
    error "Full logs: cd ${INSTALL_DIR} && docker compose -f ${COMPOSE_FILE} logs"
    exit 1
fi
