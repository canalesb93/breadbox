---
description: Run (or keep) a local Breadbox on this branch's changes and surface a testing link.
---

Make sure a local Breadbox instance is running on top of this checkout's current changes, give me link(s) to test it, then keep surfacing the link as the session goes.

## Bring it up

1. **Port** — use `$PORT` if set (worktrees get one from the session hook), else `cat .breadbox-port` if it exists, else `8080`.
2. **Already running?** Check `lsof -ti:$PORT`. If something's listening, reuse it — `make dev-watch` rebuilds Go on save and serves HTML/CSS from disk, so it already reflects new changes. Don't start a second instance.
3. **Not running** — start it with `make dev-watch` using `run_in_background: true` (needs `DATABASE_URL`, which the session hook sets). Poll `curl -sf http://localhost:$PORT/health` until it returns before reporting.
4. If air's rebuild is failing, surface the build error instead of a link.

## Surface the link

Once it's healthy, give me:
- Base URL: `http://localhost:<port>`
- The page(s) that exercise this change (e.g. `http://localhost:<port>/transactions`).

**For the rest of this session:** end any response where changes are ready to test — and any recap — with the live link. If the server has died, say so instead of handing me a stale link.
