-- +goose Up
-- category_override becomes a source-aware enum: who set the current category.
--   'none'  = uncategorized, or rule-set (lowest priority; agents & users may overwrite)
--   'agent' = set by an agent (beats rules; only a user overwrites)  [written starting in PR 2b]
--   'user'  = a human deliberately set it. SACRED — nothing auto-overwrites it.
-- Precedence: user > agent > rule. Pre-release, so this is a direct breaking
-- retype (boolean -> TEXT). Existing TRUE rows were human-set -> 'user'; FALSE -> 'none'.
ALTER TABLE transactions ALTER COLUMN category_override DROP DEFAULT;
ALTER TABLE transactions
    ALTER COLUMN category_override TYPE TEXT
    USING (CASE WHEN category_override THEN 'user' ELSE 'none' END);
ALTER TABLE transactions ALTER COLUMN category_override SET DEFAULT 'none';
ALTER TABLE transactions
    ADD CONSTRAINT transactions_category_override_check
    CHECK (category_override IN ('none', 'agent', 'user'));

-- +goose Down
ALTER TABLE transactions DROP CONSTRAINT transactions_category_override_check;
ALTER TABLE transactions ALTER COLUMN category_override DROP DEFAULT;
ALTER TABLE transactions
    ALTER COLUMN category_override TYPE BOOLEAN
    USING (CASE WHEN category_override <> 'none' THEN TRUE ELSE FALSE END);
ALTER TABLE transactions ALTER COLUMN category_override SET DEFAULT FALSE;
