package seed

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Run inserts test data into the database. All inserts use ON CONFLICT DO NOTHING
// so the function is idempotent and safe to run multiple times.
func Run(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) error {
	logger.Info("seeding database...")

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. Users
	if _, err := tx.Exec(ctx, usersSQL); err != nil {
		return fmt.Errorf("seed users: %w", err)
	}
	logger.Info("seeded users")

	// 2. Bank connections
	if _, err := tx.Exec(ctx, bankConnectionsSQL); err != nil {
		return fmt.Errorf("seed bank connections: %w", err)
	}
	logger.Info("seeded bank connections")

	// 3. Accounts
	if _, err := tx.Exec(ctx, accountsSQL); err != nil {
		return fmt.Errorf("seed accounts: %w", err)
	}
	logger.Info("seeded accounts")

	// 4. Transactions
	if _, err := tx.Exec(ctx, transactionsSQL); err != nil {
		return fmt.Errorf("seed transactions: %w", err)
	}
	logger.Info("seeded transactions")

	// 5. Sync logs
	if _, err := tx.Exec(ctx, syncLogsSQL); err != nil {
		return fmt.Errorf("seed sync logs: %w", err)
	}
	logger.Info("seeded sync logs")

	// 6. Teller bank connection
	if _, err := tx.Exec(ctx, tellerConnectionSQL); err != nil {
		return fmt.Errorf("seed teller connection: %w", err)
	}
	logger.Info("seeded teller connection")

	// 7. Teller accounts
	if _, err := tx.Exec(ctx, tellerAccountsSQL); err != nil {
		return fmt.Errorf("seed teller accounts: %w", err)
	}
	logger.Info("seeded teller accounts")

	// 8. Teller transactions
	if _, err := tx.Exec(ctx, tellerTransactionsSQL); err != nil {
		return fmt.Errorf("seed teller transactions: %w", err)
	}
	logger.Info("seeded teller transactions")

	// 9. Teller sync log
	if _, err := tx.Exec(ctx, tellerSyncLogSQL); err != nil {
		return fmt.Errorf("seed teller sync log: %w", err)
	}
	logger.Info("seeded teller sync log")

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	logger.Info("database seeded successfully")
	return nil
}

const usersSQL = `
INSERT INTO users (id, name, email) VALUES
  ('00000000-0000-0000-0000-000000000001', 'Alice Johnson', 'alice@example.com'),
  ('00000000-0000-0000-0000-000000000002', 'Bob Smith', 'bob@example.com')
ON CONFLICT (id) DO NOTHING;
`

const bankConnectionsSQL = `
INSERT INTO bank_connections (id, user_id, provider, institution_id, institution_name, external_id, encrypted_credentials, status, sync_cursor) VALUES
  ('00000000-0000-0000-0000-000000000011', '00000000-0000-0000-0000-000000000001', 'plaid', 'ins_1', 'Chase', 'seed_item_1', 'seed_encrypted_token_1', 'active', 'seed_cursor_1'),
  ('00000000-0000-0000-0000-000000000012', '00000000-0000-0000-0000-000000000002', 'plaid', 'ins_2', 'Bank of America', 'seed_item_2', 'seed_encrypted_token_2', 'active', 'seed_cursor_2')
ON CONFLICT (id) DO NOTHING;
`

const accountsSQL = `
INSERT INTO accounts (id, connection_id, external_account_id, name, type, subtype, mask, balance_current, balance_available, balance_limit, iso_currency_code) VALUES
  ('00000000-0000-0000-0000-000000000021', '00000000-0000-0000-0000-000000000011', 'seed_acct_1', 'Chase Checking',    'depository', 'checking',    '4321', 5420.50, 5200.00, NULL,     'USD'),
  ('00000000-0000-0000-0000-000000000022', '00000000-0000-0000-0000-000000000011', 'seed_acct_2', 'Chase Credit Card',  'credit',     'credit card', '9876', 1250.75, NULL,    10000.00, 'USD'),
  ('00000000-0000-0000-0000-000000000023', '00000000-0000-0000-0000-000000000012', 'seed_acct_3', 'BofA Checking',      'depository', 'checking',    '1111', 3100.00, 3100.00, NULL,     'USD'),
  ('00000000-0000-0000-0000-000000000024', '00000000-0000-0000-0000-000000000012', 'seed_acct_4', 'BofA Savings',       'depository', 'savings',     '2222', 15000.00, 15000.00, NULL,   'USD')
ON CONFLICT (id) DO NOTHING;
`

const transactionsSQL = `
INSERT INTO transactions (account_id, external_transaction_id, amount, iso_currency_code, date, name, merchant_name, category_primary, category_detailed, payment_channel, pending) VALUES
  -- Alice / Chase Checking (seed_acct_1)
  ('00000000-0000-0000-0000-000000000021', 'seed_txn_001', 4.50,    'USD', CURRENT_DATE - INTERVAL '1 day',  'Starbucks Coffee',       'Starbucks',      'FOOD_AND_DRINK',  'COFFEE_SHOPS',     'in_store', false),
  ('00000000-0000-0000-0000-000000000021', 'seed_txn_002', 42.15,   'USD', CURRENT_DATE - INTERVAL '2 days', 'Shell Gas Station',      'Shell',          'TRANSPORTATION',  'GAS_STATIONS',     'in_store', false),
  ('00000000-0000-0000-0000-000000000021', 'seed_txn_003', 89.99,   'USD', CURRENT_DATE - INTERVAL '3 days', 'Amazon.com',             'Amazon',         'SHOPPING',        'GENERAL_MERCHANDISE', 'online', false),
  ('00000000-0000-0000-0000-000000000021', 'seed_txn_004', -3200.00,'USD', CURRENT_DATE - INTERVAL '5 days', 'Payroll Deposit',        NULL,             'INCOME',          'WAGES',            'other',    false),
  ('00000000-0000-0000-0000-000000000021', 'seed_txn_005', 1850.00, 'USD', CURRENT_DATE - INTERVAL '6 days', 'Rent Payment',           NULL,             'RENT_AND_UTILITIES', 'RENT',          'other',    false),
  ('00000000-0000-0000-0000-000000000021', 'seed_txn_006', 15.99,   'USD', CURRENT_DATE - INTERVAL '7 days', 'Netflix',                'Netflix',        'ENTERTAINMENT',   'STREAMING',        'online',   false),
  ('00000000-0000-0000-0000-000000000021', 'seed_txn_007', 127.43,  'USD', CURRENT_DATE - INTERVAL '8 days', 'Whole Foods Market',     'Whole Foods',    'FOOD_AND_DRINK',  'GROCERIES',        'in_store', false),
  ('00000000-0000-0000-0000-000000000021', 'seed_txn_008', 95.20,   'USD', CURRENT_DATE - INTERVAL '10 days','Con Edison',             'Con Edison',     'RENT_AND_UTILITIES', 'ELECTRIC',      'online',   false),
  ('00000000-0000-0000-0000-000000000021', 'seed_txn_009', 12.50,   'USD', CURRENT_DATE - INTERVAL '12 days','Uber Trip',              'Uber',           'TRANSPORTATION',  'RIDE_SHARE',       'online',   false),
  ('00000000-0000-0000-0000-000000000021', 'seed_txn_010', 35.00,   'USD', CURRENT_DATE - INTERVAL '14 days','Target',                 'Target',         'SHOPPING',        'GENERAL_MERCHANDISE', 'in_store', false),
  ('00000000-0000-0000-0000-000000000021', 'seed_txn_011', 8.75,    'USD', CURRENT_DATE - INTERVAL '16 days','Dunkin Donuts',          'Dunkin',         'FOOD_AND_DRINK',  'COFFEE_SHOPS',     'in_store', false),
  ('00000000-0000-0000-0000-000000000021', 'seed_txn_012', 200.00,  'USD', CURRENT_DATE - INTERVAL '20 days','ATM Withdrawal',         NULL,             'TRANSFER_OUT',    'ATM',              'in_store', false),
  ('00000000-0000-0000-0000-000000000021', 'seed_txn_013', 5.25,    'USD', CURRENT_DATE,                     'Starbucks Coffee',       'Starbucks',      'FOOD_AND_DRINK',  'COFFEE_SHOPS',     'in_store', true),

  -- Alice / Chase Credit Card (seed_acct_2)
  ('00000000-0000-0000-0000-000000000022', 'seed_txn_014', 65.00,   'USD', CURRENT_DATE - INTERVAL '1 day',  'Trader Joes',            'Trader Joes',    'FOOD_AND_DRINK',  'GROCERIES',        'in_store', false),
  ('00000000-0000-0000-0000-000000000022', 'seed_txn_015', 149.99,  'USD', CURRENT_DATE - INTERVAL '4 days', 'Best Buy',               'Best Buy',       'SHOPPING',        'ELECTRONICS',      'in_store', false),
  ('00000000-0000-0000-0000-000000000022', 'seed_txn_016', 22.00,   'USD', CURRENT_DATE - INTERVAL '5 days', 'Chipotle',               'Chipotle',       'FOOD_AND_DRINK',  'RESTAURANTS',      'in_store', false),
  ('00000000-0000-0000-0000-000000000022', 'seed_txn_017', 9.99,    'USD', CURRENT_DATE - INTERVAL '7 days', 'Spotify',                'Spotify',        'ENTERTAINMENT',   'STREAMING',        'online',   false),
  ('00000000-0000-0000-0000-000000000022', 'seed_txn_018', 45.30,   'USD', CURRENT_DATE - INTERVAL '9 days', 'CVS Pharmacy',           'CVS',            'SHOPPING',        'PHARMACIES',       'in_store', false),
  ('00000000-0000-0000-0000-000000000022', 'seed_txn_019', 180.00,  'USD', CURRENT_DATE - INTERVAL '11 days','Delta Airlines',         'Delta Airlines', 'TRAVEL',          'AIRLINES',         'online',   false),
  ('00000000-0000-0000-0000-000000000022', 'seed_txn_020', 33.50,   'USD', CURRENT_DATE - INTERVAL '15 days','Olive Garden',           'Olive Garden',   'FOOD_AND_DRINK',  'RESTAURANTS',      'in_store', false),
  ('00000000-0000-0000-0000-000000000022', 'seed_txn_021', 75.00,   'USD', CURRENT_DATE - INTERVAL '18 days','TJ Maxx',                'TJ Maxx',        'SHOPPING',        'CLOTHING',         'in_store', false),
  ('00000000-0000-0000-0000-000000000022', 'seed_txn_022', 14.99,   'USD', CURRENT_DATE - INTERVAL '22 days','Hulu',                   'Hulu',           'ENTERTAINMENT',   'STREAMING',        'online',   false),
  ('00000000-0000-0000-0000-000000000022', 'seed_txn_023', 55.00,   'USD', CURRENT_DATE - INTERVAL '25 days','Costco',                 'Costco',         'SHOPPING',        'GENERAL_MERCHANDISE', 'in_store', false),
  ('00000000-0000-0000-0000-000000000022', 'seed_txn_024', 28.50,   'USD', CURRENT_DATE,                     'DoorDash',               'DoorDash',       'FOOD_AND_DRINK',  'RESTAURANTS',      'online',   true),

  -- Bob / BofA Checking (seed_acct_3)
  ('00000000-0000-0000-0000-000000000023', 'seed_txn_025', 6.00,    'USD', CURRENT_DATE - INTERVAL '1 day',  'McDonalds',              'McDonalds',      'FOOD_AND_DRINK',  'FAST_FOOD',        'in_store', false),
  ('00000000-0000-0000-0000-000000000023', 'seed_txn_026', 55.00,   'USD', CURRENT_DATE - INTERVAL '3 days', 'Exxon Gas',              'Exxon',          'TRANSPORTATION',  'GAS_STATIONS',     'in_store', false),
  ('00000000-0000-0000-0000-000000000023', 'seed_txn_027', -2800.00,'USD', CURRENT_DATE - INTERVAL '5 days', 'Direct Deposit',         NULL,             'INCOME',          'WAGES',            'other',    false),
  ('00000000-0000-0000-0000-000000000023', 'seed_txn_028', 1500.00, 'USD', CURRENT_DATE - INTERVAL '6 days', 'Rent',                   NULL,             'RENT_AND_UTILITIES', 'RENT',          'other',    false),
  ('00000000-0000-0000-0000-000000000023', 'seed_txn_029', 112.35,  'USD', CURRENT_DATE - INTERVAL '8 days', 'Kroger',                 'Kroger',         'FOOD_AND_DRINK',  'GROCERIES',        'in_store', false),
  ('00000000-0000-0000-0000-000000000023', 'seed_txn_030', 60.00,   'USD', CURRENT_DATE - INTERVAL '10 days','National Grid',          'National Grid',  'RENT_AND_UTILITIES', 'GAS',           'online',   false),
  ('00000000-0000-0000-0000-000000000023', 'seed_txn_031', 18.50,   'USD', CURRENT_DATE - INTERVAL '12 days','Panera Bread',           'Panera',         'FOOD_AND_DRINK',  'RESTAURANTS',      'in_store', false),
  ('00000000-0000-0000-0000-000000000023', 'seed_txn_032', 25.00,   'USD', CURRENT_DATE - INTERVAL '15 days','Lyft Ride',              'Lyft',           'TRANSPORTATION',  'RIDE_SHARE',       'online',   false),
  ('00000000-0000-0000-0000-000000000023', 'seed_txn_033', 79.99,   'USD', CURRENT_DATE - INTERVAL '20 days','Home Depot',             'Home Depot',     'SHOPPING',        'HOME_IMPROVEMENT', 'in_store', false),
  ('00000000-0000-0000-0000-000000000023', 'seed_txn_034', 150.00,  'USD', CURRENT_DATE - INTERVAL '25 days','Internet Bill',          'Spectrum',       'RENT_AND_UTILITIES', 'INTERNET',      'online',   false),
  ('00000000-0000-0000-0000-000000000023', 'seed_txn_035', 42.00,   'USD', CURRENT_DATE - INTERVAL '30 days','Planet Fitness',         'Planet Fitness', 'ENTERTAINMENT',   'GYM',              'online',   false),
  ('00000000-0000-0000-0000-000000000023', 'seed_txn_036', 7.25,    'USD', CURRENT_DATE,                     'Subway',                 'Subway',         'FOOD_AND_DRINK',  'FAST_FOOD',        'in_store', true),

  -- Bob / BofA Savings (seed_acct_4)
  ('00000000-0000-0000-0000-000000000024', 'seed_txn_037', -500.00, 'USD', CURRENT_DATE - INTERVAL '5 days', 'Transfer from Checking', NULL,             'TRANSFER_IN',     'SAVINGS',          'other',    false),
  ('00000000-0000-0000-0000-000000000024', 'seed_txn_038', -500.00, 'USD', CURRENT_DATE - INTERVAL '35 days','Transfer from Checking', NULL,             'TRANSFER_IN',     'SAVINGS',          'other',    false),
  ('00000000-0000-0000-0000-000000000024', 'seed_txn_039', -500.00, 'USD', CURRENT_DATE - INTERVAL '65 days','Transfer from Checking', NULL,             'TRANSFER_IN',     'SAVINGS',          'other',    false),
  ('00000000-0000-0000-0000-000000000024', 'seed_txn_040', 0.12,    'USD', CURRENT_DATE - INTERVAL '30 days','Interest Payment',       NULL,             'INCOME',          'INTEREST',         'other',    false),
  ('00000000-0000-0000-0000-000000000024', 'seed_txn_041', 0.14,    'USD', CURRENT_DATE - INTERVAL '60 days','Interest Payment',       NULL,             'INCOME',          'INTEREST',         'other',    false),

  -- Additional transactions for variety (Alice Checking)
  ('00000000-0000-0000-0000-000000000021', 'seed_txn_042', 38.00,   'USD', CURRENT_DATE - INTERVAL '30 days','Verizon Wireless',       'Verizon',        'RENT_AND_UTILITIES', 'PHONE',         'online',   false),
  ('00000000-0000-0000-0000-000000000021', 'seed_txn_043', 23.75,   'USD', CURRENT_DATE - INTERVAL '35 days','Chick-fil-A',            'Chick-fil-A',    'FOOD_AND_DRINK',  'FAST_FOOD',        'in_store', false),
  ('00000000-0000-0000-0000-000000000021', 'seed_txn_044', -3200.00,'USD', CURRENT_DATE - INTERVAL '36 days','Payroll Deposit',        NULL,             'INCOME',          'WAGES',            'other',    false),
  ('00000000-0000-0000-0000-000000000021', 'seed_txn_045', 52.00,   'USD', CURRENT_DATE - INTERVAL '40 days','Costco Gas',             'Costco',         'TRANSPORTATION',  'GAS_STATIONS',     'in_store', false),
  ('00000000-0000-0000-0000-000000000021', 'seed_txn_046', 1850.00, 'USD', CURRENT_DATE - INTERVAL '36 days','Rent Payment',           NULL,             'RENT_AND_UTILITIES', 'RENT',          'other',    false),
  ('00000000-0000-0000-0000-000000000021', 'seed_txn_047', 110.00,  'USD', CURRENT_DATE - INTERVAL '45 days','Whole Foods Market',     'Whole Foods',    'FOOD_AND_DRINK',  'GROCERIES',        'in_store', false),

  -- Additional Bob Checking
  ('00000000-0000-0000-0000-000000000023', 'seed_txn_048', -2800.00,'USD', CURRENT_DATE - INTERVAL '35 days','Direct Deposit',         NULL,             'INCOME',          'WAGES',            'other',    false),
  ('00000000-0000-0000-0000-000000000023', 'seed_txn_049', 1500.00, 'USD', CURRENT_DATE - INTERVAL '36 days','Rent',                   NULL,             'RENT_AND_UTILITIES', 'RENT',          'other',    false),
  ('00000000-0000-0000-0000-000000000023', 'seed_txn_050', 95.00,   'USD', CURRENT_DATE - INTERVAL '40 days','Kroger',                 'Kroger',         'FOOD_AND_DRINK',  'GROCERIES',        'in_store', false)
ON CONFLICT (external_transaction_id) DO NOTHING;
`

const syncLogsSQL = `
INSERT INTO sync_logs (id, connection_id, "trigger", status, added_count, modified_count, removed_count, started_at, completed_at) VALUES
  ('00000000-0000-0000-0000-000000000031', '00000000-0000-0000-0000-000000000011', 'initial', 'success', 19, 0, 0, NOW() - INTERVAL '1 hour', NOW() - INTERVAL '59 minutes'),
  ('00000000-0000-0000-0000-000000000032', '00000000-0000-0000-0000-000000000012', 'initial', 'success', 17, 0, 0, NOW() - INTERVAL '1 hour', NOW() - INTERVAL '58 minutes')
ON CONFLICT (id) DO NOTHING;
`

const tellerConnectionSQL = `
INSERT INTO bank_connections (id, user_id, provider, institution_name, external_id, encrypted_credentials, status, sync_cursor) VALUES
  ('00000000-0000-0000-0000-000000000013', '00000000-0000-0000-0000-000000000001', 'teller', 'Wells Fargo', 'enr_seed_teller_1', 'seed_encrypted_teller_token_1', 'active', TO_CHAR(NOW(), 'YYYY-MM-DD"T"HH24:MI:SS"Z"'))
ON CONFLICT (id) DO NOTHING;
`

const tellerAccountsSQL = `
INSERT INTO accounts (id, connection_id, external_account_id, name, type, subtype, mask, balance_current, balance_available, balance_limit, iso_currency_code) VALUES
  ('00000000-0000-0000-0000-000000000025', '00000000-0000-0000-0000-000000000013', 'seed_acct_t1', 'Wells Fargo Checking', 'depository', 'checking', '5678', 7500.00, 7200.00, NULL, 'USD'),
  ('00000000-0000-0000-0000-000000000026', '00000000-0000-0000-0000-000000000013', 'seed_acct_t2', 'Wells Fargo Savings',  'depository', 'savings',  '9012', 22000.00, 22000.00, NULL, 'USD')
ON CONFLICT (id) DO NOTHING;
`

const tellerTransactionsSQL = `
INSERT INTO transactions (account_id, external_transaction_id, amount, iso_currency_code, date, name, merchant_name, category_primary, category_detailed, payment_channel, pending) VALUES
  -- Alice / Wells Fargo Checking (seed_acct_t1)
  ('00000000-0000-0000-0000-000000000025', 'seed_txn_t01', 12.50,   'USD', CURRENT_DATE - INTERVAL '1 day',  'Taco Bell',              'Taco Bell',      'FOOD_AND_DRINK',  NULL, 'other', false),
  ('00000000-0000-0000-0000-000000000025', 'seed_txn_t02', 45.00,   'USD', CURRENT_DATE - INTERVAL '3 days', 'Chevron Gas',            'Chevron',        'TRANSPORTATION',  NULL, 'other', false),
  ('00000000-0000-0000-0000-000000000025', 'seed_txn_t03', -2500.00,'USD', CURRENT_DATE - INTERVAL '5 days', 'Payroll Direct Deposit', NULL,             'INCOME',          NULL, 'other', false),
  ('00000000-0000-0000-0000-000000000025', 'seed_txn_t04', 8.75,    'USD', CURRENT_DATE,                     'Panda Express',          'Panda Express',  'FOOD_AND_DRINK',  NULL, 'other', true),

  -- Alice / Wells Fargo Savings (seed_acct_t2)
  ('00000000-0000-0000-0000-000000000026', 'seed_txn_t05', -300.00, 'USD', CURRENT_DATE - INTERVAL '2 days', 'Transfer from Checking', NULL,             'TRANSFER_IN',     NULL, 'other', false),
  ('00000000-0000-0000-0000-000000000026', 'seed_txn_t06', 0.18,    'USD', CURRENT_DATE - INTERVAL '30 days','Interest Payment',       NULL,             'INCOME',          NULL, 'other', false)
ON CONFLICT (external_transaction_id) DO NOTHING;
`

const tellerSyncLogSQL = `
INSERT INTO sync_logs (id, connection_id, "trigger", status, added_count, modified_count, removed_count, started_at, completed_at) VALUES
  ('00000000-0000-0000-0000-000000000033', '00000000-0000-0000-0000-000000000013', 'initial', 'success', 6, 0, 0, NOW() - INTERVAL '30 minutes', NOW() - INTERVAL '29 minutes')
ON CONFLICT (id) DO NOTHING;
`
