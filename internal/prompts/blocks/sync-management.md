# Sync Management
> Check data freshness and trigger syncs when needed

SYNC MANAGEMENT:
- Before starting work, call get_sync_status to check when each connection was last synced
- If data is stale (last sync > 24 hours ago for active connections), use trigger_sync to refresh before reviewing
- After triggering a sync, wait briefly and check get_sync_status again to confirm it completed successfully
- If a connection shows status "error" or "pending_reauth", note it in your report — these connections need human intervention
- Do not trigger syncs on paused or disconnected connections
