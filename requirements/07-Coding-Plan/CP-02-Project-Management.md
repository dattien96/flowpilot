# CP-02: Project & Team Management

**Maps from:** SD-04 (Project Management), SS-01, SS-02, SS-03
**Phase:** 2 (after foundation setup)
**Depends on:** CP-01

---

## 1. Database Tables

### 1.1 Existing Tables (from migration `20260515050000`)
- `projects` — id, name, description, platform, repository_url, created_by, timestamps
- `features` — linked to project, contains business goal / acceptance criteria

### 1.2 New Tables Required (new migration)

```sql
-- Teams
CREATE TABLE teams (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now()
);

-- Project ↔ Team join table (N:N per SD-04)
CREATE TABLE project_teams (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
  UNIQUE(project_id, team_id)
);

-- Team members with level labels
CREATE TABLE team_members (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  email TEXT,
  jira_account_id TEXT,
  role TEXT NOT NULL,              -- android, ios, backend, frontend, qa, devops, ai_workflow
  level_label TEXT NOT NULL,       -- L1_intern, L2_junior, L3_middle, L4_senior, L5_lead
  skill_tags JSONB DEFAULT '[]',
  weekly_capacity_hours INT DEFAULT 40,
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now()
);

-- Note: MCP/Integration config is handled by the `integrations` table defined in CP-07.
-- This avoids duplication. The `integrations` table stores:
--   project_id, type (jira/figma/google_drive/firebase/telegram),
--   config_encrypted (JSONB), status (pending/connected/failed),
--   last_synced_at, timestamps.
-- See CP-07-Integrations-Hardening.md §2.1 for full schema.

-- Alter existing projects table to add directory_path, owner_id, and storage preference
ALTER TABLE projects ADD COLUMN IF NOT EXISTS directory_path TEXT;
ALTER TABLE projects ADD COLUMN IF NOT EXISTS owner_id TEXT;
ALTER TABLE projects ADD COLUMN IF NOT EXISTS status TEXT DEFAULT 'active';
ALTER TABLE projects ADD COLUMN IF NOT EXISTS artifact_storage_preference TEXT DEFAULT 'supabase'; -- 'supabase' or 'google_drive'

-- RLS
ALTER TABLE teams ENABLE ROW LEVEL SECURITY;
ALTER TABLE project_teams ENABLE ROW LEVEL SECURITY;
ALTER TABLE team_members ENABLE ROW LEVEL SECURITY;
```

---

## 2. Domain Model Updates

### 2.1 New Entities

```typescript
// src/domain/model/entity/team.ts
export interface Team {
  id: string;
  name: string;
  createdAt: string;
  updatedAt: string;
}

export interface TeamMember {
  id: string;
  teamId: string;
  name: string;
  email: string | null;
  jiraAccountId: string | null;
  role: MemberRole;
  levelLabel: LevelLabel;
  skillTags: string[];
  weeklyCapacityHours: number;
  createdAt: string;
  updatedAt: string;
}

export type MemberRole = 'android' | 'ios' | 'backend' | 'frontend' | 'qa' | 'devops' | 'ai_workflow';

export type LevelLabel = 'L1_intern' | 'L2_junior' | 'L3_middle' | 'L4_senior' | 'L5_lead';
```

```typescript
// src/domain/model/entity/integration.ts
export interface Integration {
  id: string;
  projectId: string;
  type: 'jira' | 'figma' | 'google_drive' | 'firebase' | 'telegram';
  configEncrypted: Record<string, unknown>;
  status: 'pending' | 'connected' | 'failed';
  lastSyncedAt: string | null;
  createdAt: string;
  updatedAt: string;
}
```

### 2.2 Update Existing Project Entity

```typescript
// src/domain/model/entity/project.ts (updated)
export interface Project {
  id: string;
  name: string;
  description: string;
  platform: 'android' | 'ios' | 'web' | 'multi';
  repositoryUrl: string;
  directoryPath: string | null;     // NEW
  ownerId: string | null;           // NEW
  status: string;                   // NEW
  createdBy: string;
  createdAt: string;
  updatedAt: string;
}
```

---

## 3. Gateway Interfaces

```typescript
// src/domain/gateway/team-gateway.ts
export interface TeamGateway {
  listTeams(): Promise<Team[]>;
  getTeamById(teamId: string): Promise<Team | null>;
  createTeam(name: string): Promise<Team>;
  updateTeam(teamId: string, name: string): Promise<Team>;
  deleteTeam(teamId: string): Promise<void>;
  listMembersByTeam(teamId: string): Promise<TeamMember[]>;
  addMember(member: Omit<TeamMember, 'id' | 'createdAt' | 'updatedAt'>): Promise<TeamMember>;
  updateMember(memberId: string, patch: Partial<TeamMember>): Promise<TeamMember>;
  removeMember(memberId: string): Promise<void>;
  linkTeamToProject(projectId: string, teamId: string): Promise<void>;
  unlinkTeamFromProject(projectId: string, teamId: string): Promise<void>;
  listTeamsByProject(projectId: string): Promise<Team[]>;
}
```

---

## 4. TanStack Query Hooks

```typescript
// src/features/projects/queries.ts
export const projectKeys = {
  all: ['projects'] as const,
  detail: (id: string) => ['project', id] as const,
  teams: (projectId: string) => ['project', projectId, 'teams'] as const,
  contexts: (projectId: string) => ['project', projectId, 'contexts'] as const,
}

export function useProjects() {
  return useQuery({ queryKey: projectKeys.all, queryFn: () => projectRepo.list() })
}

export function useProject(projectId: string) {
  return useQuery({ queryKey: projectKeys.detail(projectId), queryFn: () => projectRepo.getById(projectId) })
}
```

```typescript
// src/features/members/queries.ts
export const memberKeys = {
  byTeam: (teamId: string) => ['members', teamId] as const,
}

export function useTeamMembers(teamId: string) {
  return useQuery({ queryKey: memberKeys.byTeam(teamId), queryFn: () => teamRepo.listMembersByTeam(teamId) })
}
```

---

## 5. UI Components

### 5.1 Project Listing Screen (`/projects`)
- DataTable with columns: Name, Platform, Status, Owner, Created
- "New Project" button → opens dialog/sheet
- Click row → navigates to `/projects/:projectId`

### 5.2 Project Detail Layout (`/projects/:projectId`)
- Tabbed navigation: Overview | Business Logic | Tech Specs | Coding Plan | Master Schedule | Tasks | Members | Workflows | Settings
- Project header with name, description, status badge

### 5.3 Team Management (`/projects/:projectId/members`)
- Member DataTable: Name, Role, Level (badge), Skills (tags), Capacity
- "Add Member" dialog with form: Name, Email, Role (select), Level (select), Skills (multi-tag), Capacity (number)
- "Import from Jira" button (disabled until Jira MCP connected — Phase 7)
- Workload summary widget showing capacity vs assigned hours

### 5.4 Settings (`/projects/:projectId/settings`)
- **Storage Strategy:** Dropdown to select "Artifact Storage Preference" (Supabase vs Google Drive). Note: Requires Google Drive MCP to be connected if Google Drive is selected.
- **MCP Contexts:** List of configured MCP contexts with status badges (PENDING/CONNECTED/FAILED). "Add MCP" button → select type → configure per SD-04 §4.

---

## 6. Definition of Done — Phase 2

- [ ] New Supabase migration with teams, project_teams, team_members tables + ALTER projects
- [ ] Project CRUD: list, create, edit, delete
- [ ] Team CRUD: create, rename, delete
- [ ] Team member CRUD: add, edit, remove with level labels
- [ ] Project ↔ Team linking (N:N)
- [ ] Project settings page with MCP context list (status display only, installation in Phase 7)
- [ ] Project detail page with tabbed navigation layout
- [ ] RLS policies for all new tables
