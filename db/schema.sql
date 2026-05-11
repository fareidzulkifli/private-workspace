-- Database Schema for Personal Task Management App

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Organizations
CREATE TABLE IF NOT EXISTS organizations (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name TEXT NOT NULL,
    order_index FLOAT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Project Types Enum
DO $$ BEGIN
    CREATE TYPE project_type AS ENUM ('Work', 'Personal', 'Learning', 'Creative', 'Admin');
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

-- Projects
CREATE TABLE IF NOT EXISTS projects (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    order_index FLOAT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    description_markdown TEXT,
    goal TEXT,
    context_markdown TEXT,
    project_type project_type DEFAULT 'Work',
    ai_instructions TEXT,
    current_focus TEXT,
    target_date DATE,
    archived_at TIMESTAMP WITH TIME ZONE
);

ALTER TABLE projects ADD COLUMN IF NOT EXISTS archived_at TIMESTAMP WITH TIME ZONE;

-- Task Status Enum
DO $$ BEGIN
    CREATE TYPE task_status AS ENUM ('In Progress', 'Done', 'KIV');
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

-- Tasks
CREATE TABLE IF NOT EXISTS tasks (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    summary TEXT NOT NULL,
    notes_markdown TEXT,
    status task_status DEFAULT 'In Progress',
    urgent BOOLEAN DEFAULT FALSE,
    important BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    due_date DATE,
    order_index FLOAT NOT NULL,
    completed_at TIMESTAMP WITH TIME ZONE
);

ALTER TABLE tasks ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP;

-- Dashboard Events
CREATE TABLE IF NOT EXISTS dashboard_events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    title TEXT NOT NULL,
    event_date DATE NOT NULL,
    notes TEXT,
    color TEXT DEFAULT 'blue',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Prompt Templates
CREATE TABLE IF NOT EXISTS prompt_templates (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    org_id UUID REFERENCES organizations(id) ON DELETE SET NULL,
    title TEXT NOT NULL,
    description TEXT,
    category TEXT DEFAULT 'General',
    tags JSONB DEFAULT '[]'::jsonb,
    body TEXT NOT NULL,
    is_favorite BOOLEAN DEFAULT FALSE,
    archived_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Context Packs
CREATE TABLE IF NOT EXISTS context_packs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    org_id UUID REFERENCES organizations(id) ON DELETE SET NULL,
    title TEXT NOT NULL,
    description TEXT,
    tags JSONB DEFAULT '[]'::jsonb,
    archived_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Context Pack Items
CREATE TABLE IF NOT EXISTS context_pack_items (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    context_pack_id UUID NOT NULL REFERENCES context_packs(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    body TEXT NOT NULL,
    sort_order FLOAT NOT NULL DEFAULT 0,
    enabled_by_default BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Task Attachments
CREATE TABLE IF NOT EXISTS task_attachments (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    task_id UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    filename TEXT NOT NULL,
    r2_key TEXT NOT NULL,
    mime_type TEXT NOT NULL,
    size_bytes INTEGER NOT NULL,
    uploaded_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_projects_org_id ON projects(org_id);
CREATE INDEX IF NOT EXISTS idx_projects_archived_at ON projects(archived_at);
CREATE INDEX IF NOT EXISTS idx_tasks_project_id ON tasks(project_id);
CREATE INDEX IF NOT EXISTS idx_tasks_updated_at ON tasks(updated_at);
CREATE INDEX IF NOT EXISTS idx_dashboard_events_event_date ON dashboard_events(event_date);
CREATE INDEX IF NOT EXISTS idx_dashboard_events_updated_at ON dashboard_events(updated_at);
CREATE INDEX IF NOT EXISTS idx_prompt_templates_archived_at ON prompt_templates(archived_at);
CREATE INDEX IF NOT EXISTS idx_prompt_templates_updated_at ON prompt_templates(updated_at);
CREATE INDEX IF NOT EXISTS idx_context_packs_archived_at ON context_packs(archived_at);
CREATE INDEX IF NOT EXISTS idx_context_packs_updated_at ON context_packs(updated_at);
CREATE INDEX IF NOT EXISTS idx_context_pack_items_pack_id ON context_pack_items(context_pack_id);
CREATE INDEX IF NOT EXISTS idx_task_attachments_task_id ON task_attachments(task_id);
