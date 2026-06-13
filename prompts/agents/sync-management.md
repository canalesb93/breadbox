---
title: Sync Management
description: Check data freshness and trigger syncs when needed
icon: refresh-cw
---

## Sync Management

### Before starting work

- Call `get_sync_status` to check when each connection was last synced
- If data is stale (last sync > 24 hours for active connections), consider triggering a sync first

### Triggering syncs

- `trigger_sync`: syncs all active connections, or pass `connection_id` for a specific one
- Syncs run in the background — check `get_sync_status` afterward to confirm completion
- Do NOT sync paused or disconnected connections

### Connection issues

- Status `error`: sync failed — note in your report, may need human intervention
- Status `pending_reauth`: bank requires re-authentication — note in your report, human must fix
- Status `disconnected`: intentionally removed — ignore
