PRAGMA foreign_keys = ON;

CREATE TEMP TABLE wallet_allocation_templates_backup AS
SELECT * FROM wallet_allocation_templates;

CREATE TEMP TABLE wallet_allocations_backup AS
SELECT * FROM wallet_allocations;

CREATE TEMP TABLE wallet_transactions_backup AS
SELECT * FROM wallet_transactions;

CREATE TEMP TABLE wallet_transaction_splits_backup AS
SELECT * FROM wallet_transaction_splits;

CREATE TEMP TABLE wallet_allocation_template_categories_backup AS
SELECT * FROM wallet_allocation_template_categories;

CREATE TEMP TABLE wallet_allocation_default_categories_backup AS
SELECT * FROM wallet_allocation_default_categories;

DROP TABLE wallet_transaction_splits;
DROP TABLE wallet_allocation_default_categories;
DROP TABLE wallet_transactions;
DROP TABLE wallet_allocation_template_categories;
DROP TABLE wallet_allocations;
DROP TABLE wallet_allocation_templates;

CREATE TABLE wallet_allocation_templates (
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

CREATE TABLE wallet_allocations (
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
    CHECK (kind IN ('spend', 'adjustment', 'transfer')),
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

CREATE TABLE wallet_allocation_template_categories (
  template_id TEXT NOT NULL REFERENCES wallet_allocation_templates(id) ON DELETE CASCADE,
  category_id TEXT NOT NULL REFERENCES wallet_categories(id) ON DELETE CASCADE,
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  PRIMARY KEY (template_id, category_id)
);

CREATE TABLE wallet_allocation_default_categories (
  allocation_id TEXT NOT NULL REFERENCES wallet_allocations(id) ON DELETE CASCADE,
  category_id TEXT NOT NULL REFERENCES wallet_categories(id) ON DELETE CASCADE,
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  PRIMARY KEY (allocation_id, category_id)
);

INSERT INTO wallet_allocation_templates
  (id, name, default_amount_cents, type, carry_forward, active, sort_order, created_at, updated_at)
SELECT id, name, default_amount_cents, type, carry_forward, active, sort_order, created_at, updated_at
FROM wallet_allocation_templates_backup;

INSERT INTO wallet_allocations
  (id, month_id, template_id, name, budgeted_cents, type, carry_forward, sort_order, active, created_at, updated_at)
SELECT id, month_id, template_id, name, budgeted_cents, type, carry_forward, sort_order, active, created_at, updated_at
FROM wallet_allocations_backup;

INSERT INTO wallet_transactions
  (id, month_id, allocation_id, category_id, date, amount_cents, note, rounded, kind, source, parent_transaction_id, created_at, updated_at)
SELECT id, month_id, allocation_id, category_id, date, amount_cents, note, rounded, kind, source, parent_transaction_id, created_at, updated_at
FROM wallet_transactions_backup;

INSERT INTO wallet_transaction_splits
  (id, parent_transaction_id, child_transaction_id, created_at)
SELECT id, parent_transaction_id, child_transaction_id, created_at
FROM wallet_transaction_splits_backup;

INSERT INTO wallet_allocation_template_categories
  (template_id, category_id, sort_order, created_at)
SELECT template_id, category_id, sort_order, created_at
FROM wallet_allocation_template_categories_backup;

INSERT INTO wallet_allocation_default_categories
  (allocation_id, category_id, sort_order, created_at)
SELECT allocation_id, category_id, sort_order, created_at
FROM wallet_allocation_default_categories_backup;

CREATE INDEX idx_wallet_allocations_month_id ON wallet_allocations(month_id, active, sort_order);
CREATE INDEX idx_wallet_transactions_month_id ON wallet_transactions(month_id, date);
CREATE INDEX idx_wallet_transactions_allocation_id ON wallet_transactions(allocation_id);
CREATE INDEX idx_wallet_transactions_category_id ON wallet_transactions(category_id);
CREATE INDEX idx_wallet_allocation_template_categories_template
  ON wallet_allocation_template_categories(template_id, sort_order);
CREATE INDEX idx_wallet_allocation_template_categories_category
  ON wallet_allocation_template_categories(category_id);
CREATE INDEX idx_wallet_allocation_default_categories_allocation
  ON wallet_allocation_default_categories(allocation_id, sort_order);
CREATE INDEX idx_wallet_allocation_default_categories_category
  ON wallet_allocation_default_categories(category_id);

DROP TABLE wallet_allocation_templates_backup;
DROP TABLE wallet_allocations_backup;
DROP TABLE wallet_transactions_backup;
DROP TABLE wallet_transaction_splits_backup;
DROP TABLE wallet_allocation_template_categories_backup;
DROP TABLE wallet_allocation_default_categories_backup;
