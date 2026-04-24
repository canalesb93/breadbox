#!/bin/sh
# Breadbox bootstrap
#
# This is the tiny shim hosted at https://breadbox.sh/install.sh. It does
# nothing more than fetch the real installer from GitHub and execute it,
# forwarding any arguments. Canonical source for this file lives in the
# Breadbox repo at deploy/bootstrap.sh; it is copied into the landing-site
# repo (breadbox-site) on release. Keep them in sync.
#
# Usage:
#   curl -fsSL https://breadbox.sh/install.sh | bash
#   curl -fsSL https://breadbox.sh/install.sh | bash -s -- --domain=my.example.com
#
# We deliberately don't do any work here beyond fetch + exec so that
# bug fixes to the installer land in one place (deploy/install.sh).

set -eu

REPO="canalesb93/breadbox"
# BB_INSTALL_REF pins the installer to a specific branch or tag. Defaults to
# main. Set e.g. BB_INSTALL_REF=v0.4.0 to bootstrap from a specific release.
REF="${BB_INSTALL_REF:-main}"
URL="https://raw.githubusercontent.com/${REPO}/${REF}/deploy/install.sh"

echo "==> Fetching Breadbox installer from ${URL}"

if command -v curl >/dev/null 2>&1; then
    # -f = fail on HTTP errors; -s = silent; -S = show errors; -L = follow redirects.
    tmpfile="${TMPDIR:-/tmp}/breadbox-install.$$.sh"
    trap 'rm -f "$tmpfile"' EXIT INT TERM
    curl -fsSL "$URL" -o "$tmpfile"
    sh "$tmpfile" "$@"
elif command -v wget >/dev/null 2>&1; then
    tmpfile="${TMPDIR:-/tmp}/breadbox-install.$$.sh"
    trap 'rm -f "$tmpfile"' EXIT INT TERM
    wget -qO "$tmpfile" "$URL"
    sh "$tmpfile" "$@"
else
    echo "error: neither curl nor wget found. Install one of them and retry." >&2
    exit 1
fi
