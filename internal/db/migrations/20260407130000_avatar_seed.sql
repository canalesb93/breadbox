-- +goose Up
ALTER TABLE users ADD COLUMN avatar_seed TEXT;

-- +goose Down
ALTER TABLE users DROP COLUMN IF EXISTS avatar_seed;
