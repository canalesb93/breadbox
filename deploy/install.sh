#!/bin/sh
set -eu

# Breadbox Install Script
# Usage:
#   curl -sSL https://raw.githubusercontent.com/canalesb93/breadbox/main/deploy/install.sh | bash
#   bash install.sh [--uninstall] [--yes] [--domain=...] [--install-docker]
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
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[1;33m'
    BLUE='\033[0;34m'
    BOLD='\033[1m'
    DIM='\033[2m'
    NC='\033[0m'
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
    printf "${YELLOW}Docker volumes (postgres_data, caddy_data, caddy_config) are NOT removed.${NC}\n"
    printf "To remove volumes: docker volume rm breadbox_postgres_data breadbox_caddy_data breadbox_caddy_config\n"
    printf "\n"

    printf "Continue? [y/N] "
    read -r confirm
    case "$confirm" in
        [yY]|[yY][eE][sS]) ;;
        *) info "Uninstall cancelled."; exit 0 ;;
    esac

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

# Prompt for a yes/no answer. Returns 0 for yes, 1 for no.
# Respects AUTO_YES (treats "yes" as the default when non-interactive).
prompt_yn() {
    question="$1"
    default="${2:-n}"

    if [ "${AUTO_YES:-0}" = "1" ]; then
        [ "$default" = "y" ] && return 0
        return 1
    fi

    if [ ! -t 0 ]; then
        # Non-interactive (piped). Use default.
        [ "$default" = "y" ] && return 0
        return 1
    fi

    if [ "$default" = "y" ]; then
        printf "%s [Y/n] " "$question"
    else
        printf "%s [y/N] " "$question"
    fi
    read -r ans
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

    if [ "${AUTO_YES:-0}" = "1" ] || [ ! -t 0 ]; then
        printf "%s" "$default"
        return
    fi

    if [ -n "$default" ]; then
        printf "%s [%s]: " "$question" "$default" >&2
    else
        printf "%s: " "$question" >&2
    fi
    read -r ans
    printf "%s" "${ans:-$default}"
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
    os=$(uname -s 2>/dev/null || echo unknown)
    case "$os" in
        Linux)
            info "Installing Docker via https://get.docker.com ..."
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
        Darwin)
            die "Automatic Docker install is not supported on macOS. Install Docker Desktop from https://docs.docker.com/desktop/install/mac-install/ and re-run this script."
            ;;
        *)
            die "Automatic Docker install is not supported on ${os}. Install Docker manually from https://docs.docker.com/get-docker/ and re-run this script."
            ;;
    esac
}

# ---------------------------------------------------------------------------
# Parse arguments
# ---------------------------------------------------------------------------
AUTO_YES=0
INSTALL_DOCKER=0
DOMAIN_ARG=""

for arg in "$@"; do
    case "$arg" in
        --uninstall) do_uninstall ;;
        --yes|-y) AUTO_YES=1 ;;
        --install-docker) INSTALL_DOCKER=1 ;;
        --domain=*) DOMAIN_ARG="${arg#--domain=}" ;;
        --help|-h)
            printf "Usage: install.sh [OPTIONS]\n\n"
            printf "Options:\n"
            printf "  --uninstall         Stop containers and remove installed files\n"
            printf "  --yes, -y           Skip interactive prompts; accept defaults\n"
            printf "  --install-docker    Install Docker automatically (Linux only)\n"
            printf "  --domain=HOST       Configure the install for HTTPS at HOST (enables Caddy)\n"
            printf "  --help, -h          Show this help message\n"
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

info "Install directory: ${INSTALL_DIR}"
info "Checking prerequisites..."

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
    die "openssl is not installed. It is needed to generate encryption keys."
fi
success "openssl found"

printf "\n"

# --- Resolve version ---

info "Fetching latest release..."
TAG=$(get_latest_tag)
if [ "$TAG" = "latest" ]; then
    warn "Could not determine latest release tag. Using :latest image."
    IMAGE_TAG="latest"
else
    success "Latest release: ${TAG}"
    IMAGE_TAG="$TAG"
fi

printf "\n"

# --- Check for existing installation ---

if [ -f "${INSTALL_DIR}/.env" ]; then
    warn "Existing .env found at ${INSTALL_DIR}/.env"
    info "To avoid overwriting your configuration, the existing .env will be preserved."
    info "To start fresh, run: $0 --uninstall"
    printf "\n"
    ENV_EXISTS=1
else
    ENV_EXISTS=0
fi

# --- Domain prompt ---

DOMAIN_VALUE="$DOMAIN_ARG"
if [ -z "$DOMAIN_VALUE" ] && [ "$ENV_EXISTS" = "0" ]; then
    printf "\n"
    info "Optional: configure a public domain for automatic HTTPS via Caddy."
    info "Leave blank for a localhost-only install (ports 80/443 not bound)."
    DOMAIN_VALUE=$(prompt_value "Public domain (e.g. breadbox.example.com)" "")
fi

if [ -n "$DOMAIN_VALUE" ]; then
    info "Configuring for domain: ${DOMAIN_VALUE}"
    CADDY_PROFILE="--profile caddy"
else
    info "Localhost-only install (no HTTPS, no Caddy)"
    CADDY_PROFILE=""
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

# Record the pinned tag so update.sh can preserve it across upgrades.
# "latest" signals the user explicitly wants rolling updates.
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
SERVER_PORT=8080
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

# --- Start services ---

info "Starting Breadbox..."
cd "$INSTALL_DIR"
# Intentionally unquoted: CADDY_PROFILE is either empty or "--profile caddy"
# and we want the empty case to contribute no argument.
# shellcheck disable=SC2086
docker compose $CADDY_PROFILE -f "$COMPOSE_FILE" up -d

printf "\n"

# --- Wait for healthy ---

info "Waiting for Breadbox to start..."
healthy=0
i=0
while [ "$i" -lt 60 ]; do
    if curl -sf http://localhost:8080/health/ready >/dev/null 2>&1; then
        healthy=1
        break
    fi
    sleep 1
    i=$((i + 1))
done

printf "\n"

if [ "$healthy" -eq 1 ]; then
    printf "${GREEN}${BOLD}"
    printf "  =========================================\n"
    printf "    Breadbox is running!\n"
    printf "  =========================================\n"
    printf "${NC}\n"
    if [ -n "$DOMAIN_VALUE" ]; then
        info "Public URL:    ${BOLD}https://${DOMAIN_VALUE}${NC}"
        info "Setup wizard:  ${BOLD}https://${DOMAIN_VALUE}/setup${NC}"
    else
        info "Setup wizard:  ${BOLD}http://localhost:8080/setup${NC}"
    fi
    info "Config file:   ${INSTALL_DIR}/.env"
    info "Version pin:   ${INSTALL_DIR}/.breadbox-version (${IMAGE_TAG})"
    if [ -n "$CADDY_PROFILE" ]; then
        info "View logs:     cd ${INSTALL_DIR} && docker compose --profile caddy -f ${COMPOSE_FILE} logs -f"
    else
        info "View logs:     cd ${INSTALL_DIR} && docker compose -f ${COMPOSE_FILE} logs -f"
    fi
    info "Update:        cd ${INSTALL_DIR} && ./update.sh   (preserves your pinned version)"
    info "Uninstall:     ${0} --uninstall"
    printf "\n"
    if [ -z "$DOMAIN_VALUE" ]; then
        printf "${DIM}To enable HTTPS later, edit .env to set DOMAIN=, then:${NC}\n"
        printf "${DIM}  cd ${INSTALL_DIR} && docker compose --profile caddy -f ${COMPOSE_FILE} up -d${NC}\n"
        printf "\n"
    fi
else
    error "Breadbox did not become healthy within 60 seconds."
    error "Check logs: cd ${INSTALL_DIR} && docker compose -f ${COMPOSE_FILE} logs"
    exit 1
fi
