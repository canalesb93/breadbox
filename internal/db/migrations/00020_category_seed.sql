-- +goose Up

-- System uncategorized category (always exists, undeletable via app logic)
INSERT INTO categories (slug, display_name, parent_id, icon, color, sort_order, is_system)
VALUES ('uncategorized', 'Uncategorized', NULL, 'help-circle', '#9ca3af', 9999, TRUE)
ON CONFLICT (slug) DO NOTHING;

-- Primary categories (16 groups)
INSERT INTO categories (slug, display_name, parent_id, icon, color, sort_order, is_system) VALUES
('income',                   'Income',                 NULL, 'wallet',             '#22c55e', 1,  TRUE),
('transfer_in',              'Transfers In',           NULL, 'arrow-down-circle',  '#3b82f6', 2,  TRUE),
('transfer_out',             'Transfers Out',          NULL, 'arrow-up-circle',    '#8b5cf6', 3,  TRUE),
('loan_payments',            'Loan Payments',          NULL, 'landmark',           '#f59e0b', 4,  TRUE),
('bank_fees',                'Bank Fees',              NULL, 'building-2',         '#ef4444', 5,  TRUE),
('entertainment',            'Entertainment',          NULL, 'tv',                 '#ec4899', 6,  TRUE),
('food_and_drink',           'Food & Drink',           NULL, 'utensils',           '#f97316', 7,  TRUE),
('general_merchandise',      'Shopping',               NULL, 'shopping-bag',       '#14b8a6', 8,  TRUE),
('home_improvement',         'Home',                   NULL, 'home',               '#a855f7', 9,  TRUE),
('medical',                  'Medical',                NULL, 'heart-pulse',        '#ef4444', 10, TRUE),
('personal_care',            'Personal Care',          NULL, 'sparkles',           '#f472b6', 11, TRUE),
('general_services',         'Services',               NULL, 'wrench',             '#6366f1', 12, TRUE),
('government_and_non_profit','Government & Donations', NULL, 'building',           '#64748b', 13, TRUE),
('transportation',           'Transportation',         NULL, 'car',                '#0ea5e9', 14, TRUE),
('travel',                   'Travel',                 NULL, 'plane',              '#06b6d4', 15, TRUE),
('rent_and_utilities',       'Rent & Utilities',       NULL, 'zap',                '#eab308', 16, TRUE)
ON CONFLICT (slug) DO NOTHING;

-- Detailed categories: INCOME
INSERT INTO categories (slug, display_name, parent_id, icon, sort_order, is_system) VALUES
('income_dividends',         'Dividends',              (SELECT id FROM categories WHERE slug = 'income'), 'trending-up',    1, TRUE),
('income_interest_earned',   'Interest Earned',        (SELECT id FROM categories WHERE slug = 'income'), 'percent',        2, TRUE),
('income_retirement_pension','Retirement & Pension',   (SELECT id FROM categories WHERE slug = 'income'), 'landmark',       3, TRUE),
('income_tax_refund',        'Tax Refund',             (SELECT id FROM categories WHERE slug = 'income'), 'receipt',        4, TRUE),
('income_unemployment',      'Unemployment',           (SELECT id FROM categories WHERE slug = 'income'), 'briefcase',      5, TRUE),
('income_wages',             'Wages & Salary',         (SELECT id FROM categories WHERE slug = 'income'), 'banknote',       6, TRUE),
('income_other',             'Other Income',           (SELECT id FROM categories WHERE slug = 'income'), 'wallet',         7, TRUE)
ON CONFLICT (slug) DO NOTHING;

-- Detailed categories: TRANSFER_IN
INSERT INTO categories (slug, display_name, parent_id, icon, sort_order, is_system) VALUES
('transfer_in_cash_advances_and_loans', 'Cash Advances & Loans', (SELECT id FROM categories WHERE slug = 'transfer_in'), 'banknote',       1, TRUE),
('transfer_in_deposit',                 'Deposits',              (SELECT id FROM categories WHERE slug = 'transfer_in'), 'download',       2, TRUE),
('transfer_in_investment_and_retirement_funds', 'Investment & Retirement', (SELECT id FROM categories WHERE slug = 'transfer_in'), 'trending-up', 3, TRUE),
('transfer_in_savings',                 'Savings',               (SELECT id FROM categories WHERE slug = 'transfer_in'), 'piggy-bank',     4, TRUE),
('transfer_in_account_transfer',        'Account Transfer',      (SELECT id FROM categories WHERE slug = 'transfer_in'), 'arrow-right-left', 5, TRUE),
('transfer_in_other',                   'Other Transfer In',     (SELECT id FROM categories WHERE slug = 'transfer_in'), 'arrow-down-circle', 6, TRUE)
ON CONFLICT (slug) DO NOTHING;

-- Detailed categories: TRANSFER_OUT
INSERT INTO categories (slug, display_name, parent_id, icon, sort_order, is_system) VALUES
('transfer_out_investment_and_retirement_funds', 'Investment & Retirement', (SELECT id FROM categories WHERE slug = 'transfer_out'), 'trending-up', 1, TRUE),
('transfer_out_savings',                'Savings',               (SELECT id FROM categories WHERE slug = 'transfer_out'), 'piggy-bank',     2, TRUE),
('transfer_out_withdrawal',             'Withdrawals',           (SELECT id FROM categories WHERE slug = 'transfer_out'), 'upload',         3, TRUE),
('transfer_out_account_transfer',       'Account Transfer',      (SELECT id FROM categories WHERE slug = 'transfer_out'), 'arrow-right-left', 4, TRUE),
('transfer_out_other',                  'Other Transfer Out',    (SELECT id FROM categories WHERE slug = 'transfer_out'), 'arrow-up-circle', 5, TRUE)
ON CONFLICT (slug) DO NOTHING;

-- Detailed categories: LOAN_PAYMENTS
INSERT INTO categories (slug, display_name, parent_id, icon, sort_order, is_system) VALUES
('loan_payments_car_payment',           'Car Payment',           (SELECT id FROM categories WHERE slug = 'loan_payments'), 'car',            1, TRUE),
('loan_payments_credit_card_payment',   'Credit Card Payment',   (SELECT id FROM categories WHERE slug = 'loan_payments'), 'credit-card',    2, TRUE),
('loan_payments_personal_loan_payment', 'Personal Loan',         (SELECT id FROM categories WHERE slug = 'loan_payments'), 'user',           3, TRUE),
('loan_payments_mortgage_payment',      'Mortgage',              (SELECT id FROM categories WHERE slug = 'loan_payments'), 'home',           4, TRUE),
('loan_payments_student_loan_payment',  'Student Loan',          (SELECT id FROM categories WHERE slug = 'loan_payments'), 'graduation-cap', 5, TRUE),
('loan_payments_insurance_payment',     'Insurance Payment',     (SELECT id FROM categories WHERE slug = 'loan_payments'), 'shield',         6, TRUE),
('loan_payments_other',                 'Other Payment',         (SELECT id FROM categories WHERE slug = 'loan_payments'), 'landmark',       7, TRUE)
ON CONFLICT (slug) DO NOTHING;

-- Detailed categories: BANK_FEES
INSERT INTO categories (slug, display_name, parent_id, icon, sort_order, is_system) VALUES
('bank_fees_atm_fees',                  'ATM Fees',              (SELECT id FROM categories WHERE slug = 'bank_fees'), 'landmark',       1, TRUE),
('bank_fees_foreign_transaction_fees',  'Foreign Transaction Fees', (SELECT id FROM categories WHERE slug = 'bank_fees'), 'globe',       2, TRUE),
('bank_fees_insufficient_funds',        'Insufficient Funds',    (SELECT id FROM categories WHERE slug = 'bank_fees'), 'alert-circle',   3, TRUE),
('bank_fees_interest_charge',           'Interest Charge',       (SELECT id FROM categories WHERE slug = 'bank_fees'), 'percent',        4, TRUE),
('bank_fees_overdraft_fees',            'Overdraft Fees',        (SELECT id FROM categories WHERE slug = 'bank_fees'), 'alert-triangle', 5, TRUE),
('bank_fees_other',                     'Other Bank Fees',       (SELECT id FROM categories WHERE slug = 'bank_fees'), 'building-2',     6, TRUE)
ON CONFLICT (slug) DO NOTHING;

-- Detailed categories: ENTERTAINMENT
INSERT INTO categories (slug, display_name, parent_id, icon, sort_order, is_system) VALUES
('entertainment_casinos_and_gambling',  'Casinos & Gambling',    (SELECT id FROM categories WHERE slug = 'entertainment'), 'dice',           1, TRUE),
('entertainment_music_and_audio',       'Music & Audio',         (SELECT id FROM categories WHERE slug = 'entertainment'), 'music',          2, TRUE),
('entertainment_sporting_events_amusement_parks_and_museums', 'Events & Attractions', (SELECT id FROM categories WHERE slug = 'entertainment'), 'ticket', 3, TRUE),
('entertainment_tv_and_movies',         'TV & Movies',           (SELECT id FROM categories WHERE slug = 'entertainment'), 'film',           4, TRUE),
('entertainment_video_games',           'Video Games',           (SELECT id FROM categories WHERE slug = 'entertainment'), 'gamepad-2',      5, TRUE),
('entertainment_other',                 'Other Entertainment',   (SELECT id FROM categories WHERE slug = 'entertainment'), 'tv',             6, TRUE)
ON CONFLICT (slug) DO NOTHING;

-- Detailed categories: FOOD_AND_DRINK
INSERT INTO categories (slug, display_name, parent_id, icon, sort_order, is_system) VALUES
('food_and_drink_beer_wine_and_liquor', 'Beer, Wine & Liquor',   (SELECT id FROM categories WHERE slug = 'food_and_drink'), 'wine',           1, TRUE),
('food_and_drink_coffee',               'Coffee Shops',          (SELECT id FROM categories WHERE slug = 'food_and_drink'), 'coffee',         2, TRUE),
('food_and_drink_fast_food',            'Fast Food',             (SELECT id FROM categories WHERE slug = 'food_and_drink'), 'sandwich',       3, TRUE),
('food_and_drink_groceries',            'Groceries',             (SELECT id FROM categories WHERE slug = 'food_and_drink'), 'shopping-cart',  4, TRUE),
('food_and_drink_restaurant',           'Restaurants',           (SELECT id FROM categories WHERE slug = 'food_and_drink'), 'utensils',       5, TRUE),
('food_and_drink_vending_machines',     'Vending Machines',      (SELECT id FROM categories WHERE slug = 'food_and_drink'), 'box',            6, TRUE),
('food_and_drink_delivery',             'Food Delivery',         (SELECT id FROM categories WHERE slug = 'food_and_drink'), 'bike',           7, TRUE),
('food_and_drink_other',                'Other Food & Drink',    (SELECT id FROM categories WHERE slug = 'food_and_drink'), 'utensils',       8, TRUE)
ON CONFLICT (slug) DO NOTHING;

-- Detailed categories: GENERAL_MERCHANDISE
INSERT INTO categories (slug, display_name, parent_id, icon, sort_order, is_system) VALUES
('general_merchandise_bookstores_and_newsstands', 'Books & News',      (SELECT id FROM categories WHERE slug = 'general_merchandise'), 'book-open', 1, TRUE),
('general_merchandise_clothing_and_accessories',  'Clothing & Accessories', (SELECT id FROM categories WHERE slug = 'general_merchandise'), 'shirt', 2, TRUE),
('general_merchandise_convenience_stores',        'Convenience Stores', (SELECT id FROM categories WHERE slug = 'general_merchandise'), 'store', 3, TRUE),
('general_merchandise_department_stores',         'Department Stores',  (SELECT id FROM categories WHERE slug = 'general_merchandise'), 'building', 4, TRUE),
('general_merchandise_discount_stores',           'Discount Stores',    (SELECT id FROM categories WHERE slug = 'general_merchandise'), 'tag', 5, TRUE),
('general_merchandise_electronics',               'Electronics',        (SELECT id FROM categories WHERE slug = 'general_merchandise'), 'smartphone', 6, TRUE),
('general_merchandise_gifts_and_novelties',       'Gifts & Novelties',  (SELECT id FROM categories WHERE slug = 'general_merchandise'), 'gift', 7, TRUE),
('general_merchandise_office_supplies',           'Office Supplies',    (SELECT id FROM categories WHERE slug = 'general_merchandise'), 'paperclip', 8, TRUE),
('general_merchandise_online_marketplaces',       'Online Marketplaces',(SELECT id FROM categories WHERE slug = 'general_merchandise'), 'globe', 9, TRUE),
('general_merchandise_pet_supplies',              'Pet Supplies',       (SELECT id FROM categories WHERE slug = 'general_merchandise'), 'paw-print', 10, TRUE),
('general_merchandise_sporting_goods',            'Sporting Goods',     (SELECT id FROM categories WHERE slug = 'general_merchandise'), 'dumbbell', 11, TRUE),
('general_merchandise_superstores',               'Superstores',        (SELECT id FROM categories WHERE slug = 'general_merchandise'), 'shopping-bag', 12, TRUE),
('general_merchandise_tobacco_and_vape',          'Tobacco & Vape',     (SELECT id FROM categories WHERE slug = 'general_merchandise'), 'cigarette', 13, TRUE),
('general_merchandise_other',                     'Other Shopping',     (SELECT id FROM categories WHERE slug = 'general_merchandise'), 'shopping-bag', 14, TRUE)
ON CONFLICT (slug) DO NOTHING;

-- Detailed categories: HOME_IMPROVEMENT
INSERT INTO categories (slug, display_name, parent_id, icon, sort_order, is_system) VALUES
('home_improvement_furniture',             'Furniture',             (SELECT id FROM categories WHERE slug = 'home_improvement'), 'armchair',       1, TRUE),
('home_improvement_hardware',              'Hardware',              (SELECT id FROM categories WHERE slug = 'home_improvement'), 'hammer',         2, TRUE),
('home_improvement_repair_and_maintenance','Repair & Maintenance',  (SELECT id FROM categories WHERE slug = 'home_improvement'), 'wrench',         3, TRUE),
('home_improvement_security',              'Security',              (SELECT id FROM categories WHERE slug = 'home_improvement'), 'shield',         4, TRUE),
('home_improvement_other',                 'Other Home',            (SELECT id FROM categories WHERE slug = 'home_improvement'), 'home',           5, TRUE)
ON CONFLICT (slug) DO NOTHING;

-- Detailed categories: MEDICAL
INSERT INTO categories (slug, display_name, parent_id, icon, sort_order, is_system) VALUES
('medical_dental_care',                 'Dental Care',           (SELECT id FROM categories WHERE slug = 'medical'), 'smile',          1, TRUE),
('medical_eye_care',                    'Eye Care',              (SELECT id FROM categories WHERE slug = 'medical'), 'eye',            2, TRUE),
('medical_nursing_care',                'Nursing Care',          (SELECT id FROM categories WHERE slug = 'medical'), 'stethoscope',    3, TRUE),
('medical_pharmacies_and_supplements',  'Pharmacies',            (SELECT id FROM categories WHERE slug = 'medical'), 'pill',           4, TRUE),
('medical_primary_care',                'Primary Care',          (SELECT id FROM categories WHERE slug = 'medical'), 'heart-pulse',    5, TRUE),
('medical_veterinary_services',         'Veterinary',            (SELECT id FROM categories WHERE slug = 'medical'), 'paw-print',      6, TRUE),
('medical_other',                       'Other Medical',         (SELECT id FROM categories WHERE slug = 'medical'), 'heart-pulse',    7, TRUE)
ON CONFLICT (slug) DO NOTHING;

-- Detailed categories: PERSONAL_CARE
INSERT INTO categories (slug, display_name, parent_id, icon, sort_order, is_system) VALUES
('personal_care_gyms_and_fitness_centers','Gyms & Fitness',       (SELECT id FROM categories WHERE slug = 'personal_care'), 'dumbbell',       1, TRUE),
('personal_care_hair_and_beauty',         'Hair & Beauty',        (SELECT id FROM categories WHERE slug = 'personal_care'), 'scissors',       2, TRUE),
('personal_care_laundry_and_dry_cleaning','Laundry & Dry Cleaning',(SELECT id FROM categories WHERE slug = 'personal_care'), 'shirt',         3, TRUE),
('personal_care_other',                   'Other Personal Care',  (SELECT id FROM categories WHERE slug = 'personal_care'), 'sparkles',       4, TRUE)
ON CONFLICT (slug) DO NOTHING;

-- Detailed categories: GENERAL_SERVICES
INSERT INTO categories (slug, display_name, parent_id, icon, sort_order, is_system) VALUES
('general_services_accounting_and_financial_planning', 'Accounting & Financial Planning', (SELECT id FROM categories WHERE slug = 'general_services'), 'calculator', 1, TRUE),
('general_services_automotive',            'Automotive',            (SELECT id FROM categories WHERE slug = 'general_services'), 'car',            2, TRUE),
('general_services_childcare',             'Childcare',             (SELECT id FROM categories WHERE slug = 'general_services'), 'baby',           3, TRUE),
('general_services_consulting_and_legal',  'Consulting & Legal',    (SELECT id FROM categories WHERE slug = 'general_services'), 'scale',          4, TRUE),
('general_services_education',             'Education',             (SELECT id FROM categories WHERE slug = 'general_services'), 'graduation-cap', 5, TRUE),
('general_services_insurance',             'Insurance',             (SELECT id FROM categories WHERE slug = 'general_services'), 'shield',         6, TRUE),
('general_services_postage_and_shipping',  'Postage & Shipping',   (SELECT id FROM categories WHERE slug = 'general_services'), 'package',        7, TRUE),
('general_services_storage',               'Storage',               (SELECT id FROM categories WHERE slug = 'general_services'), 'archive',        8, TRUE),
('general_services_other',                 'Other Services',        (SELECT id FROM categories WHERE slug = 'general_services'), 'wrench',         9, TRUE)
ON CONFLICT (slug) DO NOTHING;

-- Detailed categories: GOVERNMENT_AND_NON_PROFIT
INSERT INTO categories (slug, display_name, parent_id, icon, sort_order, is_system) VALUES
('government_and_non_profit_donations',                        'Donations',            (SELECT id FROM categories WHERE slug = 'government_and_non_profit'), 'heart',     1, TRUE),
('government_and_non_profit_government_departments_and_agencies','Government Agencies', (SELECT id FROM categories WHERE slug = 'government_and_non_profit'), 'landmark',  2, TRUE),
('government_and_non_profit_tax_payment',                      'Tax Payment',          (SELECT id FROM categories WHERE slug = 'government_and_non_profit'), 'receipt',    3, TRUE),
('government_and_non_profit_other',                            'Other Government',     (SELECT id FROM categories WHERE slug = 'government_and_non_profit'), 'building',   4, TRUE)
ON CONFLICT (slug) DO NOTHING;

-- Detailed categories: TRANSPORTATION
INSERT INTO categories (slug, display_name, parent_id, icon, sort_order, is_system) VALUES
('transportation_bikes_and_scooters', 'Bikes & Scooters',  (SELECT id FROM categories WHERE slug = 'transportation'), 'bike',           1, TRUE),
('transportation_gas',                'Gas',               (SELECT id FROM categories WHERE slug = 'transportation'), 'fuel',           2, TRUE),
('transportation_parking',            'Parking',           (SELECT id FROM categories WHERE slug = 'transportation'), 'square-parking', 3, TRUE),
('transportation_public_transit',     'Public Transit',    (SELECT id FROM categories WHERE slug = 'transportation'), 'train',          4, TRUE),
('transportation_taxis_and_ride_shares','Taxis & Rideshares',(SELECT id FROM categories WHERE slug = 'transportation'), 'map-pin',      5, TRUE),
('transportation_tolls',              'Tolls',             (SELECT id FROM categories WHERE slug = 'transportation'), 'milestone',      6, TRUE),
('transportation_other',              'Other Transportation',(SELECT id FROM categories WHERE slug = 'transportation'), 'car',           7, TRUE)
ON CONFLICT (slug) DO NOTHING;

-- Detailed categories: TRAVEL
INSERT INTO categories (slug, display_name, parent_id, icon, sort_order, is_system) VALUES
('travel_flights',     'Flights',       (SELECT id FROM categories WHERE slug = 'travel'), 'plane',          1, TRUE),
('travel_lodging',     'Lodging',       (SELECT id FROM categories WHERE slug = 'travel'), 'bed',            2, TRUE),
('travel_rental_cars', 'Rental Cars',   (SELECT id FROM categories WHERE slug = 'travel'), 'car',            3, TRUE),
('travel_other',       'Other Travel',  (SELECT id FROM categories WHERE slug = 'travel'), 'plane',          4, TRUE)
ON CONFLICT (slug) DO NOTHING;

-- Detailed categories: RENT_AND_UTILITIES
INSERT INTO categories (slug, display_name, parent_id, icon, sort_order, is_system) VALUES
('rent_and_utilities_gas_and_electricity',       'Gas & Electricity',      (SELECT id FROM categories WHERE slug = 'rent_and_utilities'), 'flame',     1, TRUE),
('rent_and_utilities_internet_and_cable',        'Internet & Cable',       (SELECT id FROM categories WHERE slug = 'rent_and_utilities'), 'wifi',      2, TRUE),
('rent_and_utilities_rent',                      'Rent',                   (SELECT id FROM categories WHERE slug = 'rent_and_utilities'), 'home',      3, TRUE),
('rent_and_utilities_sewage_and_waste_management','Sewage & Waste',        (SELECT id FROM categories WHERE slug = 'rent_and_utilities'), 'trash-2',   4, TRUE),
('rent_and_utilities_telephone',                 'Telephone',              (SELECT id FROM categories WHERE slug = 'rent_and_utilities'), 'phone',     5, TRUE),
('rent_and_utilities_water',                     'Water',                  (SELECT id FROM categories WHERE slug = 'rent_and_utilities'), 'droplets',  6, TRUE),
('rent_and_utilities_other',                     'Other Utilities',        (SELECT id FROM categories WHERE slug = 'rent_and_utilities'), 'zap',       7, TRUE)
ON CONFLICT (slug) DO NOTHING;

-- ==========================================
-- Plaid provider mappings (primary + detailed)
-- ==========================================

-- Primary mappings (Plaid primary string → primary category)
INSERT INTO category_mappings (provider, provider_category, category_id) VALUES
('plaid', 'INCOME',                   (SELECT id FROM categories WHERE slug = 'income')),
('plaid', 'TRANSFER_IN',              (SELECT id FROM categories WHERE slug = 'transfer_in')),
('plaid', 'TRANSFER_OUT',             (SELECT id FROM categories WHERE slug = 'transfer_out')),
('plaid', 'LOAN_PAYMENTS',            (SELECT id FROM categories WHERE slug = 'loan_payments')),
('plaid', 'BANK_FEES',                (SELECT id FROM categories WHERE slug = 'bank_fees')),
('plaid', 'ENTERTAINMENT',            (SELECT id FROM categories WHERE slug = 'entertainment')),
('plaid', 'FOOD_AND_DRINK',           (SELECT id FROM categories WHERE slug = 'food_and_drink')),
('plaid', 'GENERAL_MERCHANDISE',      (SELECT id FROM categories WHERE slug = 'general_merchandise')),
('plaid', 'HOME_IMPROVEMENT',         (SELECT id FROM categories WHERE slug = 'home_improvement')),
('plaid', 'MEDICAL',                  (SELECT id FROM categories WHERE slug = 'medical')),
('plaid', 'PERSONAL_CARE',            (SELECT id FROM categories WHERE slug = 'personal_care')),
('plaid', 'GENERAL_SERVICES',         (SELECT id FROM categories WHERE slug = 'general_services')),
('plaid', 'GOVERNMENT_AND_NON_PROFIT',(SELECT id FROM categories WHERE slug = 'government_and_non_profit')),
('plaid', 'TRANSPORTATION',           (SELECT id FROM categories WHERE slug = 'transportation')),
('plaid', 'TRAVEL',                   (SELECT id FROM categories WHERE slug = 'travel')),
('plaid', 'RENT_AND_UTILITIES',       (SELECT id FROM categories WHERE slug = 'rent_and_utilities'))
ON CONFLICT (provider, provider_category) DO NOTHING;

-- Detailed mappings: INCOME
INSERT INTO category_mappings (provider, provider_category, category_id) VALUES
('plaid', 'INCOME_DIVIDENDS',         (SELECT id FROM categories WHERE slug = 'income_dividends')),
('plaid', 'INCOME_INTEREST_EARNED',   (SELECT id FROM categories WHERE slug = 'income_interest_earned')),
('plaid', 'INCOME_RETIREMENT_PENSION',(SELECT id FROM categories WHERE slug = 'income_retirement_pension')),
('plaid', 'INCOME_TAX_REFUND',        (SELECT id FROM categories WHERE slug = 'income_tax_refund')),
('plaid', 'INCOME_UNEMPLOYMENT',      (SELECT id FROM categories WHERE slug = 'income_unemployment')),
('plaid', 'INCOME_WAGES',             (SELECT id FROM categories WHERE slug = 'income_wages')),
('plaid', 'INCOME_OTHER_INCOME',      (SELECT id FROM categories WHERE slug = 'income_other'))
ON CONFLICT (provider, provider_category) DO NOTHING;

-- Detailed mappings: TRANSFER_IN
INSERT INTO category_mappings (provider, provider_category, category_id) VALUES
('plaid', 'TRANSFER_IN_CASH_ADVANCES_AND_LOANS',          (SELECT id FROM categories WHERE slug = 'transfer_in_cash_advances_and_loans')),
('plaid', 'TRANSFER_IN_DEPOSIT',                           (SELECT id FROM categories WHERE slug = 'transfer_in_deposit')),
('plaid', 'TRANSFER_IN_INVESTMENT_AND_RETIREMENT_FUNDS',   (SELECT id FROM categories WHERE slug = 'transfer_in_investment_and_retirement_funds')),
('plaid', 'TRANSFER_IN_SAVINGS',                           (SELECT id FROM categories WHERE slug = 'transfer_in_savings')),
('plaid', 'TRANSFER_IN_ACCOUNT_TRANSFER',                  (SELECT id FROM categories WHERE slug = 'transfer_in_account_transfer')),
('plaid', 'TRANSFER_IN_OTHER_TRANSFER_IN',                 (SELECT id FROM categories WHERE slug = 'transfer_in_other'))
ON CONFLICT (provider, provider_category) DO NOTHING;

-- Detailed mappings: TRANSFER_OUT
INSERT INTO category_mappings (provider, provider_category, category_id) VALUES
('plaid', 'TRANSFER_OUT_INVESTMENT_AND_RETIREMENT_FUNDS',  (SELECT id FROM categories WHERE slug = 'transfer_out_investment_and_retirement_funds')),
('plaid', 'TRANSFER_OUT_SAVINGS',                          (SELECT id FROM categories WHERE slug = 'transfer_out_savings')),
('plaid', 'TRANSFER_OUT_WITHDRAWAL',                       (SELECT id FROM categories WHERE slug = 'transfer_out_withdrawal')),
('plaid', 'TRANSFER_OUT_ACCOUNT_TRANSFER',                 (SELECT id FROM categories WHERE slug = 'transfer_out_account_transfer')),
('plaid', 'TRANSFER_OUT_OTHER_TRANSFER_OUT',               (SELECT id FROM categories WHERE slug = 'transfer_out_other'))
ON CONFLICT (provider, provider_category) DO NOTHING;

-- Detailed mappings: LOAN_PAYMENTS
INSERT INTO category_mappings (provider, provider_category, category_id) VALUES
('plaid', 'LOAN_PAYMENTS_CAR_PAYMENT',           (SELECT id FROM categories WHERE slug = 'loan_payments_car_payment')),
('plaid', 'LOAN_PAYMENTS_CREDIT_CARD_PAYMENT',   (SELECT id FROM categories WHERE slug = 'loan_payments_credit_card_payment')),
('plaid', 'LOAN_PAYMENTS_PERSONAL_LOAN_PAYMENT', (SELECT id FROM categories WHERE slug = 'loan_payments_personal_loan_payment')),
('plaid', 'LOAN_PAYMENTS_MORTGAGE_PAYMENT',      (SELECT id FROM categories WHERE slug = 'loan_payments_mortgage_payment')),
('plaid', 'LOAN_PAYMENTS_STUDENT_LOAN_PAYMENT',  (SELECT id FROM categories WHERE slug = 'loan_payments_student_loan_payment')),
('plaid', 'LOAN_PAYMENTS_INSURANCE_PAYMENT',     (SELECT id FROM categories WHERE slug = 'loan_payments_insurance_payment')),
('plaid', 'LOAN_PAYMENTS_OTHER_PAYMENT',         (SELECT id FROM categories WHERE slug = 'loan_payments_other'))
ON CONFLICT (provider, provider_category) DO NOTHING;

-- Detailed mappings: BANK_FEES
INSERT INTO category_mappings (provider, provider_category, category_id) VALUES
('plaid', 'BANK_FEES_ATM_FEES',                  (SELECT id FROM categories WHERE slug = 'bank_fees_atm_fees')),
('plaid', 'BANK_FEES_FOREIGN_TRANSACTION_FEES',  (SELECT id FROM categories WHERE slug = 'bank_fees_foreign_transaction_fees')),
('plaid', 'BANK_FEES_INSUFFICIENT_FUNDS',        (SELECT id FROM categories WHERE slug = 'bank_fees_insufficient_funds')),
('plaid', 'BANK_FEES_INTEREST_CHARGE',           (SELECT id FROM categories WHERE slug = 'bank_fees_interest_charge')),
('plaid', 'BANK_FEES_OVERDRAFT_FEES',            (SELECT id FROM categories WHERE slug = 'bank_fees_overdraft_fees')),
('plaid', 'BANK_FEES_OTHER_BANK_FEES',           (SELECT id FROM categories WHERE slug = 'bank_fees_other'))
ON CONFLICT (provider, provider_category) DO NOTHING;

-- Detailed mappings: ENTERTAINMENT
INSERT INTO category_mappings (provider, provider_category, category_id) VALUES
('plaid', 'ENTERTAINMENT_CASINOS_AND_GAMBLING',                       (SELECT id FROM categories WHERE slug = 'entertainment_casinos_and_gambling')),
('plaid', 'ENTERTAINMENT_MUSIC_AND_AUDIO',                            (SELECT id FROM categories WHERE slug = 'entertainment_music_and_audio')),
('plaid', 'ENTERTAINMENT_SPORTING_EVENTS_AMUSEMENT_PARKS_AND_MUSEUMS',(SELECT id FROM categories WHERE slug = 'entertainment_sporting_events_amusement_parks_and_museums')),
('plaid', 'ENTERTAINMENT_TV_AND_MOVIES',                              (SELECT id FROM categories WHERE slug = 'entertainment_tv_and_movies')),
('plaid', 'ENTERTAINMENT_VIDEO_GAMES',                                (SELECT id FROM categories WHERE slug = 'entertainment_video_games')),
('plaid', 'ENTERTAINMENT_OTHER_ENTERTAINMENT',                        (SELECT id FROM categories WHERE slug = 'entertainment_other'))
ON CONFLICT (provider, provider_category) DO NOTHING;

-- Detailed mappings: FOOD_AND_DRINK
INSERT INTO category_mappings (provider, provider_category, category_id) VALUES
('plaid', 'FOOD_AND_DRINK_BEER_WINE_AND_LIQUOR',     (SELECT id FROM categories WHERE slug = 'food_and_drink_beer_wine_and_liquor')),
('plaid', 'FOOD_AND_DRINK_COFFEE',                    (SELECT id FROM categories WHERE slug = 'food_and_drink_coffee')),
('plaid', 'FOOD_AND_DRINK_FAST_FOOD',                 (SELECT id FROM categories WHERE slug = 'food_and_drink_fast_food')),
('plaid', 'FOOD_AND_DRINK_GROCERIES',                 (SELECT id FROM categories WHERE slug = 'food_and_drink_groceries')),
('plaid', 'FOOD_AND_DRINK_RESTAURANT',                (SELECT id FROM categories WHERE slug = 'food_and_drink_restaurant')),
('plaid', 'FOOD_AND_DRINK_VENDING_MACHINES',          (SELECT id FROM categories WHERE slug = 'food_and_drink_vending_machines')),
('plaid', 'FOOD_AND_DRINK_FOOD_DELIVERY_SERVICES',    (SELECT id FROM categories WHERE slug = 'food_and_drink_delivery')),
('plaid', 'FOOD_AND_DRINK_OTHER_FOOD_AND_DRINK',      (SELECT id FROM categories WHERE slug = 'food_and_drink_other'))
ON CONFLICT (provider, provider_category) DO NOTHING;

-- Detailed mappings: GENERAL_MERCHANDISE
INSERT INTO category_mappings (provider, provider_category, category_id) VALUES
('plaid', 'GENERAL_MERCHANDISE_BOOKSTORES_AND_NEWSSTANDS',  (SELECT id FROM categories WHERE slug = 'general_merchandise_bookstores_and_newsstands')),
('plaid', 'GENERAL_MERCHANDISE_CLOTHING_AND_ACCESSORIES',   (SELECT id FROM categories WHERE slug = 'general_merchandise_clothing_and_accessories')),
('plaid', 'GENERAL_MERCHANDISE_CONVENIENCE_STORES',         (SELECT id FROM categories WHERE slug = 'general_merchandise_convenience_stores')),
('plaid', 'GENERAL_MERCHANDISE_DEPARTMENT_STORES',          (SELECT id FROM categories WHERE slug = 'general_merchandise_department_stores')),
('plaid', 'GENERAL_MERCHANDISE_DISCOUNT_STORES',            (SELECT id FROM categories WHERE slug = 'general_merchandise_discount_stores')),
('plaid', 'GENERAL_MERCHANDISE_ELECTRONICS',                (SELECT id FROM categories WHERE slug = 'general_merchandise_electronics')),
('plaid', 'GENERAL_MERCHANDISE_GIFTS_AND_NOVELTIES',        (SELECT id FROM categories WHERE slug = 'general_merchandise_gifts_and_novelties')),
('plaid', 'GENERAL_MERCHANDISE_OFFICE_SUPPLIES',            (SELECT id FROM categories WHERE slug = 'general_merchandise_office_supplies')),
('plaid', 'GENERAL_MERCHANDISE_ONLINE_MARKETPLACES',        (SELECT id FROM categories WHERE slug = 'general_merchandise_online_marketplaces')),
('plaid', 'GENERAL_MERCHANDISE_PET_SUPPLIES',               (SELECT id FROM categories WHERE slug = 'general_merchandise_pet_supplies')),
('plaid', 'GENERAL_MERCHANDISE_SPORTING_GOODS',             (SELECT id FROM categories WHERE slug = 'general_merchandise_sporting_goods')),
('plaid', 'GENERAL_MERCHANDISE_SUPERSTORES',                (SELECT id FROM categories WHERE slug = 'general_merchandise_superstores')),
('plaid', 'GENERAL_MERCHANDISE_TOBACCO_AND_VAPE',           (SELECT id FROM categories WHERE slug = 'general_merchandise_tobacco_and_vape')),
('plaid', 'GENERAL_MERCHANDISE_OTHER_GENERAL_MERCHANDISE',  (SELECT id FROM categories WHERE slug = 'general_merchandise_other'))
ON CONFLICT (provider, provider_category) DO NOTHING;

-- Detailed mappings: HOME_IMPROVEMENT
INSERT INTO category_mappings (provider, provider_category, category_id) VALUES
('plaid', 'HOME_IMPROVEMENT_FURNITURE',              (SELECT id FROM categories WHERE slug = 'home_improvement_furniture')),
('plaid', 'HOME_IMPROVEMENT_HARDWARE',               (SELECT id FROM categories WHERE slug = 'home_improvement_hardware')),
('plaid', 'HOME_IMPROVEMENT_REPAIR_AND_MAINTENANCE', (SELECT id FROM categories WHERE slug = 'home_improvement_repair_and_maintenance')),
('plaid', 'HOME_IMPROVEMENT_SECURITY',               (SELECT id FROM categories WHERE slug = 'home_improvement_security')),
('plaid', 'HOME_IMPROVEMENT_OTHER_HOME_IMPROVEMENT', (SELECT id FROM categories WHERE slug = 'home_improvement_other'))
ON CONFLICT (provider, provider_category) DO NOTHING;

-- Detailed mappings: MEDICAL
INSERT INTO category_mappings (provider, provider_category, category_id) VALUES
('plaid', 'MEDICAL_DENTAL_CARE',                (SELECT id FROM categories WHERE slug = 'medical_dental_care')),
('plaid', 'MEDICAL_EYE_CARE',                   (SELECT id FROM categories WHERE slug = 'medical_eye_care')),
('plaid', 'MEDICAL_NURSING_CARE',               (SELECT id FROM categories WHERE slug = 'medical_nursing_care')),
('plaid', 'MEDICAL_PHARMACIES_AND_SUPPLEMENTS', (SELECT id FROM categories WHERE slug = 'medical_pharmacies_and_supplements')),
('plaid', 'MEDICAL_PRIMARY_CARE',               (SELECT id FROM categories WHERE slug = 'medical_primary_care')),
('plaid', 'MEDICAL_VETERINARY_SERVICES',        (SELECT id FROM categories WHERE slug = 'medical_veterinary_services')),
('plaid', 'MEDICAL_OTHER_MEDICAL',              (SELECT id FROM categories WHERE slug = 'medical_other'))
ON CONFLICT (provider, provider_category) DO NOTHING;

-- Detailed mappings: PERSONAL_CARE
INSERT INTO category_mappings (provider, provider_category, category_id) VALUES
('plaid', 'PERSONAL_CARE_GYMS_AND_FITNESS_CENTERS', (SELECT id FROM categories WHERE slug = 'personal_care_gyms_and_fitness_centers')),
('plaid', 'PERSONAL_CARE_HAIR_AND_BEAUTY',          (SELECT id FROM categories WHERE slug = 'personal_care_hair_and_beauty')),
('plaid', 'PERSONAL_CARE_LAUNDRY_AND_DRY_CLEANING', (SELECT id FROM categories WHERE slug = 'personal_care_laundry_and_dry_cleaning')),
('plaid', 'PERSONAL_CARE_OTHER_PERSONAL_CARE',      (SELECT id FROM categories WHERE slug = 'personal_care_other'))
ON CONFLICT (provider, provider_category) DO NOTHING;

-- Detailed mappings: GENERAL_SERVICES
INSERT INTO category_mappings (provider, provider_category, category_id) VALUES
('plaid', 'GENERAL_SERVICES_ACCOUNTING_AND_FINANCIAL_PLANNING', (SELECT id FROM categories WHERE slug = 'general_services_accounting_and_financial_planning')),
('plaid', 'GENERAL_SERVICES_AUTOMOTIVE',             (SELECT id FROM categories WHERE slug = 'general_services_automotive')),
('plaid', 'GENERAL_SERVICES_CHILDCARE',              (SELECT id FROM categories WHERE slug = 'general_services_childcare')),
('plaid', 'GENERAL_SERVICES_CONSULTING_AND_LEGAL',   (SELECT id FROM categories WHERE slug = 'general_services_consulting_and_legal')),
('plaid', 'GENERAL_SERVICES_EDUCATION',              (SELECT id FROM categories WHERE slug = 'general_services_education')),
('plaid', 'GENERAL_SERVICES_INSURANCE',              (SELECT id FROM categories WHERE slug = 'general_services_insurance')),
('plaid', 'GENERAL_SERVICES_POSTAGE_AND_SHIPPING',   (SELECT id FROM categories WHERE slug = 'general_services_postage_and_shipping')),
('plaid', 'GENERAL_SERVICES_STORAGE',                (SELECT id FROM categories WHERE slug = 'general_services_storage')),
('plaid', 'GENERAL_SERVICES_OTHER_GENERAL_SERVICES', (SELECT id FROM categories WHERE slug = 'general_services_other'))
ON CONFLICT (provider, provider_category) DO NOTHING;

-- Detailed mappings: GOVERNMENT_AND_NON_PROFIT
INSERT INTO category_mappings (provider, provider_category, category_id) VALUES
('plaid', 'GOVERNMENT_AND_NON_PROFIT_DONATIONS',                         (SELECT id FROM categories WHERE slug = 'government_and_non_profit_donations')),
('plaid', 'GOVERNMENT_AND_NON_PROFIT_GOVERNMENT_DEPARTMENTS_AND_AGENCIES',(SELECT id FROM categories WHERE slug = 'government_and_non_profit_government_departments_and_agencies')),
('plaid', 'GOVERNMENT_AND_NON_PROFIT_TAX_PAYMENT',                       (SELECT id FROM categories WHERE slug = 'government_and_non_profit_tax_payment')),
('plaid', 'GOVERNMENT_AND_NON_PROFIT_OTHER_GOVERNMENT_AND_NON_PROFIT',   (SELECT id FROM categories WHERE slug = 'government_and_non_profit_other'))
ON CONFLICT (provider, provider_category) DO NOTHING;

-- Detailed mappings: TRANSPORTATION
INSERT INTO category_mappings (provider, provider_category, category_id) VALUES
('plaid', 'TRANSPORTATION_BIKES_AND_SCOOTERS',  (SELECT id FROM categories WHERE slug = 'transportation_bikes_and_scooters')),
('plaid', 'TRANSPORTATION_GAS',                 (SELECT id FROM categories WHERE slug = 'transportation_gas')),
('plaid', 'TRANSPORTATION_PARKING',             (SELECT id FROM categories WHERE slug = 'transportation_parking')),
('plaid', 'TRANSPORTATION_PUBLIC_TRANSIT',      (SELECT id FROM categories WHERE slug = 'transportation_public_transit')),
('plaid', 'TRANSPORTATION_TAXIS_AND_RIDE_SHARES',(SELECT id FROM categories WHERE slug = 'transportation_taxis_and_ride_shares')),
('plaid', 'TRANSPORTATION_TOLLS',               (SELECT id FROM categories WHERE slug = 'transportation_tolls')),
('plaid', 'TRANSPORTATION_OTHER_TRANSPORTATION',(SELECT id FROM categories WHERE slug = 'transportation_other'))
ON CONFLICT (provider, provider_category) DO NOTHING;

-- Detailed mappings: TRAVEL
INSERT INTO category_mappings (provider, provider_category, category_id) VALUES
('plaid', 'TRAVEL_FLIGHTS',     (SELECT id FROM categories WHERE slug = 'travel_flights')),
('plaid', 'TRAVEL_LODGING',     (SELECT id FROM categories WHERE slug = 'travel_lodging')),
('plaid', 'TRAVEL_RENTAL_CARS', (SELECT id FROM categories WHERE slug = 'travel_rental_cars')),
('plaid', 'TRAVEL_OTHER_TRAVEL',(SELECT id FROM categories WHERE slug = 'travel_other'))
ON CONFLICT (provider, provider_category) DO NOTHING;

-- Detailed mappings: RENT_AND_UTILITIES
INSERT INTO category_mappings (provider, provider_category, category_id) VALUES
('plaid', 'RENT_AND_UTILITIES_GAS_AND_ELECTRICITY',        (SELECT id FROM categories WHERE slug = 'rent_and_utilities_gas_and_electricity')),
('plaid', 'RENT_AND_UTILITIES_INTERNET_AND_CABLE',         (SELECT id FROM categories WHERE slug = 'rent_and_utilities_internet_and_cable')),
('plaid', 'RENT_AND_UTILITIES_RENT',                       (SELECT id FROM categories WHERE slug = 'rent_and_utilities_rent')),
('plaid', 'RENT_AND_UTILITIES_SEWAGE_AND_WASTE_MANAGEMENT',(SELECT id FROM categories WHERE slug = 'rent_and_utilities_sewage_and_waste_management')),
('plaid', 'RENT_AND_UTILITIES_TELEPHONE',                  (SELECT id FROM categories WHERE slug = 'rent_and_utilities_telephone')),
('plaid', 'RENT_AND_UTILITIES_WATER',                      (SELECT id FROM categories WHERE slug = 'rent_and_utilities_water')),
('plaid', 'RENT_AND_UTILITIES_OTHER_UTILITIES',            (SELECT id FROM categories WHERE slug = 'rent_and_utilities_other'))
ON CONFLICT (provider, provider_category) DO NOTHING;

-- ==========================================
-- Teller provider mappings (27 raw categories)
-- ==========================================
INSERT INTO category_mappings (provider, provider_category, category_id) VALUES
('teller', 'accommodation',  (SELECT id FROM categories WHERE slug = 'travel_lodging')),
('teller', 'advertising',    (SELECT id FROM categories WHERE slug = 'general_services_other')),
('teller', 'bar',            (SELECT id FROM categories WHERE slug = 'food_and_drink_beer_wine_and_liquor')),
('teller', 'charity',        (SELECT id FROM categories WHERE slug = 'government_and_non_profit_donations')),
('teller', 'clothing',       (SELECT id FROM categories WHERE slug = 'general_merchandise_clothing_and_accessories')),
('teller', 'dining',         (SELECT id FROM categories WHERE slug = 'food_and_drink_restaurant')),
('teller', 'education',      (SELECT id FROM categories WHERE slug = 'general_services_education')),
('teller', 'electronics',    (SELECT id FROM categories WHERE slug = 'general_merchandise_electronics')),
('teller', 'entertainment',  (SELECT id FROM categories WHERE slug = 'entertainment_other')),
('teller', 'fuel',           (SELECT id FROM categories WHERE slug = 'transportation_gas')),
('teller', 'general',        (SELECT id FROM categories WHERE slug = 'general_merchandise_other')),
('teller', 'groceries',      (SELECT id FROM categories WHERE slug = 'food_and_drink_groceries')),
('teller', 'health',         (SELECT id FROM categories WHERE slug = 'medical_other')),
('teller', 'home',           (SELECT id FROM categories WHERE slug = 'home_improvement_other')),
('teller', 'income',         (SELECT id FROM categories WHERE slug = 'income_other')),
('teller', 'insurance',      (SELECT id FROM categories WHERE slug = 'loan_payments_insurance_payment')),
('teller', 'investment',     (SELECT id FROM categories WHERE slug = 'transfer_in_investment_and_retirement_funds')),
('teller', 'loan',           (SELECT id FROM categories WHERE slug = 'loan_payments_other')),
('teller', 'office',         (SELECT id FROM categories WHERE slug = 'general_services_other')),
('teller', 'phone',          (SELECT id FROM categories WHERE slug = 'rent_and_utilities_telephone')),
('teller', 'service',        (SELECT id FROM categories WHERE slug = 'general_services_other')),
('teller', 'shopping',       (SELECT id FROM categories WHERE slug = 'general_merchandise_other')),
('teller', 'software',       (SELECT id FROM categories WHERE slug = 'general_services_other')),
('teller', 'sport',          (SELECT id FROM categories WHERE slug = 'entertainment_sporting_events_amusement_parks_and_museums')),
('teller', 'tax',            (SELECT id FROM categories WHERE slug = 'government_and_non_profit_tax_payment')),
('teller', 'transport',      (SELECT id FROM categories WHERE slug = 'transportation_other')),
('teller', 'transportation', (SELECT id FROM categories WHERE slug = 'transportation_other')),
('teller', 'utilities',      (SELECT id FROM categories WHERE slug = 'rent_and_utilities_other'))
ON CONFLICT (provider, provider_category) DO NOTHING;

-- ==========================================
-- Backfill existing transactions
-- ==========================================

-- Pass 1: Plaid detailed category match
UPDATE transactions t
SET category_id = cm.category_id
FROM category_mappings cm
JOIN accounts a ON t.account_id = a.id
JOIN bank_connections c ON a.connection_id = c.id
WHERE c.provider = 'plaid'
  AND cm.provider = 'plaid'
  AND cm.provider_category = t.category_detailed
  AND t.category_id IS NULL
  AND t.deleted_at IS NULL;

-- Pass 2: Plaid primary-only fallback
UPDATE transactions t
SET category_id = cm.category_id
FROM category_mappings cm
JOIN accounts a ON t.account_id = a.id
JOIN bank_connections c ON a.connection_id = c.id
WHERE c.provider = 'plaid'
  AND cm.provider = 'plaid'
  AND cm.provider_category = t.category_primary
  AND t.category_id IS NULL
  AND t.deleted_at IS NULL;

-- Pass 3: Teller — match by raw Teller category via category_detailed reverse lookup.
-- Teller transactions store Plaid-translated strings in category_detailed.
-- For each Teller mapping, find transactions whose category_detailed matches the
-- Plaid translation of that Teller category.
UPDATE transactions t
SET category_id = cm.category_id
FROM category_mappings cm
JOIN accounts a ON t.account_id = a.id
JOIN bank_connections c ON a.connection_id = c.id
WHERE c.provider = 'teller'
  AND cm.provider = 'teller'
  AND t.category_id IS NULL
  AND t.deleted_at IS NULL
  AND (
    (cm.provider_category = 'accommodation'  AND t.category_detailed = 'TRAVEL_LODGING') OR
    (cm.provider_category = 'advertising'    AND t.category_detailed = 'GENERAL_SERVICES_OTHER_GENERAL_SERVICES' AND t.category_primary = 'GENERAL_SERVICES') OR
    (cm.provider_category = 'bar'            AND t.category_detailed = 'FOOD_AND_DRINK_BEER_WINE_AND_LIQUOR') OR
    (cm.provider_category = 'charity'        AND t.category_detailed = 'GOVERNMENT_AND_NON_PROFIT_DONATIONS') OR
    (cm.provider_category = 'clothing'       AND t.category_detailed = 'GENERAL_MERCHANDISE_CLOTHING_AND_ACCESSORIES') OR
    (cm.provider_category = 'dining'         AND t.category_detailed = 'FOOD_AND_DRINK_RESTAURANT') OR
    (cm.provider_category = 'education'      AND t.category_detailed = 'GENERAL_SERVICES_EDUCATION') OR
    (cm.provider_category = 'electronics'    AND t.category_detailed = 'GENERAL_MERCHANDISE_ELECTRONICS') OR
    (cm.provider_category = 'entertainment'  AND t.category_detailed = 'ENTERTAINMENT_OTHER_ENTERTAINMENT') OR
    (cm.provider_category = 'fuel'           AND t.category_detailed = 'TRANSPORTATION_GAS') OR
    (cm.provider_category = 'general'        AND t.category_detailed = 'GENERAL_MERCHANDISE_OTHER_GENERAL_MERCHANDISE') OR
    (cm.provider_category = 'groceries'      AND t.category_detailed = 'FOOD_AND_DRINK_GROCERIES') OR
    (cm.provider_category = 'health'         AND t.category_detailed = 'MEDICAL_OTHER_MEDICAL') OR
    (cm.provider_category = 'home'           AND t.category_detailed = 'HOME_IMPROVEMENT_OTHER_HOME_IMPROVEMENT') OR
    (cm.provider_category = 'income'         AND t.category_detailed = 'INCOME_OTHER_INCOME') OR
    (cm.provider_category = 'insurance'      AND t.category_detailed = 'LOAN_PAYMENTS_INSURANCE_PAYMENT') OR
    (cm.provider_category = 'investment'     AND t.category_detailed = 'TRANSFER_IN_INVESTMENT_AND_RETIREMENT_FUNDS') OR
    (cm.provider_category = 'loan'           AND t.category_detailed = 'LOAN_PAYMENTS_OTHER_PAYMENT') OR
    (cm.provider_category = 'office'         AND t.category_detailed = 'GENERAL_SERVICES_OTHER_GENERAL_SERVICES' AND t.category_primary = 'GENERAL_SERVICES') OR
    (cm.provider_category = 'phone'          AND t.category_detailed = 'RENT_AND_UTILITIES_TELEPHONE') OR
    (cm.provider_category = 'service'        AND t.category_detailed = 'GENERAL_SERVICES_OTHER_GENERAL_SERVICES' AND t.category_primary = 'GENERAL_SERVICES') OR
    (cm.provider_category = 'shopping'       AND t.category_detailed = 'GENERAL_MERCHANDISE_OTHER_GENERAL_MERCHANDISE') OR
    (cm.provider_category = 'software'       AND t.category_detailed = 'GENERAL_SERVICES_OTHER_GENERAL_SERVICES' AND t.category_primary = 'GENERAL_SERVICES') OR
    (cm.provider_category = 'sport'          AND t.category_detailed = 'ENTERTAINMENT_SPORTING_EVENTS_AMUSEMENT_PARKS_AND_MUSEUMS') OR
    (cm.provider_category = 'tax'            AND t.category_detailed = 'GOVERNMENT_AND_NON_PROFIT_TAX_PAYMENT') OR
    (cm.provider_category = 'transport'      AND t.category_detailed = 'TRANSPORTATION_OTHER_TRANSPORTATION' AND t.category_primary = 'TRANSPORTATION') OR
    (cm.provider_category = 'transportation' AND t.category_detailed = 'TRANSPORTATION_OTHER_TRANSPORTATION' AND t.category_primary = 'TRANSPORTATION') OR
    (cm.provider_category = 'utilities'      AND t.category_detailed = 'RENT_AND_UTILITIES_OTHER_UTILITIES')
  );

-- Pass 4: Remaining NULL → uncategorized
UPDATE transactions
SET category_id = (SELECT id FROM categories WHERE slug = 'uncategorized')
WHERE category_id IS NULL
  AND deleted_at IS NULL;

-- +goose Down
-- Clear backfilled category_id values
UPDATE transactions SET category_id = NULL, category_override = FALSE WHERE category_id IS NOT NULL;

-- Remove all seeded mappings and categories
DELETE FROM category_mappings;
DELETE FROM categories;
