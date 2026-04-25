-- +goose Up
-- Backfill annotations.actor_id for actor_type='user' rows where the recorded
-- ID is an auth_accounts.id rather than the linked users.id. Before #788 the
-- admin session built service.Actor from auth_accounts.id, so the avatar
-- handler at GET /avatars/{id} (which keys on users.id) missed for every
-- timeline entry written by an admin and fell through to the generated
-- pattern. ActorFromSession now prefers users.id; this migration repairs
-- existing rows so older timeline entries also resolve uploaded avatars.
UPDATE annotations a
SET actor_id = aa.user_id::text
FROM auth_accounts aa
WHERE a.actor_type = 'user'
  AND a.actor_id IS NOT NULL
  AND a.actor_id ~ '^[0-9a-fA-F-]{36}$'
  AND a.actor_id::uuid = aa.id
  AND aa.user_id IS NOT NULL;

-- +goose Down
-- One-way data repair. The original auth_accounts.id values are not preserved
-- per row, so reversal would require re-deriving them from session history,
-- which we do not retain. No-op on rollback.
SELECT 1;
