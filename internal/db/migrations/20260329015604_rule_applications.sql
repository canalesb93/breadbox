-- +goose Up
CREATE TABLE transaction_rule_applications (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    transaction_id  UUID          NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
    rule_id         UUID          NOT NULL REFERENCES transaction_rules(id) ON DELETE CASCADE,
    action_field    TEXT          NOT NULL,
    action_value    TEXT          NOT NULL,
    applied_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    applied_by      TEXT          NOT NULL DEFAULT 'sync'
);

CREATE UNIQUE INDEX tra_txn_rule_field_idx ON transaction_rule_applications(transaction_id, rule_id, action_field);
CREATE INDEX tra_rule_id_idx ON transaction_rule_applications(rule_id);
CREATE INDEX tra_transaction_id_idx ON transaction_rule_applications(transaction_id);

COMMENT ON TABLE transaction_rule_applications IS 'Junction table tracking which rules applied which actions to which transactions';
COMMENT ON COLUMN transaction_rule_applications.action_field IS 'Which action field was set (e.g., category)';
COMMENT ON COLUMN transaction_rule_applications.action_value IS 'The value that was set (e.g., food_and_drink_coffee)';
COMMENT ON COLUMN transaction_rule_applications.applied_by IS 'How the rule was applied: sync, retroactive, manual';

-- +goose Down
DROP TABLE IF EXISTS transaction_rule_applications;
