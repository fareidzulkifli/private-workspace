PRAGMA foreign_keys = ON;

INSERT OR IGNORE INTO wallet_allocation_templates
  (id, name, default_amount_cents, type, carry_forward, active, sort_order, created_at, updated_at)
VALUES
  ('wallet-template-food', 'Food', 0, 'flexible', 0, 1, 10, '1970-01-01T00:00:00Z', '1970-01-01T00:00:00Z'),
  ('wallet-template-work-expense', 'Work Expense', 0, 'flexible', 0, 1, 20, '1970-01-01T00:00:00Z', '1970-01-01T00:00:00Z'),
  ('wallet-template-transport', 'Transport', 0, 'flexible', 0, 1, 30, '1970-01-01T00:00:00Z', '1970-01-01T00:00:00Z'),
  ('wallet-template-bills', 'Bills', 0, 'fixed', 0, 1, 40, '1970-01-01T00:00:00Z', '1970-01-01T00:00:00Z'),
  ('wallet-template-family', 'Family', 0, 'flexible', 0, 1, 50, '1970-01-01T00:00:00Z', '1970-01-01T00:00:00Z'),
  ('wallet-template-savings', 'Savings', 0, 'sinking_fund', 1, 1, 60, '1970-01-01T00:00:00Z', '1970-01-01T00:00:00Z'),
  ('wallet-template-emergency-fund', 'Emergency Fund', 0, 'sinking_fund', 1, 1, 70, '1970-01-01T00:00:00Z', '1970-01-01T00:00:00Z'),
  ('wallet-template-misc', 'Misc', 0, 'flexible', 0, 1, 80, '1970-01-01T00:00:00Z', '1970-01-01T00:00:00Z');

INSERT OR IGNORE INTO wallet_income_templates
  (id, name, default_amount_cents, default_day, active, sort_order, created_at, updated_at)
VALUES
  ('wallet-income-template-salary', 'Salary', 0, NULL, 1, 10, '1970-01-01T00:00:00Z', '1970-01-01T00:00:00Z'),
  ('wallet-income-template-freelance', 'Freelance', 0, NULL, 0, 20, '1970-01-01T00:00:00Z', '1970-01-01T00:00:00Z'),
  ('wallet-income-template-bonus', 'Bonus', 0, NULL, 0, 30, '1970-01-01T00:00:00Z', '1970-01-01T00:00:00Z');
