# Sync Management
> Check data freshness and trigger syncs when needed

BEFORE STARTING WORK:
- Call get_sync_status to check when each connection was last synced
- If data is stale (last sync > 24 hours for active connections), consider triggering a sync first

TRIGGERING SYNCS:
- trigger_sync: syncs all active connections, or pass connection_id for a specific one
- Syncs run in the background — check get_sync_status afterward to confirm completion
- Do NOT sync paused or disconnected connections

CONNECTION ISSUES:
- Status "error": sync failed — note in your report, may need human intervention
- Status "pending_reauth": bank requires re-authentication — note in your report, human must fix
- Status "disconnected": intentionally removed — ignore
