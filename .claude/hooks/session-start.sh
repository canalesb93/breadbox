#!/bin/bash
# Session startup hook for Claude Code Web.
# Installs tools and generates gitignored build artifacts.

# Only run in Claude Code Web (remote) sessions
if [ "${CLAUDE_CODE_REMOTE:-}" != "true" ]; then
  exit 0
fi

PROJECT_DIR="${CLAUDE_PROJECT_DIR:-$(pwd)}"
cd "$PROJECT_DIR"

SQLC_VERSION="1.30.0"

# --- Go modules ---
echo "==> Downloading Go modules..."
if ! GONOSUMCHECK=* GONOSUMDB=* GOPROXY=direct go mod download 2>&1; then
  echo "WARN: go mod download failed, continuing anyway"
fi

# --- sqlc ---
echo "==> Installing sqlc ${SQLC_VERSION}..."
if ! command -v sqlc &>/dev/null || [ "$(sqlc version 2>/dev/null)" != "v${SQLC_VERSION}" ]; then
  curl -sL "https://github.com/sqlc-dev/sqlc/releases/download/v${SQLC_VERSION}/sqlc_${SQLC_VERSION}_linux_amd64.tar.gz" \
    | tar xz -C /usr/local/bin sqlc \
    && echo "    sqlc $(sqlc version) installed" \
    || echo "WARN: sqlc install failed"
fi

echo "==> Generating sqlc code..."
if ! sqlc generate 2>&1; then
  echo "WARN: sqlc generate failed"
fi

# --- CSS ---
echo "==> Building CSS (downloads tailwindcss-extra if needed)..."
if ! make css 2>&1; then
  echo "WARN: make css failed"
fi

# --- Test database ---
echo "==> Setting up test database..."
if command -v pg_isready &>/dev/null; then
  # Start PostgreSQL if not running
  if ! pg_isready -q 2>/dev/null; then
    pg_ctlcluster 16 main start 2>/dev/null || true
  fi
  # Create test user and database (idempotent)
  sudo -u postgres psql -tc "SELECT 1 FROM pg_roles WHERE rolname='breadbox'" 2>/dev/null | grep -q 1 \
    || sudo -u postgres psql -c "CREATE ROLE breadbox WITH LOGIN PASSWORD 'breadbox'" 2>/dev/null
  sudo -u postgres psql -tc "SELECT 1 FROM pg_database WHERE datname='breadbox_test'" 2>/dev/null | grep -q 1 \
    || sudo -u postgres createdb -O breadbox breadbox_test 2>/dev/null
  echo "    breadbox_test database ready"
else
  echo "WARN: pg_isready not found, skipping test DB setup"
fi

echo "==> Session setup complete."
