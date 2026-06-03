#!/usr/bin/env bash
# dev-lib.sh — shared helpers for managed Breadbox dev servers + deterministic
# port assignment. Sourced by scripts/dev-server, scripts/dev-port,
# scripts/ui-validate, and the session-start / session-end hooks.
#
# Goals:
#   1. One place that decides which port a worktree's dev server uses, so no
#      tool has to "fish" for a free port.
#   2. A registry of running servers (keyed by port, tagged with worktree +
#      pid) so they can be found, reused, and reaped — no more orphaned
#      `breadbox serve` processes piling up after sessions close.
#
# Pure bash 3.2 + coreutils + lsof + curl. No jq, no associative arrays
# (macOS ships bash 3.2). Local-dev only — cloud sessions pin SERVER_PORT=8081
# and don't use the background manager.

# --- tunables -------------------------------------------------------------
BB_PORT_MIN="${BB_PORT_MIN:-8081}"
BB_PORT_MAX="${BB_PORT_MAX:-8099}"

# --- paths ----------------------------------------------------------------
bb_state_dir()   { printf '%s\n' "${BB_STATE_DIR:-$HOME/.local/share/breadbox}"; }
bb_servers_dir() { printf '%s\n' "$(bb_state_dir)/dev-servers"; }
bb_log()         { printf '%s\n' "$*" >&2; }

# Worktree root for a dir (defaults to cwd). Falls back to the dir itself.
bb_worktree_root() {
  local d="${1:-$PWD}"
  ( cd "$d" 2>/dev/null && git rev-parse --show-toplevel 2>/dev/null ) \
    || printf '%s\n' "$d"
}

bb_current_branch() {
  local d="${1:-$PWD}"
  ( cd "$d" 2>/dev/null && git rev-parse --abbrev-ref HEAD 2>/dev/null ) || true
}

# True only inside a linked worktree (git worktree add), not the main checkout.
# In a linked worktree --git-dir (.../.git/worktrees/<name>) differs from
# --git-common-dir (.../.git); in the main repo they match.
bb_is_linked_worktree() {
  local d="${1:-$PWD}" gd cd
  gd="$( cd "$d" 2>/dev/null && git rev-parse --git-dir 2>/dev/null )" || return 1
  cd="$( cd "$d" 2>/dev/null && git rev-parse --git-common-dir 2>/dev/null )" || return 1
  [ -n "$gd" ] && [ "$gd" != "$cd" ]
}

# --- primitives -----------------------------------------------------------
# -nP: skip DNS + service-name lookups; -sTCP:LISTEN: only count a listener.
bb_port_in_use() { lsof -nP -iTCP:"$1" -sTCP:LISTEN >/dev/null 2>&1; }
bb_pid_alive()   { [ -n "$1" ] && kill -0 "$1" >/dev/null 2>&1; }
bb_now()         { date +%s; }
bb_mtime()       { stat -f %m "$1" 2>/dev/null || stat -c %Y "$1" 2>/dev/null || echo 0; }
bb_health()      { curl -fsS -o /dev/null --max-time 2 "http://localhost:$1/health/live" 2>/dev/null; }

# Claude Code exposes the session id to hooks (stdin JSON) and to the session
# environment as CLAUDE_CODE_SESSION_ID. Older builds used CLAUDE_SESSION_ID, so
# accept both. Used to tag registry entries with their creating session so
# session-end won't kill a server another live session in the same worktree owns.
bb_session_id()  { printf '%s' "${CLAUDE_CODE_SESSION_ID:-${CLAUDE_SESSION_ID:-}}"; }

# meta files are simple key=value lines. pid is a number, or RESERVED (a port
# held by session-start before any server is booted).
bb_meta_get() { # <file> <key>
  [ -f "$1" ] || return 1
  sed -n "s/^$2=//p" "$1" | head -1
}

# --- env ------------------------------------------------------------------
# Populate DATABASE_URL + ENCRYPTION_KEY for a server boot. Mirrors the
# session-start hook's resolution: .local.env (canonical) -> on-disk key cache.
bb_load_env() { # <root>
  local root="$1" cache cand _opts
  if [ -f "$root/.local.env" ]; then
    # Source tolerantly: a .local.env that references an unset var must not
    # abort a caller running `set -euo pipefail`. Save + restore errexit/nounset.
    _opts="$-"
    set -a; set +eu
    . "$root/.local.env" || true
    case "$_opts" in *e*) set -e;; esac
    case "$_opts" in *u*) set -u;; esac
    case "$_opts" in *a*) ;; *) set +a;; esac   # only disable allexport if caller had it off
  fi
  export DATABASE_URL="${DATABASE_URL:-postgres://breadbox:breadbox@localhost:5432/breadbox?sslmode=disable}"
  if [ -z "${ENCRYPTION_KEY:-}" ]; then
    cache="$(bb_state_dir)/dev-encryption-key"
    if [ -f "$cache" ]; then
      cand="$(head -1 "$cache" 2>/dev/null | tr -d '\r\n')"
      if [[ "$cand" =~ ^[0-9a-fA-F]{64}$ ]]; then export ENCRYPTION_KEY="$cand"; fi
    fi
  fi
}

# --- registry -------------------------------------------------------------
# Print the port whose registry entry is owned by <root>, if any.
bb_server_for_worktree() { # <root>
  local root="$1" dir f wt
  dir="$(bb_servers_dir)"
  [ -d "$dir" ] || return 1
  for f in "$dir"/*; do
    [ -f "$f" ] || continue
    wt="$(bb_meta_get "$f" worktree)"
    if [ "$wt" = "$root" ]; then
      basename "$f"
      return 0
    fi
  done
  return 1
}

# Claim a port for <root>, writing a placeholder registry entry. Preference:
# explicit SERVER_PORT/PORT env -> existing .breadbox-port -> existing
# reservation owned by this root -> first free port in range. <state> is the
# pid field to stamp (RESERVED for a reservation, PENDING for an imminent boot).
# Echoes the claimed port, or returns non-zero if the range is exhausted.
bb_claim_port() { # <root> <state>
  local root="$1" state="${2:-PENDING}" dir port pref owner opid
  dir="$(bb_servers_dir)"; mkdir -p "$dir"

  pref=""
  [ -n "${SERVER_PORT:-}" ] && pref="$SERVER_PORT"
  if [ -z "$pref" ] && [ -n "${PORT:-}" ] && [ "$PORT" != "8080" ]; then pref="$PORT"; fi
  if [ -z "$pref" ] && [ -f "$root/.breadbox-port" ]; then
    pref="$(head -1 "$root/.breadbox-port" 2>/dev/null | tr -dc '0-9')"
  fi

  for port in $pref $(seq "$BB_PORT_MIN" "$BB_PORT_MAX"); do
    [ -n "$port" ] || continue
    if [ -f "$dir/$port" ]; then
      owner="$(bb_meta_get "$dir/$port" worktree)"
      if [ "$owner" = "$root" ]; then
        printf '%s\n' "$port"; return 0   # already ours — reuse the claim
      fi
      opid="$(bb_meta_get "$dir/$port" pid)"
      if [ "$opid" = "RESERVED" ] || [ "$opid" = "PENDING" ] || bb_pid_alive "$opid"; then
        continue                          # held by another live worktree
      fi
      rm -f "$dir/$port"                   # stale entry — reclaim
    fi
    bb_port_in_use "$port" && continue
    if ( set -o noclobber
         printf 'port=%s\npid=%s\nworktree=%s\nbranch=%s\nstarted=%s\nsession=%s\n' \
           "$port" "$state" "$root" "$(bb_current_branch "$root")" "$(bb_now)" "$(bb_session_id)" \
           > "$dir/$port" ) 2>/dev/null; then
      printf '%s\n' "$port"; return 0
    fi
  done
  return 1
}

bb_meta_write() { # <port> <pid> <root> <log>
  local port="$1" pid="$2" root="$3" log="$4"
  printf 'port=%s\npid=%s\nworktree=%s\nbranch=%s\nstarted=%s\nlog=%s\nsession=%s\n' \
    "$port" "$pid" "$root" "$(bb_current_branch "$root")" "$(bb_now)" "$log" \
    "$(bb_session_id)" > "$(bb_servers_dir)/$port"
}

# Stop the server registered on <port> and clear its entry.
bb_stop_port() { # <port>
  local port="$1" f pid root i
  f="$(bb_servers_dir)/$port"
  pid=""; root=""
  if [ -f "$f" ]; then
    pid="$(bb_meta_get "$f" pid)"
    root="$(bb_meta_get "$f" worktree)"
  fi
  case "$pid" in
    ''|RESERVED|PENDING) : ;;
    *)
      # launch() execs into the server, so the recorded pid IS the listener —
      # killing it frees the port. `|| true` keeps a lost race (pid already
      # gone) from aborting cleanup under the caller's set -e.
      if bb_pid_alive "$pid"; then
        kill "$pid" 2>/dev/null || true
        for i in 1 2 3 4 5 6 7 8 9 10; do
          bb_pid_alive "$pid" || break
          sleep 0.3
        done
        bb_pid_alive "$pid" && { kill -9 "$pid" 2>/dev/null || true; }
      fi
      ;;
  esac
  rm -f "$f"
  if [ -n "$root" ] && [ -f "$root/.breadbox-port" ]; then
    [ "$(head -1 "$root/.breadbox-port" 2>/dev/null | tr -dc '0-9')" = "$port" ] \
      && rm -f "$root/.breadbox-port"
  fi
}

# Stop the managed server for a worktree root (if any).
bb_stop_worktree() { # <root>
  local root="$1" port
  port="$(bb_server_for_worktree "$root" || true)"
  [ -n "$port" ] || return 0
  bb_log "==> stopping dev server on :$port ($root)"
  bb_stop_port "$port"
}

# Reap orphans: forget dead servers, kill servers whose worktree was removed,
# and drop stale RESERVED/PENDING claims for worktrees that no longer exist.
bb_reap() {
  local dir f pid wt killed
  dir="$(bb_servers_dir)"
  [ -d "$dir" ] || return 0
  killed=0
  for f in "$dir"/*; do
    [ -f "$f" ] || continue
    pid="$(bb_meta_get "$f" pid)"
    wt="$(bb_meta_get "$f" worktree)"
    case "$pid" in
      RESERVED)
        # A session reservation survives while its worktree exists.
        if [ -n "$wt" ] && [ ! -d "$wt" ]; then rm -f "$f"; fi
        ;;
      PENDING)
        # A boot-in-progress claim should be short-lived. Drop it if the
        # worktree is gone, or if it's older than 10 min (a crashed/killed
        # boot that never reached bb_meta_write) — otherwise it would hold the
        # port forever with no time-based recovery.
        if { [ -n "$wt" ] && [ ! -d "$wt" ]; } \
           || [ "$(( $(bb_now) - $(bb_mtime "$f") ))" -gt 600 ]; then
          rm -f "$f"
        fi
        ;;
      ''|*[!0-9]*)
        rm -f "$f"            # malformed
        ;;
      *)
        if ! bb_pid_alive "$pid"; then
          rm -f "$f"          # process gone — forget it
        elif [ -n "$wt" ] && [ ! -d "$wt" ]; then
          kill "$pid" 2>/dev/null && killed=$((killed + 1))
          rm -f "$f"          # worktree removed but server lingered — reap it
        fi
        ;;
    esac
  done
  [ "$killed" -gt 0 ] && bb_log "==> reaped $killed orphaned dev server(s)"
  return 0
}

# Blunt instrument: stop EVERY process listening on the dev range (including
# other worktrees') and clear the registry. SIGTERM first, wait, then SIGKILL
# survivors — a process that ignores TERM (e.g. an `air` supervisor) would
# otherwise live on untracked. Only registry entries whose process is actually
# gone are removed, so a true survivor stays findable via dev-ps/dev-reap.
bb_stop_all() { # [lo] [hi]
  local lo="${1:-8080}" hi="${2:-8099}" pids pid i dir f epid
  pids="$(lsof -ti:"$lo"-"$hi" 2>/dev/null | sort -u || true)"
  if [ -n "$pids" ]; then
    for pid in $pids; do kill "$pid" 2>/dev/null || true; done
    for i in 1 2 3 4 5 6 7 8 9 10; do
      pids="$(lsof -ti:"$lo"-"$hi" 2>/dev/null | sort -u || true)"
      [ -n "$pids" ] || break
      sleep 0.3
    done
    pids="$(lsof -ti:"$lo"-"$hi" 2>/dev/null | sort -u || true)"
    for pid in $pids; do kill -9 "$pid" 2>/dev/null || true; done
    bb_log "==> stopped dev instances on $lo-$hi"
  else
    bb_log "No dev instances running on $lo-$hi."
  fi
  dir="$(bb_servers_dir)"
  [ -d "$dir" ] || return 0
  for f in "$dir"/*; do
    [ -f "$f" ] || continue
    epid="$(bb_meta_get "$f" pid)"
    case "$epid" in
      ''|RESERVED|PENDING) rm -f "$f" ;;          # no live process behind these
      *) bb_pid_alive "$epid" || rm -f "$f" ;;    # keep tracking a survivor
    esac
  done
  return 0
}
