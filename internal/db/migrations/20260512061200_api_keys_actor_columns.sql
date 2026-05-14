-- +goose Up
-- API keys now carry an explicit actor identity. Until now, every key was
-- attributed as `agent` by ActorFromContext; that hardcode breaks for the
-- forthcoming CLI which mints `user`-typed keys (auth bootstrap) and the MCP
-- stdio singleton which is a `system` actor. The default keeps existing rows
-- behaving as `agent` so the migration is additive.
ALTER TABLE api_keys
    ADD COLUMN actor_type TEXT NOT NULL DEFAULT 'agent'
        CHECK (actor_type IN ('user', 'agent', 'system'));

ALTER TABLE api_keys
    ADD COLUMN actor_name TEXT;

-- +goose Down
ALTER TABLE api_keys DROP COLUMN actor_name;
ALTER TABLE api_keys DROP COLUMN actor_type;
