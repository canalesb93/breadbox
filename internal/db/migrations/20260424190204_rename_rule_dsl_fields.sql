-- +goose Up
-- Rewrite transaction_rules.conditions to use the new provider_* DSL field
-- names that mirror the column rename in the prior migration. Walks the
-- condition tree recursively so nested and/or/not compounds are covered.

-- +goose StatementBegin
CREATE FUNCTION rename_rule_dsl_fields(cond JSONB) RETURNS JSONB AS $$
DECLARE
    renames JSONB := '{
        "name": "provider_name",
        "merchant_name": "provider_merchant_name",
        "category_primary": "provider_category_primary",
        "category_detailed": "provider_category_detailed"
    }'::JSONB;
    result  JSONB;
    key     TEXT;
BEGIN
    IF cond IS NULL OR jsonb_typeof(cond) <> 'object' THEN
        RETURN cond;
    END IF;

    result := cond;

    IF result ? 'and' AND jsonb_typeof(result->'and') = 'array' THEN
        result := jsonb_set(result, '{and}',
            (SELECT COALESCE(jsonb_agg(rename_rule_dsl_fields(e)), '[]'::jsonb)
             FROM jsonb_array_elements(result->'and') e));
    END IF;

    IF result ? 'or' AND jsonb_typeof(result->'or') = 'array' THEN
        result := jsonb_set(result, '{or}',
            (SELECT COALESCE(jsonb_agg(rename_rule_dsl_fields(e)), '[]'::jsonb)
             FROM jsonb_array_elements(result->'or') e));
    END IF;

    IF result ? 'not' THEN
        result := jsonb_set(result, '{not}', rename_rule_dsl_fields(result->'not'));
    END IF;

    IF result ? 'field' AND jsonb_typeof(result->'field') = 'string' THEN
        key := result->>'field';
        IF renames ? key THEN
            result := jsonb_set(result, '{field}', to_jsonb(renames->>key));
        END IF;
    END IF;

    RETURN result;
END;
$$ LANGUAGE plpgsql IMMUTABLE;
-- +goose StatementEnd

UPDATE transaction_rules
   SET conditions = rename_rule_dsl_fields(conditions)
 WHERE conditions IS NOT NULL;

DROP FUNCTION rename_rule_dsl_fields(JSONB);

-- +goose Down
-- +goose StatementBegin
CREATE FUNCTION rename_rule_dsl_fields_down(cond JSONB) RETURNS JSONB AS $$
DECLARE
    renames JSONB := '{
        "provider_name": "name",
        "provider_merchant_name": "merchant_name",
        "provider_category_primary": "category_primary",
        "provider_category_detailed": "category_detailed"
    }'::JSONB;
    result  JSONB;
    key     TEXT;
BEGIN
    IF cond IS NULL OR jsonb_typeof(cond) <> 'object' THEN
        RETURN cond;
    END IF;

    result := cond;

    IF result ? 'and' AND jsonb_typeof(result->'and') = 'array' THEN
        result := jsonb_set(result, '{and}',
            (SELECT COALESCE(jsonb_agg(rename_rule_dsl_fields_down(e)), '[]'::jsonb)
             FROM jsonb_array_elements(result->'and') e));
    END IF;

    IF result ? 'or' AND jsonb_typeof(result->'or') = 'array' THEN
        result := jsonb_set(result, '{or}',
            (SELECT COALESCE(jsonb_agg(rename_rule_dsl_fields_down(e)), '[]'::jsonb)
             FROM jsonb_array_elements(result->'or') e));
    END IF;

    IF result ? 'not' THEN
        result := jsonb_set(result, '{not}', rename_rule_dsl_fields_down(result->'not'));
    END IF;

    IF result ? 'field' AND jsonb_typeof(result->'field') = 'string' THEN
        key := result->>'field';
        IF renames ? key THEN
            result := jsonb_set(result, '{field}', to_jsonb(renames->>key));
        END IF;
    END IF;

    RETURN result;
END;
$$ LANGUAGE plpgsql IMMUTABLE;
-- +goose StatementEnd

UPDATE transaction_rules
   SET conditions = rename_rule_dsl_fields_down(conditions)
 WHERE conditions IS NOT NULL;

DROP FUNCTION rename_rule_dsl_fields_down(JSONB);
