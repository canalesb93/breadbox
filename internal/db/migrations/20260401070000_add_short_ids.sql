-- +goose Up

-- PL/pgSQL function to generate random 8-char base62 strings.
-- Used as a trigger to auto-populate short_id on INSERT.
CREATE OR REPLACE FUNCTION generate_short_id() RETURNS TEXT AS $$
DECLARE
    chars TEXT := '0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz';
    result TEXT := '';
    i INT;
    bytes BYTEA;
BEGIN
    bytes := gen_random_bytes(8);
    FOR i IN 0..7 LOOP
        result := result || substr(chars, (get_byte(bytes, i) % 62) + 1, 1);
    END LOOP;
    RETURN result;
END;
$$ LANGUAGE plpgsql;

-- Trigger function: sets short_id on INSERT if not already provided.
CREATE OR REPLACE FUNCTION set_short_id() RETURNS TRIGGER AS $$
DECLARE
    new_id TEXT;
    attempts INT := 0;
BEGIN
    IF NEW.short_id IS NOT NULL AND NEW.short_id != '' THEN
        RETURN NEW;
    END IF;
    LOOP
        new_id := generate_short_id();
        -- Check uniqueness within this table using dynamic SQL
        PERFORM 1 FROM pg_catalog.pg_class WHERE FALSE; -- placeholder; uniqueness enforced by UNIQUE index
        NEW.short_id := new_id;
        RETURN NEW;
    END LOOP;
END;
$$ LANGUAGE plpgsql;

-- Add short_id columns and triggers to all entity tables.
-- Column is nullable initially for the backfill step.

ALTER TABLE transactions ADD COLUMN short_id TEXT;
ALTER TABLE accounts ADD COLUMN short_id TEXT;
ALTER TABLE users ADD COLUMN short_id TEXT;
ALTER TABLE categories ADD COLUMN short_id TEXT;
ALTER TABLE bank_connections ADD COLUMN short_id TEXT;
ALTER TABLE review_queue ADD COLUMN short_id TEXT;
ALTER TABLE transaction_rules ADD COLUMN short_id TEXT;
ALTER TABLE account_links ADD COLUMN short_id TEXT;
ALTER TABLE transaction_matches ADD COLUMN short_id TEXT;
ALTER TABLE transaction_comments ADD COLUMN short_id TEXT;
ALTER TABLE agent_reports ADD COLUMN short_id TEXT;
ALTER TABLE sync_logs ADD COLUMN short_id TEXT;

-- Backfill existing rows.
UPDATE transactions SET short_id = generate_short_id() WHERE short_id IS NULL;
UPDATE accounts SET short_id = generate_short_id() WHERE short_id IS NULL;
UPDATE users SET short_id = generate_short_id() WHERE short_id IS NULL;
UPDATE categories SET short_id = generate_short_id() WHERE short_id IS NULL;
UPDATE bank_connections SET short_id = generate_short_id() WHERE short_id IS NULL;
UPDATE review_queue SET short_id = generate_short_id() WHERE short_id IS NULL;
UPDATE transaction_rules SET short_id = generate_short_id() WHERE short_id IS NULL;
UPDATE account_links SET short_id = generate_short_id() WHERE short_id IS NULL;
UPDATE transaction_matches SET short_id = generate_short_id() WHERE short_id IS NULL;
UPDATE transaction_comments SET short_id = generate_short_id() WHERE short_id IS NULL;
UPDATE agent_reports SET short_id = generate_short_id() WHERE short_id IS NULL;
UPDATE sync_logs SET short_id = generate_short_id() WHERE short_id IS NULL;

-- Make NOT NULL and add unique indexes.
ALTER TABLE transactions ALTER COLUMN short_id SET NOT NULL;
ALTER TABLE accounts ALTER COLUMN short_id SET NOT NULL;
ALTER TABLE users ALTER COLUMN short_id SET NOT NULL;
ALTER TABLE categories ALTER COLUMN short_id SET NOT NULL;
ALTER TABLE bank_connections ALTER COLUMN short_id SET NOT NULL;
ALTER TABLE review_queue ALTER COLUMN short_id SET NOT NULL;
ALTER TABLE transaction_rules ALTER COLUMN short_id SET NOT NULL;
ALTER TABLE account_links ALTER COLUMN short_id SET NOT NULL;
ALTER TABLE transaction_matches ALTER COLUMN short_id SET NOT NULL;
ALTER TABLE transaction_comments ALTER COLUMN short_id SET NOT NULL;
ALTER TABLE agent_reports ALTER COLUMN short_id SET NOT NULL;
ALTER TABLE sync_logs ALTER COLUMN short_id SET NOT NULL;

CREATE UNIQUE INDEX transactions_short_id_idx ON transactions (short_id);
CREATE UNIQUE INDEX accounts_short_id_idx ON accounts (short_id);
CREATE UNIQUE INDEX users_short_id_idx ON users (short_id);
CREATE UNIQUE INDEX categories_short_id_idx ON categories (short_id);
CREATE UNIQUE INDEX bank_connections_short_id_idx ON bank_connections (short_id);
CREATE UNIQUE INDEX review_queue_short_id_idx ON review_queue (short_id);
CREATE UNIQUE INDEX transaction_rules_short_id_idx ON transaction_rules (short_id);
CREATE UNIQUE INDEX account_links_short_id_idx ON account_links (short_id);
CREATE UNIQUE INDEX transaction_matches_short_id_idx ON transaction_matches (short_id);
CREATE UNIQUE INDEX transaction_comments_short_id_idx ON transaction_comments (short_id);
CREATE UNIQUE INDEX agent_reports_short_id_idx ON agent_reports (short_id);
CREATE UNIQUE INDEX sync_logs_short_id_idx ON sync_logs (short_id);

-- Install triggers for auto-generating short_id on new rows.
CREATE TRIGGER set_short_id_transactions BEFORE INSERT ON transactions FOR EACH ROW EXECUTE FUNCTION set_short_id();
CREATE TRIGGER set_short_id_accounts BEFORE INSERT ON accounts FOR EACH ROW EXECUTE FUNCTION set_short_id();
CREATE TRIGGER set_short_id_users BEFORE INSERT ON users FOR EACH ROW EXECUTE FUNCTION set_short_id();
CREATE TRIGGER set_short_id_categories BEFORE INSERT ON categories FOR EACH ROW EXECUTE FUNCTION set_short_id();
CREATE TRIGGER set_short_id_bank_connections BEFORE INSERT ON bank_connections FOR EACH ROW EXECUTE FUNCTION set_short_id();
CREATE TRIGGER set_short_id_review_queue BEFORE INSERT ON review_queue FOR EACH ROW EXECUTE FUNCTION set_short_id();
CREATE TRIGGER set_short_id_transaction_rules BEFORE INSERT ON transaction_rules FOR EACH ROW EXECUTE FUNCTION set_short_id();
CREATE TRIGGER set_short_id_account_links BEFORE INSERT ON account_links FOR EACH ROW EXECUTE FUNCTION set_short_id();
CREATE TRIGGER set_short_id_transaction_matches BEFORE INSERT ON transaction_matches FOR EACH ROW EXECUTE FUNCTION set_short_id();
CREATE TRIGGER set_short_id_transaction_comments BEFORE INSERT ON transaction_comments FOR EACH ROW EXECUTE FUNCTION set_short_id();
CREATE TRIGGER set_short_id_agent_reports BEFORE INSERT ON agent_reports FOR EACH ROW EXECUTE FUNCTION set_short_id();
CREATE TRIGGER set_short_id_sync_logs BEFORE INSERT ON sync_logs FOR EACH ROW EXECUTE FUNCTION set_short_id();

-- +goose Down
DROP TRIGGER IF EXISTS set_short_id_transactions ON transactions;
DROP TRIGGER IF EXISTS set_short_id_accounts ON accounts;
DROP TRIGGER IF EXISTS set_short_id_users ON users;
DROP TRIGGER IF EXISTS set_short_id_categories ON categories;
DROP TRIGGER IF EXISTS set_short_id_bank_connections ON bank_connections;
DROP TRIGGER IF EXISTS set_short_id_review_queue ON review_queue;
DROP TRIGGER IF EXISTS set_short_id_transaction_rules ON transaction_rules;
DROP TRIGGER IF EXISTS set_short_id_account_links ON account_links;
DROP TRIGGER IF EXISTS set_short_id_transaction_matches ON transaction_matches;
DROP TRIGGER IF EXISTS set_short_id_transaction_comments ON transaction_comments;
DROP TRIGGER IF EXISTS set_short_id_agent_reports ON agent_reports;
DROP TRIGGER IF EXISTS set_short_id_sync_logs ON sync_logs;

DROP INDEX IF EXISTS transactions_short_id_idx;
DROP INDEX IF EXISTS accounts_short_id_idx;
DROP INDEX IF EXISTS users_short_id_idx;
DROP INDEX IF EXISTS categories_short_id_idx;
DROP INDEX IF EXISTS bank_connections_short_id_idx;
DROP INDEX IF EXISTS review_queue_short_id_idx;
DROP INDEX IF EXISTS transaction_rules_short_id_idx;
DROP INDEX IF EXISTS account_links_short_id_idx;
DROP INDEX IF EXISTS transaction_matches_short_id_idx;
DROP INDEX IF EXISTS transaction_comments_short_id_idx;
DROP INDEX IF EXISTS agent_reports_short_id_idx;
DROP INDEX IF EXISTS sync_logs_short_id_idx;

ALTER TABLE transactions DROP COLUMN IF EXISTS short_id;
ALTER TABLE accounts DROP COLUMN IF EXISTS short_id;
ALTER TABLE users DROP COLUMN IF EXISTS short_id;
ALTER TABLE categories DROP COLUMN IF EXISTS short_id;
ALTER TABLE bank_connections DROP COLUMN IF EXISTS short_id;
ALTER TABLE review_queue DROP COLUMN IF EXISTS short_id;
ALTER TABLE transaction_rules DROP COLUMN IF EXISTS short_id;
ALTER TABLE account_links DROP COLUMN IF EXISTS short_id;
ALTER TABLE transaction_matches DROP COLUMN IF EXISTS short_id;
ALTER TABLE transaction_comments DROP COLUMN IF EXISTS short_id;
ALTER TABLE agent_reports DROP COLUMN IF EXISTS short_id;
ALTER TABLE sync_logs DROP COLUMN IF EXISTS short_id;

DROP FUNCTION IF EXISTS set_short_id();
DROP FUNCTION IF EXISTS generate_short_id();
