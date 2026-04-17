-- +goose Up

-- Rename the `on_update` trigger to `on_change`. The CHECK constraint is
-- widened to accept both values for a transition window; existing rows
-- are flipped to the new name. The service layer normalizes either input
-- to `on_change` on write, and the sync resolver treats both identically,
-- so an accidentally-pinned `on_update` value continues to function.
--
-- A later migration can tighten the CHECK to `on_change`-only once the
-- renaming window closes; no schema lock is needed in the meantime.

ALTER TABLE transaction_rules DROP CONSTRAINT IF EXISTS transaction_rules_trigger_check;
ALTER TABLE transaction_rules
    ADD CONSTRAINT transaction_rules_trigger_check
    CHECK (trigger IN ('on_create', 'on_update', 'on_change', 'always'));

UPDATE transaction_rules SET trigger = 'on_change' WHERE trigger = 'on_update';

-- +goose Down

-- Revert rows and tighten the constraint back to the original set.
UPDATE transaction_rules SET trigger = 'on_update' WHERE trigger = 'on_change';

ALTER TABLE transaction_rules DROP CONSTRAINT IF EXISTS transaction_rules_trigger_check;
ALTER TABLE transaction_rules
    ADD CONSTRAINT transaction_rules_trigger_check
    CHECK (trigger IN ('on_create', 'on_update', 'always'));
