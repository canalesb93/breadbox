#!/bin/bash
set -euo pipefail

# Only run in Claude Code Web (remote) sessions
if [ "${CLAUDE_CODE_REMOTE:-}" != "true" ]; then
  exit 0
fi

cd "$CLAUDE_PROJECT_DIR"

echo "==> Downloading Go modules..."
GONOSUMCHECK=* GONOSUMDB=* GOPROXY=direct go mod download

echo "==> Installing sqlc..."
SQLC_VERSION="1.28.0"
if ! command -v sqlc &>/dev/null; then
  curl -sL "https://github.com/sqlc-dev/sqlc/releases/download/v${SQLC_VERSION}/sqlc_${SQLC_VERSION}_linux_amd64.tar.gz" -o /tmp/sqlc.tar.gz
  tar -xzf /tmp/sqlc.tar.gz -C /usr/local/bin sqlc
  rm -f /tmp/sqlc.tar.gz
fi

echo "==> Generating sqlc code..."
sqlc generate

echo "==> Building CSS (downloads tailwindcss-extra if needed)..."
make css

echo "==> Session setup complete."
