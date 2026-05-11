-- Secure Row Level Security (RLS) Policies
-- These policies restrict access to authenticated users only.

-- 1. Reset existing policies to avoid duplicates
DROP POLICY IF EXISTS "Allow all for organizations" ON organizations;
DROP POLICY IF EXISTS "Allow all for projects" ON projects;
DROP POLICY IF EXISTS "Allow all for tasks" ON tasks;
DROP POLICY IF EXISTS "Allow all for task_attachments" ON task_attachments;
DROP POLICY IF EXISTS "Allow all for prompt_templates" ON prompt_templates;
DROP POLICY IF EXISTS "Allow all for context_packs" ON context_packs;
DROP POLICY IF EXISTS "Allow all for context_pack_items" ON context_pack_items;
DROP POLICY IF EXISTS "Anon access for organizations" ON organizations;
DROP POLICY IF EXISTS "Anon access for projects" ON projects;
DROP POLICY IF EXISTS "Anon access for tasks" ON tasks;
DROP POLICY IF EXISTS "Anon access for task_attachments" ON task_attachments;
DROP POLICY IF EXISTS "Anon access for prompt_templates" ON prompt_templates;
DROP POLICY IF EXISTS "Anon access for context_packs" ON context_packs;
DROP POLICY IF EXISTS "Anon access for context_pack_items" ON context_pack_items;
DROP POLICY IF EXISTS "Authenticated users can manage organizations" ON organizations;
DROP POLICY IF EXISTS "Authenticated users can manage projects" ON projects;
DROP POLICY IF EXISTS "Authenticated users can manage tasks" ON tasks;
DROP POLICY IF EXISTS "Authenticated users can manage task_attachments" ON task_attachments;
DROP POLICY IF EXISTS "Authenticated users can manage prompt_templates" ON prompt_templates;
DROP POLICY IF EXISTS "Authenticated users can manage context_packs" ON context_packs;
DROP POLICY IF EXISTS "Authenticated users can manage context_pack_items" ON context_pack_items;

-- 2. Organizations
CREATE POLICY "Authenticated users can manage organizations" ON organizations
FOR ALL TO authenticated USING (true) WITH CHECK (true);

-- 3. Projects
CREATE POLICY "Authenticated users can manage projects" ON projects
FOR ALL TO authenticated USING (true) WITH CHECK (true);

-- 4. Tasks
CREATE POLICY "Authenticated users can manage tasks" ON tasks
FOR ALL TO authenticated USING (true) WITH CHECK (true);

-- 5. Task Attachments
CREATE POLICY "Authenticated users can manage task_attachments" ON task_attachments
FOR ALL TO authenticated USING (true) WITH CHECK (true);

-- 6. Prompt Templates
CREATE POLICY "Authenticated users can manage prompt_templates" ON prompt_templates
FOR ALL TO authenticated USING (true) WITH CHECK (true);

-- 7. Context Packs
CREATE POLICY "Authenticated users can manage context_packs" ON context_packs
FOR ALL TO authenticated USING (true) WITH CHECK (true);

-- 8. Context Pack Items
CREATE POLICY "Authenticated users can manage context_pack_items" ON context_pack_items
FOR ALL TO authenticated USING (true) WITH CHECK (true);
