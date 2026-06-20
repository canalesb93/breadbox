-- +goose Up
-- P3 of the rules-as-universal-substrate sprint: drop category_override and its
-- precedence guard entirely. Provenance/precedence (user > agent > rule) is
-- DEFERRED — rules, agents, and users all just write category_id
-- (last-writer-wins). The sync engine only runs rules on isNew||isChanged
-- transactions, so a user's manual edit is not continuously re-clobbered; that
-- gate is good-enough stickiness without a provenance column. Annotations remain
-- the audit log; no logic keys off them.
--
-- Pre-release, sprint branch only, no backward-compat required.
ALTER TABLE transactions DROP CONSTRAINT IF EXISTS transactions_category_override_check;
ALTER TABLE transactions DROP COLUMN IF EXISTS category_override;

-- +goose Down
-- NON-REVERSIBLE: the per-row provenance (who set each category: none/agent/user)
-- is destroyed by the Up drop and cannot be reconstructed. This Down only
-- restores the column shape so goose can roll the migration version back; every
-- existing row resets to the 'none' default, losing all prior override state.
ALTER TABLE transactions
    ADD COLUMN IF NOT EXISTS category_override TEXT NOT NULL DEFAULT 'none';
ALTER TABLE transactions
    ADD CONSTRAINT transactions_category_override_check
    CHECK (category_override IN ('none', 'agent', 'user'));
