PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS wallet_allocation_template_categories (
  template_id TEXT NOT NULL REFERENCES wallet_allocation_templates(id) ON DELETE CASCADE,
  category_id TEXT NOT NULL REFERENCES wallet_categories(id) ON DELETE CASCADE,
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  PRIMARY KEY (template_id, category_id)
);

CREATE TABLE IF NOT EXISTS wallet_allocation_default_categories (
  allocation_id TEXT NOT NULL REFERENCES wallet_allocations(id) ON DELETE CASCADE,
  category_id TEXT NOT NULL REFERENCES wallet_categories(id) ON DELETE CASCADE,
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  PRIMARY KEY (allocation_id, category_id)
);

CREATE INDEX IF NOT EXISTS idx_wallet_allocation_template_categories_template
  ON wallet_allocation_template_categories(template_id, sort_order);
CREATE INDEX IF NOT EXISTS idx_wallet_allocation_template_categories_category
  ON wallet_allocation_template_categories(category_id);
CREATE INDEX IF NOT EXISTS idx_wallet_allocation_default_categories_allocation
  ON wallet_allocation_default_categories(allocation_id, sort_order);
CREATE INDEX IF NOT EXISTS idx_wallet_allocation_default_categories_category
  ON wallet_allocation_default_categories(category_id);

INSERT OR IGNORE INTO wallet_allocation_template_categories
  (template_id, category_id, sort_order, created_at)
VALUES
  ('wallet-template-work-expense', 'wallet-category-office-supplies', 10, '1970-01-01T00:00:00Z'),
  ('wallet-template-work-expense', 'wallet-category-parking', 20, '1970-01-01T00:00:00Z'),
  ('wallet-template-work-expense', 'wallet-category-coffee', 30, '1970-01-01T00:00:00Z'),
  ('wallet-template-work-expense', 'wallet-category-office-misc', 40, '1970-01-01T00:00:00Z'),
  ('wallet-template-food', 'wallet-category-food', 10, '1970-01-01T00:00:00Z'),
  ('wallet-template-food', 'wallet-category-groceries', 20, '1970-01-01T00:00:00Z'),
  ('wallet-template-food', 'wallet-category-lunch', 30, '1970-01-01T00:00:00Z'),
  ('wallet-template-transport', 'wallet-category-transport', 10, '1970-01-01T00:00:00Z'),
  ('wallet-template-transport', 'wallet-category-grab', 20, '1970-01-01T00:00:00Z'),
  ('wallet-template-bills', 'wallet-category-bills', 10, '1970-01-01T00:00:00Z'),
  ('wallet-template-family', 'wallet-category-family', 10, '1970-01-01T00:00:00Z'),
  ('wallet-template-misc', 'wallet-category-misc', 10, '1970-01-01T00:00:00Z');

INSERT OR IGNORE INTO wallet_allocation_default_categories
  (allocation_id, category_id, sort_order, created_at)
SELECT a.id, tc.category_id, tc.sort_order, '1970-01-01T00:00:00Z'
FROM wallet_allocations a
JOIN wallet_allocation_template_categories tc ON tc.template_id = a.template_id;
