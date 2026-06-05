-- +goose NO TRANSACTION
-- +goose Up
-- Add 'simplefin' to the provider_type enum so SimpleFIN connections can be
-- stored alongside Plaid/Teller/CSV. Appending an enum value at the end is the
-- shared-DB-safe form (per .claude/rules/migrations.md): older `breadbox serve`
-- processes keep working since they never write the new value until they pick
-- up the SimpleFIN code.
--
-- ALTER TYPE ... ADD VALUE cannot run inside a transaction block (and the new
-- value isn't usable until commit), so this migration is marked NO TRANSACTION.
-- IF NOT EXISTS makes it idempotent across re-runs / shared dev DBs.
ALTER TYPE provider_type ADD VALUE IF NOT EXISTS 'simplefin';

-- +goose Down
-- PostgreSQL cannot remove a value from an enum type, so the down migration is
-- intentionally a no-op. Removing 'simplefin' would require recreating the type
-- and rewriting every dependent column — never safe on a shared DB.
SELECT 1;
