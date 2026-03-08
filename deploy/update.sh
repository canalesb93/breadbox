#!/usr/bin/env bash
set -euo pipefail

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

# --- Parse flags ---
AUTO_YES=false
for arg in "$@"; do
    case "$arg" in
        --yes|-y) AUTO_YES=true ;;
    esac
done

# --- Determine install directory ---
INSTALL_DIR="${INSTALL_DIR:-/opt/breadbox}"
COMPOSE_FILE="$INSTALL_DIR/docker-compose.prod.yml"

if [[ ! -f "$COMPOSE_FILE" ]]; then
    error "Breadbox installation not found at $INSTALL_DIR"
    error "Set INSTALL_DIR if installed elsewhere."
    exit 1
fi

cd "$INSTALL_DIR"

# --- Get current version ---
CURRENT_VERSION="unknown"
if curl -sf http://localhost:8080/health/ready > /dev/null 2>&1; then
    CURRENT_VERSION=$(curl -sf http://localhost:8080/api/v1/version 2>/dev/null | grep -o '"version":"[^"]*"' | cut -d'"' -f4 || echo "unknown")
fi

info "Current version: ${CURRENT_VERSION}"

# --- Confirm update ---
if [[ "$AUTO_YES" != true ]]; then
    read -rp "Pull latest images and restart? [Y/n]: " confirm
    confirm="${confirm:-Y}"
    if [[ "${confirm,,}" != "y" ]]; then
        info "Update cancelled."
        exit 0
    fi
fi

# --- Pull and restart ---
info "Pulling latest images..."
docker compose -f docker-compose.prod.yml pull

info "Restarting services..."
docker compose -f docker-compose.prod.yml up -d

# --- Wait for healthy ---
info "Waiting for Breadbox to become healthy..."
for i in $(seq 1 60); do
    if curl -sf http://localhost:8080/health/ready > /dev/null 2>&1; then
        NEW_VERSION=$(curl -sf http://localhost:8080/api/v1/version 2>/dev/null | grep -o '"version":"[^"]*"' | cut -d'"' -f4 || echo "unknown")
        success "Updated: ${CURRENT_VERSION} → ${NEW_VERSION}"
        exit 0
    fi
    sleep 1
done

error "Breadbox did not become healthy within 60 seconds."
error "Check logs with: docker compose -f docker-compose.prod.yml logs"
exit 1
