-- +goose Up
-- Operator-editable note attached to each agent_run. Used by humans to
-- record context around a manual fire ("triggered to test category X
-- changes") or annotate a cron run after the fact ("known false-positive
-- run; investigating"). Free-form TEXT; no constraints — UI caps length.
ALTER TABLE agent_runs
    ADD COLUMN operator_note TEXT NULL;

-- +goose Down
ALTER TABLE agent_runs
    DROP COLUMN operator_note;
