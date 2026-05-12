PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS wallet_months (
  id TEXT PRIMARY KEY,
  month TEXT NOT NULL UNIQUE,
  opening_balance_cents INTEGER NOT NULL DEFAULT 0,
  wallet_balance_cents INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'open'
    CHECK (status IN ('open', 'closed')),
  closed_at TEXT,
  closed_wallet_balance_cents INTEGER,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS wallet_categories (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  system_key TEXT UNIQUE,
  active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS wallet_allocation_templates (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  default_amount_cents INTEGER NOT NULL DEFAULT 0,
  type TEXT NOT NULL DEFAULT 'flexible'
    CHECK (type IN ('fixed', 'flexible', 'sinking_fund', 'one_off', 'subscription')),
  carry_forward INTEGER NOT NULL DEFAULT 0 CHECK (carry_forward IN (0, 1)),
  active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS wallet_income_templates (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  default_amount_cents INTEGER NOT NULL DEFAULT 0,
  default_day INTEGER CHECK (default_day IS NULL OR (default_day >= 1 AND default_day <= 31)),
  active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS wallet_income_items (
  id TEXT PRIMARY KEY,
  month_id TEXT NOT NULL REFERENCES wallet_months(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  amount_cents INTEGER NOT NULL CHECK (amount_cents >= 0),
  received_date TEXT,
  applies_to_month TEXT NOT NULL,
  notes TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS wallet_allocations (
  id TEXT PRIMARY KEY,
  month_id TEXT NOT NULL REFERENCES wallet_months(id) ON DELETE CASCADE,
  template_id TEXT REFERENCES wallet_allocation_templates(id) ON DELETE SET NULL,
  name TEXT NOT NULL,
  budgeted_cents INTEGER NOT NULL DEFAULT 0 CHECK (budgeted_cents >= 0),
  type TEXT NOT NULL DEFAULT 'flexible'
    CHECK (type IN ('fixed', 'flexible', 'sinking_fund', 'one_off', 'subscription')),
  carry_forward INTEGER NOT NULL DEFAULT 0 CHECK (carry_forward IN (0, 1)),
  sort_order INTEGER NOT NULL DEFAULT 0,
  active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS wallet_transactions (
  id TEXT PRIMARY KEY,
  month_id TEXT NOT NULL REFERENCES wallet_months(id) ON DELETE CASCADE,
  allocation_id TEXT NOT NULL REFERENCES wallet_allocations(id) ON DELETE CASCADE,
  category_id TEXT NOT NULL REFERENCES wallet_categories(id) ON DELETE RESTRICT,
  date TEXT NOT NULL,
  amount_cents INTEGER NOT NULL CHECK (amount_cents > 0),
  note TEXT,
  rounded INTEGER NOT NULL DEFAULT 0 CHECK (rounded IN (0, 1)),
  kind TEXT NOT NULL DEFAULT 'spend'
    CHECK (kind IN ('spend', 'adjustment', 'transfer')),
  source TEXT NOT NULL DEFAULT 'manual'
    CHECK (source IN ('manual', 'reconciliation', 'split', 'system')),
  parent_transaction_id TEXT REFERENCES wallet_transactions(id) ON DELETE SET NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS wallet_transaction_splits (
  id TEXT PRIMARY KEY,
  parent_transaction_id TEXT NOT NULL REFERENCES wallet_transactions(id) ON DELETE CASCADE,
  child_transaction_id TEXT NOT NULL REFERENCES wallet_transactions(id) ON DELETE CASCADE,
  created_at TEXT NOT NULL,
  UNIQUE (child_transaction_id)
);

CREATE TABLE IF NOT EXISTS wallet_balance_updates (
  id TEXT PRIMARY KEY,
  month_id TEXT NOT NULL REFERENCES wallet_months(id) ON DELETE CASCADE,
  previous_balance_cents INTEGER NOT NULL,
  new_balance_cents INTEGER NOT NULL,
  expected_balance_cents INTEGER NOT NULL,
  difference_cents INTEGER NOT NULL,
  note TEXT,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS wallet_reconciliation_adjustments (
  id TEXT PRIMARY KEY,
  month_id TEXT NOT NULL REFERENCES wallet_months(id) ON DELETE CASCADE,
  balance_update_id TEXT REFERENCES wallet_balance_updates(id) ON DELETE SET NULL,
  amount_cents INTEGER NOT NULL,
  reason TEXT NOT NULL
    CHECK (reason IN ('rounding', 'missed_transaction', 'cash_variance', 'manual_correction')),
  note TEXT,
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_wallet_months_month ON wallet_months(month);
CREATE INDEX IF NOT EXISTS idx_wallet_income_items_month_id ON wallet_income_items(month_id);
CREATE INDEX IF NOT EXISTS idx_wallet_allocations_month_id ON wallet_allocations(month_id, active, sort_order);
CREATE INDEX IF NOT EXISTS idx_wallet_transactions_month_id ON wallet_transactions(month_id, date);
CREATE INDEX IF NOT EXISTS idx_wallet_transactions_allocation_id ON wallet_transactions(allocation_id);
CREATE INDEX IF NOT EXISTS idx_wallet_transactions_category_id ON wallet_transactions(category_id);
CREATE INDEX IF NOT EXISTS idx_wallet_transactions_review ON wallet_transactions(month_id, category_id, rounded);
CREATE INDEX IF NOT EXISTS idx_wallet_balance_updates_month_id ON wallet_balance_updates(month_id, created_at);
CREATE INDEX IF NOT EXISTS idx_wallet_reconciliation_adjustments_month_id ON wallet_reconciliation_adjustments(month_id);
CREATE INDEX IF NOT EXISTS idx_wallet_allocation_templates_active ON wallet_allocation_templates(active, sort_order);
CREATE INDEX IF NOT EXISTS idx_wallet_income_templates_active ON wallet_income_templates(active, sort_order);
CREATE INDEX IF NOT EXISTS idx_wallet_categories_active ON wallet_categories(active, sort_order);

INSERT OR IGNORE INTO wallet_categories (id, name, system_key, active, sort_order, created_at, updated_at) VALUES
  ('wallet-category-unsorted', 'Unsorted', 'unsorted', 1, 0, '1970-01-01T00:00:00Z', '1970-01-01T00:00:00Z'),
  ('wallet-category-food', 'Food', NULL, 1, 10, '1970-01-01T00:00:00Z', '1970-01-01T00:00:00Z'),
  ('wallet-category-transport', 'Transport', NULL, 1, 20, '1970-01-01T00:00:00Z', '1970-01-01T00:00:00Z'),
  ('wallet-category-groceries', 'Groceries', NULL, 1, 30, '1970-01-01T00:00:00Z', '1970-01-01T00:00:00Z'),
  ('wallet-category-bills', 'Bills', NULL, 1, 40, '1970-01-01T00:00:00Z', '1970-01-01T00:00:00Z'),
  ('wallet-category-shopping', 'Shopping', NULL, 1, 50, '1970-01-01T00:00:00Z', '1970-01-01T00:00:00Z'),
  ('wallet-category-family', 'Family', NULL, 1, 60, '1970-01-01T00:00:00Z', '1970-01-01T00:00:00Z'),
  ('wallet-category-health', 'Health', NULL, 1, 70, '1970-01-01T00:00:00Z', '1970-01-01T00:00:00Z'),
  ('wallet-category-misc', 'Misc', NULL, 1, 80, '1970-01-01T00:00:00Z', '1970-01-01T00:00:00Z'),
  ('wallet-category-lunch', 'Lunch', NULL, 1, 90, '1970-01-01T00:00:00Z', '1970-01-01T00:00:00Z'),
  ('wallet-category-grab', 'Grab', NULL, 1, 100, '1970-01-01T00:00:00Z', '1970-01-01T00:00:00Z'),
  ('wallet-category-coffee', 'Coffee', NULL, 1, 110, '1970-01-01T00:00:00Z', '1970-01-01T00:00:00Z'),
  ('wallet-category-parking', 'Parking', NULL, 1, 120, '1970-01-01T00:00:00Z', '1970-01-01T00:00:00Z'),
  ('wallet-category-office-supplies', 'Office Supplies', NULL, 1, 130, '1970-01-01T00:00:00Z', '1970-01-01T00:00:00Z'),
  ('wallet-category-office-misc', 'Office Misc', NULL, 1, 140, '1970-01-01T00:00:00Z', '1970-01-01T00:00:00Z');
