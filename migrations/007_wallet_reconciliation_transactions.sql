PRAGMA foreign_keys = ON;

CREATE TEMP TABLE wallet_transactions_backup AS
SELECT * FROM wallet_transactions;

CREATE TEMP TABLE wallet_transaction_splits_backup AS
SELECT * FROM wallet_transaction_splits;

DROP TABLE wallet_transaction_splits;
DROP TABLE wallet_transactions;

CREATE TABLE wallet_transactions (
  id TEXT PRIMARY KEY,
  month_id TEXT NOT NULL REFERENCES wallet_months(id) ON DELETE CASCADE,
  allocation_id TEXT NOT NULL REFERENCES wallet_allocations(id) ON DELETE CASCADE,
  category_id TEXT NOT NULL REFERENCES wallet_categories(id) ON DELETE RESTRICT,
  date TEXT NOT NULL,
  amount_cents INTEGER NOT NULL CHECK (amount_cents > 0),
  note TEXT,
  rounded INTEGER NOT NULL DEFAULT 0 CHECK (rounded IN (0, 1)),
  kind TEXT NOT NULL DEFAULT 'spend'
    CHECK (kind IN ('spend', 'income', 'adjustment', 'transfer')),
  source TEXT NOT NULL DEFAULT 'manual'
    CHECK (source IN ('manual', 'reconciliation', 'split', 'system')),
  parent_transaction_id TEXT REFERENCES wallet_transactions(id) ON DELETE SET NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE wallet_transaction_splits (
  id TEXT PRIMARY KEY,
  parent_transaction_id TEXT NOT NULL REFERENCES wallet_transactions(id) ON DELETE CASCADE,
  child_transaction_id TEXT NOT NULL REFERENCES wallet_transactions(id) ON DELETE CASCADE,
  created_at TEXT NOT NULL,
  UNIQUE (child_transaction_id)
);

INSERT INTO wallet_transactions
  (id, month_id, allocation_id, category_id, date, amount_cents, note, rounded, kind, source, parent_transaction_id, created_at, updated_at)
SELECT id, month_id, allocation_id, category_id, date, amount_cents, note, rounded, kind, source, parent_transaction_id, created_at, updated_at
FROM wallet_transactions_backup;

INSERT INTO wallet_transaction_splits
  (id, parent_transaction_id, child_transaction_id, created_at)
SELECT id, parent_transaction_id, child_transaction_id, created_at
FROM wallet_transaction_splits_backup;

CREATE INDEX idx_wallet_transactions_month_id ON wallet_transactions(month_id, date);
CREATE INDEX idx_wallet_transactions_allocation_id ON wallet_transactions(allocation_id);
CREATE INDEX idx_wallet_transactions_category_id ON wallet_transactions(category_id);
CREATE INDEX idx_wallet_transactions_review ON wallet_transactions(month_id, category_id, rounded);

INSERT OR IGNORE INTO wallet_categories
  (id, name, system_key, active, sort_order, created_at, updated_at)
VALUES
  ('wallet-category-reconciliation-adjustment', 'Adjustment', 'reconciliation_adjustment', 1, 1000000, '1970-01-01T00:00:00Z', '1970-01-01T00:00:00Z');

UPDATE wallet_categories
SET system_key = 'reconciliation_adjustment',
    active = 1,
    sort_order = 1000000,
    updated_at = '1970-01-01T00:00:00Z'
WHERE name = 'Adjustment';

DROP TABLE wallet_transactions_backup;
DROP TABLE wallet_transaction_splits_backup;
