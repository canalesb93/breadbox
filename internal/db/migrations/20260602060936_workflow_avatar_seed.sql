-- +goose Up
-- Per-workflow DiceBear avatar seed. NULL falls back to the workflow's slug
-- (the historical seed), so existing rows keep their current avatar until an
-- operator changes it from the Workflows reconfigure drawer. Additive — safe
-- on the shared dev DB.
ALTER TABLE workflows ADD COLUMN avatar_seed TEXT;

-- +goose Down
ALTER TABLE workflows DROP COLUMN avatar_seed;
