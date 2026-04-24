#!/usr/bin/env bash
set -euo pipefail

# Breadbox Update Script
#
# Preserves the version pin written by install.sh (.breadbox-version).
# If the user installed v0.3.1, `update.sh` will pull v0.3.1 again, NOT
# silently roll them forward to main. Run `update.sh --bump vX.Y.Z` or
# `update.sh --bump latest` to explicitly change the pin.

# --- Color helpers ---
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

info()    { echo -e "${BLUE}[INFO]${NC} $*"; }
warn()    { echo -e "${YELLOW}[WARN]${NC} $*"; }
error()   { echo -e "${RED}[ERROR]${NC} $*" >&2; }
success() { echo -e "${GREEN}[OK]${NC} $*"; }

REPO="canalesb93/breadbox"

# --- Parse flags ---
AUTO_YES=false
BUMP_TARGET=""
for arg in "$@"; do
    case "$arg" in
        --yes|-y) AUTO_YES=true ;;
        --bump=*) BUMP_TARGET="${arg#--bump=}" ;;
        --bump)
            error "--bump requires a value, e.g. --bump=v0.4.0 or --bump=latest"
            exit 2
            ;;
        --help|-h)
            cat <<EOF
Usage: update.sh [OPTIONS]

Pulls the Breadbox image pinned in .breadbox-version and restarts the stack.

Options:
  --yes, -y          Skip confirmation prompt
  --bump=<tag>       Explicitly change the pinned version (e.g. --bump=v0.4.0,
                     --bump=latest). Rewrites .breadbox-version and the image
                     tag in docker-compose.prod.yml.
  --help, -h         Show this message
EOF
            exit 0
            ;;
    esac
done

# --- Determine install directory ---
# Mirror install.sh's convention: root → /opt/breadbox, user → $HOME/.breadbox.
if [[ -z "${INSTALL_DIR:-}" ]]; then
    if [[ "$(id -u)" = "0" ]]; then
        INSTALL_DIR="/opt/breadbox"
    else
        INSTALL_DIR="${HOME:-.}/.breadbox"
    fi
fi
COMPOSE_FILE="$INSTALL_DIR/docker-compose.prod.yml"
VERSION_FILE="$INSTALL_DIR/.breadbox-version"

if [[ ! -f "$COMPOSE_FILE" ]]; then
    error "Breadbox installation not found at $INSTALL_DIR"
    error "Set INSTALL_DIR if installed elsewhere."
    exit 1
fi

cd "$INSTALL_DIR"

# --- Resolve target tag ---
# Precedence: --bump=X > .breadbox-version > "latest" (with a warning).
if [[ -n "$BUMP_TARGET" ]]; then
    TARGET_TAG="$BUMP_TARGET"
    info "Bumping pinned version to: $TARGET_TAG"
elif [[ -f "$VERSION_FILE" ]]; then
    TARGET_TAG=$(tr -d '[:space:]' < "$VERSION_FILE")
    if [[ -z "$TARGET_TAG" ]]; then
        warn "$VERSION_FILE is empty; falling back to 'latest'"
        TARGET_TAG="latest"
    fi
    info "Target version from .breadbox-version: $TARGET_TAG"
else
    warn "No .breadbox-version file; defaulting to 'latest'"
    warn "Run with --bump=vX.Y.Z to pin to a specific release"
    TARGET_TAG="latest"
fi

# --- Rewrite compose image tag if bumping or repairing drift ---
# This is the CRITICAL fix: previously `docker compose pull` on an unpinned
# compose file would silently yank users forward. Now we rewrite the image
# line to match the pinned tag before pulling, so `pull` is always for the
# intended version.
if grep -Eq "^[[:space:]]*image: ghcr.io/${REPO}:" "$COMPOSE_FILE"; then
    CURRENT_IMAGE_TAG=$(grep -Eo "ghcr.io/${REPO}:[^\"'[:space:]]+" "$COMPOSE_FILE" | head -1 | sed "s|ghcr.io/${REPO}:||")
    if [[ "$CURRENT_IMAGE_TAG" != "$TARGET_TAG" ]]; then
        info "Rewriting compose image: ghcr.io/${REPO}:${CURRENT_IMAGE_TAG} → :${TARGET_TAG}"
        tmpfile="${COMPOSE_FILE}.tmp"
        # POSIX-portable (BSD vs GNU sed): write to temp, move over.
        sed "s|ghcr.io/${REPO}:${CURRENT_IMAGE_TAG}|ghcr.io/${REPO}:${TARGET_TAG}|g" \
            "$COMPOSE_FILE" > "$tmpfile"
        mv "$tmpfile" "$COMPOSE_FILE"
    fi
fi

# Persist the pin (create it if this is a legacy install that never had one).
printf "%s\n" "$TARGET_TAG" > "$VERSION_FILE"

# --- Detect caddy profile ---
# If the .env has DOMAIN set, assume caddy was started with --profile caddy.
CADDY_PROFILE=()
if [[ -f "$INSTALL_DIR/.env" ]] && grep -Eq '^[[:space:]]*DOMAIN=.+' "$INSTALL_DIR/.env"; then
    CADDY_PROFILE=(--profile caddy)
fi

# --- Get current version ---
CURRENT_VERSION="unknown"
if curl -sf http://localhost:8080/health/ready > /dev/null 2>&1; then
    CURRENT_VERSION=$(curl -sf http://localhost:8080/api/v1/version 2>/dev/null | grep -o '"version":"[^"]*"' | cut -d'"' -f4 || echo "unknown")
fi

info "Current running version: ${CURRENT_VERSION}"
info "Target version:          ${TARGET_TAG}"

# --- Confirm update ---
if [[ "$AUTO_YES" != true ]]; then
    read -rp "Pull ${TARGET_TAG} and restart? [Y/n]: " confirm
    confirm="${confirm:-Y}"
    if [[ "${confirm,,}" != "y" ]]; then
        info "Update cancelled."
        exit 0
    fi
fi

# --- Pull and restart ---
info "Pulling image..."
docker compose "${CADDY_PROFILE[@]}" -f docker-compose.prod.yml pull

info "Restarting services..."
docker compose "${CADDY_PROFILE[@]}" -f docker-compose.prod.yml up -d

# --- Wait for healthy ---
info "Waiting for Breadbox to become healthy..."
for _ in $(seq 1 60); do
    if curl -sf http://localhost:8080/health/ready > /dev/null 2>&1; then
        NEW_VERSION=$(curl -sf http://localhost:8080/api/v1/version 2>/dev/null | grep -o '"version":"[^"]*"' | cut -d'"' -f4 || echo "unknown")
        success "Updated: ${CURRENT_VERSION} → ${NEW_VERSION} (pinned: ${TARGET_TAG})"
        exit 0
    fi
    sleep 1
done

error "Breadbox did not become healthy within 60 seconds."
error "Check logs with: docker compose -f docker-compose.prod.yml logs"
exit 1
