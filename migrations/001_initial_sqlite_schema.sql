PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS schema_migrations (
  version TEXT PRIMARY KEY,
  applied_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS admin_users (
  id TEXT PRIMARY KEY,
  email TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES admin_users(id) ON DELETE CASCADE,
  token_hash TEXT NOT NULL UNIQUE,
  csrf_secret TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL,
  last_seen_at TEXT,
  user_agent TEXT,
  ip_address TEXT
);

CREATE TABLE IF NOT EXISTS organizations (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  slug TEXT NOT NULL UNIQUE,
  order_index REAL NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS projects (
  id TEXT PRIMARY KEY,
  org_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  order_index REAL NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  description_markdown TEXT,
  goal TEXT,
  context_markdown TEXT,
  project_type TEXT NOT NULL DEFAULT 'Work'
    CHECK (project_type IN ('Work', 'Personal', 'Learning', 'Creative', 'Admin')),
  ai_instructions TEXT,
  current_focus TEXT,
  target_date TEXT,
  archived_at TEXT
);

CREATE TABLE IF NOT EXISTS tasks (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  summary TEXT NOT NULL,
  notes_markdown TEXT,
  status TEXT NOT NULL DEFAULT 'In Progress'
    CHECK (status IN ('In Progress', 'Done', 'KIV')),
  urgent INTEGER NOT NULL DEFAULT 0 CHECK (urgent IN (0, 1)),
  important INTEGER NOT NULL DEFAULT 0 CHECK (important IN (0, 1)),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  due_date TEXT,
  order_index REAL NOT NULL DEFAULT 0,
  completed_at TEXT
);

CREATE TABLE IF NOT EXISTS dashboard_events (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  event_date TEXT NOT NULL,
  notes TEXT,
  color TEXT NOT NULL DEFAULT 'blue'
    CHECK (color IN ('blue', 'green', 'red', 'violet', 'slate')),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS prompt_templates (
  id TEXT PRIMARY KEY,
  org_id TEXT REFERENCES organizations(id) ON DELETE SET NULL,
  title TEXT NOT NULL,
  description TEXT,
  category TEXT NOT NULL DEFAULT 'General',
  tags TEXT NOT NULL DEFAULT '[]',
  body TEXT NOT NULL,
  is_favorite INTEGER NOT NULL DEFAULT 0 CHECK (is_favorite IN (0, 1)),
  archived_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  CHECK (json_valid(tags))
);

CREATE TABLE IF NOT EXISTS context_packs (
  id TEXT PRIMARY KEY,
  org_id TEXT REFERENCES organizations(id) ON DELETE SET NULL,
  title TEXT NOT NULL,
  description TEXT,
  tags TEXT NOT NULL DEFAULT '[]',
  archived_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  CHECK (json_valid(tags))
);

CREATE TABLE IF NOT EXISTS context_pack_items (
  id TEXT PRIMARY KEY,
  context_pack_id TEXT NOT NULL REFERENCES context_packs(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  body TEXT NOT NULL,
  sort_order REAL NOT NULL DEFAULT 0,
  enabled_by_default INTEGER NOT NULL DEFAULT 1 CHECK (enabled_by_default IN (0, 1)),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS task_attachments (
  id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  filename TEXT NOT NULL,
  r2_key TEXT NOT NULL,
  mime_type TEXT NOT NULL,
  size_bytes INTEGER NOT NULL,
  uploaded_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_organizations_order ON organizations(order_index);
CREATE INDEX IF NOT EXISTS idx_projects_org_id ON projects(org_id);
CREATE INDEX IF NOT EXISTS idx_projects_archived_at ON projects(archived_at);
CREATE INDEX IF NOT EXISTS idx_projects_order ON projects(org_id, archived_at, order_index);
CREATE INDEX IF NOT EXISTS idx_tasks_project_id ON tasks(project_id);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
CREATE INDEX IF NOT EXISTS idx_tasks_due_date ON tasks(due_date);
CREATE INDEX IF NOT EXISTS idx_tasks_updated_at ON tasks(updated_at);
CREATE INDEX IF NOT EXISTS idx_tasks_order ON tasks(project_id, status, order_index);
CREATE INDEX IF NOT EXISTS idx_dashboard_events_event_date ON dashboard_events(event_date);
CREATE INDEX IF NOT EXISTS idx_dashboard_events_updated_at ON dashboard_events(updated_at);
CREATE INDEX IF NOT EXISTS idx_prompt_templates_archived_at ON prompt_templates(archived_at);
CREATE INDEX IF NOT EXISTS idx_prompt_templates_updated_at ON prompt_templates(updated_at);
CREATE INDEX IF NOT EXISTS idx_context_packs_archived_at ON context_packs(archived_at);
CREATE INDEX IF NOT EXISTS idx_context_packs_updated_at ON context_packs(updated_at);
CREATE INDEX IF NOT EXISTS idx_context_pack_items_pack_id ON context_pack_items(context_pack_id, sort_order);
CREATE INDEX IF NOT EXISTS idx_task_attachments_task_id ON task_attachments(task_id);
CREATE INDEX IF NOT EXISTS idx_sessions_token_hash ON sessions(token_hash);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);
