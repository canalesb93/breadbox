#!/bin/bash
# Session startup hook for Claude Code.
# - Remote (web) sessions: installs tools and generates build artifacts from scratch
# - Local worktree sessions: verifies build and injects env vars
#   (file copying is handled by .worktreeinclude)

PROJECT_DIR="${CLAUDE_PROJECT_DIR:-$(pwd)}"
cd "$PROJECT_DIR"

# --- Local sessions ---
if [ "${CLAUDE_CODE_REMOTE:-}" != "true" ]; then
  # Check if we're in a git worktree
  GIT_COMMON="$(git rev-parse --git-common-dir 2>/dev/null || echo ".git")"
  if [ "$GIT_COMMON" = ".git" ]; then
    # Main repo — nothing to do
    exit 0
  fi

  # Worktree: verify build works with the copied artifacts
  echo "==> Worktree session — verifying Go build..."
  if go build ./... 2>&1; then
    echo "    Build OK"
  else
    echo "WARN: go build failed — check for missing generated files"
  fi

  # Inject env vars if CLAUDE_ENV_FILE is available
  if [ -n "${CLAUDE_ENV_FILE:-}" ]; then
    # DATABASE_URL
    if [ -z "${DATABASE_URL:-}" ]; then
      echo 'DATABASE_URL=postgres://breadbox:breadbox@localhost:5432/breadbox?sslmode=disable' >> "$CLAUDE_ENV_FILE"
      echo "    Set DATABASE_URL"
    fi

    # ENCRYPTION_KEY — grab from running breadbox process
    if [ -z "${ENCRYPTION_KEY:-}" ]; then
      RUNNING_PID="$(pgrep -f 'breadbox serve' | head -1 || true)"
      if [ -n "$RUNNING_PID" ]; then
        EK="$(ps eww -p "$RUNNING_PID" 2>/dev/null | tr ' ' '\n' | grep '^ENCRYPTION_KEY=' | cut -d= -f2 || true)"
        if [ -n "$EK" ]; then
          echo "ENCRYPTION_KEY=$EK" >> "$CLAUDE_ENV_FILE"
          echo "    Set ENCRYPTION_KEY from running process"
        fi
      fi
    fi

    # PORT — find next available port starting from 8081
    # Uses lock files under main repo's .claude/ to prevent races between
    # concurrent worktree sessions that haven't started their server yet.
    if [ -z "${PORT:-}" ]; then
      MAIN_REPO="$(dirname "$GIT_COMMON")"
      PORT_LOCKS="$MAIN_REPO/.claude/port-locks"
      mkdir -p "$PORT_LOCKS"

      # Clean up stale lock files (port not in use AND lock older than 5 min)
      for lockfile in "$PORT_LOCKS"/*; do
        [ -f "$lockfile" ] || continue
        LOCK_PORT="$(basename "$lockfile")"
        if ! lsof -i :"$LOCK_PORT" >/dev/null 2>&1; then
          LOCK_AGE=$(( $(date +%s) - $(stat -f %m "$lockfile" 2>/dev/null || echo 0) ))
          if [ "$LOCK_AGE" -gt 300 ]; then
            rm -f "$lockfile"
          fi
        fi
      done

      PORT=8081
      while [ "$PORT" -le 8099 ]; do
        if ! lsof -i :"$PORT" >/dev/null 2>&1 && ! [ -f "$PORT_LOCKS/$PORT" ]; then
          # Claim it atomically
          if (set -o noclobber; echo "$$" > "$PORT_LOCKS/$PORT") 2>/dev/null; then
            break
          fi
        fi
        PORT=$((PORT + 1))
      done

      if [ "$PORT" -le 8099 ]; then
        echo "PORT=$PORT" >> "$CLAUDE_ENV_FILE"
        echo "    Set PORT=$PORT (use 'make dev' to start server)"
      else
        echo "WARN: no available port in 8081-8099"
      fi
    fi
  fi

  echo "==> Worktree setup complete."
  exit 0
fi

# --- Remote (web) sessions below ---

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
