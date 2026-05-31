-- +goose Up
-- source_template records which code-defined Workflow preset a definition was
-- instantiated from (NULL = hand-authored). The preset gallery uses it to show
-- "enabled vs available" and to prevent enabling the same preset twice. Presets
-- are NOT seeded rows — a row only exists once the household enables one.
ALTER TABLE agent_definitions ADD COLUMN source_template TEXT NULL;

-- +goose Down
ALTER TABLE agent_definitions DROP COLUMN IF EXISTS source_template;
