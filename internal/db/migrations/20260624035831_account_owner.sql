-- +goose Up
-- Per-account owner override. Lets an account be attributed to a household
-- member independent of its bank connection's owner — needed when one
-- connection (e.g. a SimpleFIN bridge) spans accounts belonging to different
-- people. Nullable: NULL means "inherit the connection owner". Attribution is
-- resolved at read time via COALESCE(t.attributed_user_id, a.owner_user_id,
-- bc.user_id), so reassigning re-routes existing transactions with no backfill.
ALTER TABLE accounts ADD COLUMN owner_user_id UUID NULL REFERENCES users(id) ON DELETE SET NULL;
CREATE INDEX accounts_owner_user_idx ON accounts(owner_user_id) WHERE owner_user_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS accounts_owner_user_idx;
ALTER TABLE accounts DROP COLUMN IF EXISTS owner_user_id;
