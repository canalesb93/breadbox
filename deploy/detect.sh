#!/bin/sh
# detect.sh — platform detection helpers for the Breadbox installer.
#
# Sourced by install.sh. Sets the following variables when run:
#
#   BB_OS            linux | darwin | unknown
#   BB_ARCH          amd64 | arm64 | <raw uname -m>
#   BB_DISTRO        debian | ubuntu | fedora | rhel | arch | alpine | macos | unknown
#   BB_DISTRO_VERSION version string from /etc/os-release (best effort)
#   BB_PKG_MANAGER   apt | dnf | yum | pacman | apk | brew | none
#   BB_INIT_SYSTEM   systemd | launchd | none
#
# All variables are non-empty on exit; unknown values are literal "unknown"
# or "none". The detector does not touch the network and is safe to dot-source.
#
# CLI: running `deploy/detect.sh` directly prints the detected values and
# exits 0. `deploy/detect.sh --test` runs the self-tests at the bottom of
# this file.

# shellcheck disable=SC2034  # variables are consumed by dot-sourcing caller.

detect_os() {
    case "$(uname -s 2>/dev/null || echo unknown)" in
        Linux) BB_OS=linux ;;
        Darwin) BB_OS=darwin ;;
        *) BB_OS=unknown ;;
    esac
}

detect_arch() {
    raw=$(uname -m 2>/dev/null || echo unknown)
    case "$raw" in
        x86_64|amd64) BB_ARCH=amd64 ;;
        aarch64|arm64) BB_ARCH=arm64 ;;
        armv7l|armv7|armhf) BB_ARCH=armv7 ;;
        *) BB_ARCH="$raw" ;;
    esac
}

detect_distro() {
    BB_DISTRO=unknown
    BB_DISTRO_VERSION=unknown

    if [ "$BB_OS" = "darwin" ]; then
        BB_DISTRO=macos
        BB_DISTRO_VERSION=$(sw_vers -productVersion 2>/dev/null || echo unknown)
        return
    fi

    # /etc/os-release is the standard across modern Linux distros.
    if [ -r /etc/os-release ]; then
        # Parse ID= and VERSION_ID= without sourcing (avoid shell surprises).
        id_raw=$(grep -E '^ID=' /etc/os-release | head -1 | sed 's/^ID=//;s/"//g')
        ver_raw=$(grep -E '^VERSION_ID=' /etc/os-release | head -1 | sed 's/^VERSION_ID=//;s/"//g')

        case "$id_raw" in
            debian|ubuntu|fedora|arch|alpine)
                BB_DISTRO="$id_raw"
                ;;
            rhel|centos|rocky|almalinux)
                BB_DISTRO=rhel
                ;;
            *)
                # Fall back to ID_LIKE for derivatives.
                like_raw=$(grep -E '^ID_LIKE=' /etc/os-release | head -1 | sed 's/^ID_LIKE=//;s/"//g')
                case "$like_raw" in
                    *debian*|*ubuntu*) BB_DISTRO=debian ;;
                    *rhel*|*fedora*) BB_DISTRO=rhel ;;
                    *arch*) BB_DISTRO=arch ;;
                    *) BB_DISTRO=unknown ;;
                esac
                ;;
        esac

        [ -n "$ver_raw" ] && BB_DISTRO_VERSION="$ver_raw"
    fi
}

detect_pkg_manager() {
    if [ "$BB_OS" = "darwin" ]; then
        if command -v brew >/dev/null 2>&1; then
            BB_PKG_MANAGER=brew
        else
            BB_PKG_MANAGER=none
        fi
        return
    fi

    # Order matters: prefer dnf over yum, apt over nothing, etc.
    for candidate in apt dnf yum pacman apk; do
        if command -v "$candidate" >/dev/null 2>&1; then
            BB_PKG_MANAGER="$candidate"
            return
        fi
    done
    BB_PKG_MANAGER=none
}

detect_init_system() {
    case "$BB_OS" in
        darwin)
            BB_INIT_SYSTEM=launchd
            ;;
        linux)
            # systemd is the de facto default on modern distros.
            # `systemctl --version` succeeding is the cleanest signal.
            if command -v systemctl >/dev/null 2>&1 && systemctl --version >/dev/null 2>&1; then
                BB_INIT_SYSTEM=systemd
            else
                BB_INIT_SYSTEM=none
            fi
            ;;
        *)
            BB_INIT_SYSTEM=none
            ;;
    esac
}

# Install a package via the detected package manager. Best-effort; callers
# should check $? and fall through to a manual-install message on failure.
#
# Usage: bb_pkg_install <package-name>
bb_pkg_install() {
    pkg="$1"
    [ -z "$pkg" ] && return 2
    case "$BB_PKG_MANAGER" in
        apt)
            DEBIAN_FRONTEND=noninteractive apt-get update -qq \
              && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq "$pkg"
            ;;
        dnf)  dnf install -y "$pkg" ;;
        yum)  yum install -y "$pkg" ;;
        pacman) pacman -S --noconfirm "$pkg" ;;
        apk)  apk add --no-cache "$pkg" ;;
        brew) brew install "$pkg" ;;
        *) return 1 ;;
    esac
}

bb_detect_all() {
    detect_os
    detect_arch
    detect_distro
    detect_pkg_manager
    detect_init_system
}

# If invoked directly, print the detected values (or run self-tests).
if [ "$(basename "${0:-}")" = "detect.sh" ]; then
    bb_detect_all

    if [ "${1:-}" = "--test" ]; then
        # Minimal self-tests: every variable must be non-empty, and the OS
        # must be a recognised value (the fallback to "unknown" is allowed,
        # but the variable itself must be set).
        fail=0
        for var in BB_OS BB_ARCH BB_DISTRO BB_DISTRO_VERSION BB_PKG_MANAGER BB_INIT_SYSTEM; do
            eval "val=\${$var:-}"
            if [ -z "$val" ]; then
                printf "FAIL: %s is empty\n" "$var" >&2
                fail=1
            fi
        done
        # OS should be one of the known values.
        case "$BB_OS" in
            linux|darwin|unknown) ;;
            *) printf "FAIL: BB_OS=%s is not one of {linux,darwin,unknown}\n" "$BB_OS" >&2; fail=1 ;;
        esac
        # Arch should be non-empty and not start with "unknown"-as-prefix oddities.
        [ -z "$BB_ARCH" ] && { printf "FAIL: BB_ARCH empty\n" >&2; fail=1; }

        if [ "$fail" = "0" ]; then
            printf "ok: all detect.sh self-tests passed\n"
            exit 0
        else
            exit 1
        fi
    fi

    printf "BB_OS=%s\n" "$BB_OS"
    printf "BB_ARCH=%s\n" "$BB_ARCH"
    printf "BB_DISTRO=%s\n" "$BB_DISTRO"
    printf "BB_DISTRO_VERSION=%s\n" "$BB_DISTRO_VERSION"
    printf "BB_PKG_MANAGER=%s\n" "$BB_PKG_MANAGER"
    printf "BB_INIT_SYSTEM=%s\n" "$BB_INIT_SYSTEM"
fi
