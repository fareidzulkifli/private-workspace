PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS gitnote_shares (
  id TEXT PRIMARY KEY,
  token_hash TEXT NOT NULL UNIQUE,
  path_prefix TEXT NOT NULL,
  title TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  revoked_at TEXT,
  expires_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_gitnote_shares_token_hash ON gitnote_shares(token_hash);
CREATE INDEX IF NOT EXISTS idx_gitnote_shares_revoked_at ON gitnote_shares(revoked_at);
CREATE INDEX IF NOT EXISTS idx_gitnote_shares_expires_at ON gitnote_shares(expires_at);
