-- +goose Up
CREATE TABLE transaction_rules (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT          NOT NULL,
    conditions      JSONB         NOT NULL,
    category_id     UUID          NULL REFERENCES categories(id) ON DELETE CASCADE,
    priority        INTEGER       NOT NULL DEFAULT 0,
    enabled         BOOLEAN       NOT NULL DEFAULT TRUE,
    expires_at      TIMESTAMPTZ   NULL,
    created_by_type TEXT          NOT NULL DEFAULT 'user' CHECK (created_by_type IN ('user', 'agent', 'system')),
    created_by_id   TEXT          NULL,
    created_by_name TEXT          NOT NULL DEFAULT 'Breadbox',
    hit_count       INTEGER       NOT NULL DEFAULT 0,
    last_hit_at     TIMESTAMPTZ   NULL,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE INDEX transaction_rules_enabled_priority_idx ON transaction_rules(priority DESC) WHERE enabled = TRUE;
CREATE INDEX transaction_rules_category_id_idx ON transaction_rules(category_id);
CREATE INDEX transaction_rules_expires_at_idx ON transaction_rules(expires_at) WHERE expires_at IS NOT NULL;

-- +goose Down
DROP TABLE IF EXISTS transaction_rules;
