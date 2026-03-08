-- +goose Up
-- Fix Teller transactions that used incorrect SHOPPING category strings.
-- Plaid's taxonomy uses GENERAL_MERCHANDISE, not SHOPPING.
UPDATE transactions t
SET category_primary = 'GENERAL_MERCHANDISE',
    category_detailed = CASE category_detailed
        WHEN 'SHOPPING_CLOTHING_AND_ACCESSORIES' THEN 'GENERAL_MERCHANDISE_CLOTHING_AND_ACCESSORIES'
        WHEN 'SHOPPING_ELECTRONICS' THEN 'GENERAL_MERCHANDISE_ELECTRONICS'
        WHEN 'SHOPPING_OTHER_SHOPPING' THEN 'GENERAL_MERCHANDISE_OTHER_GENERAL_MERCHANDISE'
    END
FROM accounts a
JOIN bank_connections c ON a.connection_id = c.id
WHERE t.account_id = a.id
  AND c.provider = 'teller'
  AND t.category_primary = 'SHOPPING'
  AND t.deleted_at IS NULL;

-- +goose Down
UPDATE transactions t
SET category_primary = 'SHOPPING',
    category_detailed = CASE category_detailed
        WHEN 'GENERAL_MERCHANDISE_CLOTHING_AND_ACCESSORIES' THEN 'SHOPPING_CLOTHING_AND_ACCESSORIES'
        WHEN 'GENERAL_MERCHANDISE_ELECTRONICS' THEN 'SHOPPING_ELECTRONICS'
        WHEN 'GENERAL_MERCHANDISE_OTHER_GENERAL_MERCHANDISE' THEN 'SHOPPING_OTHER_SHOPPING'
    END
FROM accounts a
JOIN bank_connections c ON a.connection_id = c.id
WHERE t.account_id = a.id
  AND c.provider = 'teller'
  AND t.category_primary = 'GENERAL_MERCHANDISE'
  AND t.category_detailed IN (
    'GENERAL_MERCHANDISE_CLOTHING_AND_ACCESSORIES',
    'GENERAL_MERCHANDISE_ELECTRONICS',
    'GENERAL_MERCHANDISE_OTHER_GENERAL_MERCHANDISE'
  )
  AND t.deleted_at IS NULL;
