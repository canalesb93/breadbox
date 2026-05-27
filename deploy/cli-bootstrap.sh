#!/bin/sh
# Breadbox CLI bootstrap
#
# This is the tiny shim hosted at https://breadbox.sh/cli.sh. It does
# nothing more than fetch the real CLI installer from GitHub and execute
# it, forwarding any arguments. Canonical source for this file lives in
# the Breadbox repo at deploy/cli-bootstrap.sh; it is copied into the
# landing-site repo (breadbox-site) on release. Keep them in sync.
#
# Usage:
#   curl -fsSL https://breadbox.sh/cli.sh | bash
#   curl -fsSL https://breadbox.sh/cli.sh | bash -s -- --host=https://breadbox.example.com
#
# We deliberately don't do any work here beyond fetch + exec so that
# bug fixes to the installer land in one place (deploy/cli.sh).

set -eu

REPO="canalesb93/breadbox"
# BB_INSTALL_REF pins the installer to a specific branch or tag. Defaults
# to main. Set e.g. BB_INSTALL_REF=v0.4.0 to bootstrap from a release.
REF="${BB_INSTALL_REF:-main}"
URL="https://raw.githubusercontent.com/${REPO}/${REF}/deploy/cli.sh"

echo "==> Fetching Breadbox CLI installer from ${URL}"

tmpfile="${TMPDIR:-/tmp}/breadbox-cli-install.$$.sh"
trap 'rm -f "$tmpfile"' EXIT INT TERM

if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$URL" -o "$tmpfile"
elif command -v wget >/dev/null 2>&1; then
    wget -qO "$tmpfile" "$URL"
else
    echo "error: neither curl nor wget found. Install one and retry." >&2
    exit 1
fi

# cli.sh uses bash features. Use bash explicitly so its shebang's
# semantics apply even on systems where /bin/sh is dash.
if command -v bash >/dev/null 2>&1; then
    bash "$tmpfile" "$@"
else
    echo "error: bash not found. Breadbox's CLI installer requires bash. Install it via your package manager (e.g. 'apt install bash') and retry." >&2
    exit 1
fi
