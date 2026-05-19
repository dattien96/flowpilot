CREATE TABLE IF NOT EXISTS teams (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE IF NOT EXISTS project_teams (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
  UNIQUE(project_id, team_id)
);

CREATE TABLE IF NOT EXISTS team_members (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  email TEXT,
  jira_account_id TEXT,
  role TEXT NOT NULL,
  level_label TEXT NOT NULL,
  skill_tags JSONB DEFAULT '[]'::jsonb,
  weekly_capacity_hours INT DEFAULT 40,
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now()
);

ALTER TABLE projects
  ADD COLUMN IF NOT EXISTS directory_path TEXT,
  ADD COLUMN IF NOT EXISTS owner_id TEXT,
  ADD COLUMN IF NOT EXISTS status TEXT DEFAULT 'active',
  ADD COLUMN IF NOT EXISTS artifact_storage_preference TEXT DEFAULT 'supabase';

ALTER TABLE teams ENABLE ROW LEVEL SECURITY;
ALTER TABLE project_teams ENABLE ROW LEVEL SECURITY;
ALTER TABLE team_members ENABLE ROW LEVEL SECURITY;

CREATE POLICY "teams_select_all" ON teams FOR SELECT USING (true);
CREATE POLICY "teams_insert_all" ON teams FOR INSERT WITH CHECK (true);
CREATE POLICY "teams_update_all" ON teams FOR UPDATE USING (true) WITH CHECK (true);
CREATE POLICY "teams_delete_all" ON teams FOR DELETE USING (true);

CREATE POLICY "project_teams_select_all" ON project_teams FOR SELECT USING (true);
CREATE POLICY "project_teams_insert_all" ON project_teams FOR INSERT WITH CHECK (true);
CREATE POLICY "project_teams_delete_all" ON project_teams FOR DELETE USING (true);

CREATE POLICY "team_members_select_all" ON team_members FOR SELECT USING (true);
CREATE POLICY "team_members_insert_all" ON team_members FOR INSERT WITH CHECK (true);
CREATE POLICY "team_members_update_all" ON team_members FOR UPDATE USING (true) WITH CHECK (true);
CREATE POLICY "team_members_delete_all" ON team_members FOR DELETE USING (true);
