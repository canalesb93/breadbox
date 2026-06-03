#!/bin/bash
# SessionEnd hook — stop the managed dev server this session's worktree started,
# and reap any orphaned servers left by previously-closed sessions.
#
# This is the cleanup that was missing: nothing used to stop a `breadbox serve`
# when a session closed, so stale instances accumulated across 8081-8099. We
# only stop the CURRENT worktree's managed server (tracked in the dev-server
# registry) — sibling worktrees' servers are left alone.
#
# Skipped on reason=clear (a /clear keeps the session alive; killing the dev
# server then would be hostile). Reap still runs (cheap, only touches dead
# entries).

PROJECT_DIR="${CLAUDE_PROJECT_DIR:-$(pwd)}"

# Hook input arrives as JSON on stdin (cwd, reason, session_id). Parse without a
# hard python3 dependency (stock macOS often lacks it): try python3, then fall
# back to a sed string-field extractor.
INPUT="$(cat 2>/dev/null || true)"
bb_json_field() { # <key> — reads $INPUT
  local key="$1" val=""
  if [ -n "$INPUT" ] && command -v python3 >/dev/null 2>&1; then
    val="$(printf '%s' "$INPUT" | python3 -c "import sys,json
try: print(json.load(sys.stdin).get('$key',''))
except Exception: pass" 2>/dev/null || true)"
  fi
  if [ -z "$val" ] && [ -n "$INPUT" ]; then
    val="$(printf '%s' "$INPUT" | sed -n "s/.*\"$key\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p" | head -1)"
  fi
  printf '%s' "$val"
}
CWD="$(bb_json_field cwd)"
REASON="$(bb_json_field reason)"
[ -n "$CWD" ] || CWD="$PWD"
# Registry entries are tagged with the ENV session id (bb_session_id). Read the
# session from the SAME source here so a same-session comparison is identical by
# construction; only fall back to the stdin JSON id if the env var is absent.
# (Using a different source than the writer could make the owner check think a
# sibling owns the server and skip cleanup — a leak. Same source avoids that.)
SESSION="${CLAUDE_CODE_SESSION_ID:-${CLAUDE_SESSION_ID:-}}"
[ -n "$SESSION" ] || SESSION="$(bb_json_field session_id)"

# Locate the lib relative to the active worktree if the project-dir copy is absent.
LIB="$PROJECT_DIR/scripts/dev-lib.sh"
if [ ! -f "$LIB" ] && [ -f "$CWD/scripts/dev-lib.sh" ]; then
  LIB="$CWD/scripts/dev-lib.sh"
fi
[ -f "$LIB" ] || exit 0   # nothing we can do without the lib

# shellcheck source=scripts/dev-lib.sh
. "$LIB"

if [ "$REASON" != "clear" ]; then
  ROOT="$(bb_worktree_root "$CWD")"
  PORT_OWNED="$(bb_server_for_worktree "$ROOT" || true)"
  if [ -n "$PORT_OWNED" ]; then
    # If the registry entry records a different session than the one ending,
    # another live session shares this worktree — leave its server running.
    ENTRY_SESSION="$(bb_meta_get "$(bb_servers_dir)/$PORT_OWNED" session 2>/dev/null || true)"
    if [ -n "$ENTRY_SESSION" ] && [ -n "$SESSION" ] && [ "$ENTRY_SESSION" != "$SESSION" ]; then
      :   # owned by another session — don't kill it
    else
      bb_stop_worktree "$ROOT" || true
    fi
  fi
fi
bb_reap || true
exit 0
