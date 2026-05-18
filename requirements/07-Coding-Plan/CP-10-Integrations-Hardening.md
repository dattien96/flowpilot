# CP-10: Integrations, Security Hardening & Audit

**Maps from:** SD-03 (Util Tools), SD-04 §4 (MCP Installation), Google Doc §10, §13.8
**Phase:** 7
**Depends on:** CP-09

---

## 1. Core Concept

This final phase connects FlowPilot to external systems (Jira, Firebase, Google Drive, Telegram) and hardens the entire application for production use with proper RLS policies, input validation, and audit trails.

---

## 2. Jira MCP Integration

### 2.1 Database
```sql
-- Cache Jira members for project syncing
CREATE TABLE jira_members_cache (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  jira_account_id TEXT NOT NULL,
  display_name TEXT NOT NULL,
  email TEXT,
  avatar_url TEXT,
  synced_at TIMESTAMPTZ DEFAULT now(),
  UNIQUE(project_id, jira_account_id)
);

-- Cache Jira issues for status syncing
CREATE TABLE jira_issues_cache (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  task_id UUID REFERENCES tasks(id) ON DELETE SET NULL,
  jira_issue_key TEXT NOT NULL,
  jira_status TEXT,
  jira_summary TEXT,
  synced_at TIMESTAMPTZ DEFAULT now(),
  UNIQUE(project_id, jira_issue_key)
);

-- Integration config (securely stored)
CREATE TABLE integrations (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  type TEXT NOT NULL,                 -- jira, firebase, google_drive, telegram
  config_encrypted JSONB NOT NULL,    -- tokens, URLs, etc. (encrypt sensitive values)
  status TEXT NOT NULL DEFAULT 'pending',  -- pending, connected, failed
  last_synced_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now()
);

ALTER TABLE jira_members_cache ENABLE ROW LEVEL SECURITY;
ALTER TABLE jira_issues_cache ENABLE ROW LEVEL SECURITY;
ALTER TABLE integrations ENABLE ROW LEVEL SECURITY;
```

### 2.2 Edge Functions for Jira

```
POST /functions/v1/jira-import-members
Body: { projectId, integrationId }
→ Calls Jira REST API → upserts into jira_members_cache
→ Optionally maps to team_members by jira_account_id

POST /functions/v1/jira-create-issue
Body: { projectId, taskId, integrationId }
→ Creates Jira issue from task → stores jira_issue_key on task

POST /functions/v1/jira-sync-status
Body: { projectId, integrationId }
→ Reads Jira issue statuses → updates jira_issues_cache
→ Optionally syncs back to tasks.status
```

### 2.3 UI Integration Points
- `/projects/:projectId/members` → "Import from Jira" button (now enabled)
- `/projects/:projectId/tasks` → "Create Jira Issue" on each task card
- `/projects/:projectId/tasks` → Jira status badge sync indicator
- `/projects/:projectId/settings` → Jira integration configuration

---

## 3. Other MCP Integrations

### 3.1 Firebase MCP
- **Edge Function:** `firebase-fetch-crashes` — pulls crash logs for Issue Analysis step
- **Edge Function:** `firebase-fetch-analytics` — pulls usage data for Analytics Review step
- **Config:** Firebase project ID + service account key (stored encrypted in `integrations`)

### 3.2 Google Drive MCP
- **Edge Function:** `google-drive-sync-artifacts` — uploads artifacts to a configured Drive folder
- **Edge Function:** `google-drive-sync-skills` — syncs `.claude/`, `.codex/`, `.gemini/` skill folders to Drive
- **Config:** OAuth2 tokens (stored encrypted)

### 3.3 Telegram MCP
- **Edge Function:** `telegram-send-notification` — sends workflow status updates to a configured Telegram chat
- **Config:** Bot token + chat ID (stored encrypted)
- **Trigger:** Automatically called when a workflow step enters `WAITING_USER_APPROVAL` or `DONE`

---

## 4. Security Hardening

### 4.1 RLS Policy Audit
All existing permissive policies (`using (true)`) must be tightened:

```sql
-- Example: projects visible only to owner or team members
CREATE POLICY "projects_select_own" ON projects
  FOR SELECT TO authenticated
  USING (
    created_by = auth.uid()::text
    OR id IN (
      SELECT pt.project_id FROM project_teams pt
      JOIN team_members tm ON tm.team_id = pt.team_id
      WHERE tm.email = auth.jwt()->'email'
    )
  );

-- Example: features restricted to project members
CREATE POLICY "features_select_project_member" ON features
  FOR SELECT TO authenticated
  USING (
    project_id IN (
      SELECT id FROM projects WHERE created_by = auth.uid()::text
    )
  );
```

### 4.2 Input Validation (Zod Schemas)
All forms must validate with Zod before submitting:

```typescript
// src/lib/validators/project.ts
import { z } from 'zod'

export const createProjectSchema = z.object({
  name: z.string().min(3).max(100),
  description: z.string().min(10).max(500),
  platform: z.enum(['android', 'ios', 'web', 'multi']),
  repositoryUrl: z.string().url(),
  directoryPath: z.string().optional(),
})

// src/lib/validators/team-member.ts
export const addMemberSchema = z.object({
  name: z.string().min(2),
  email: z.string().email().optional(),
  role: z.enum(['android', 'ios', 'backend', 'frontend', 'qa', 'devops', 'ai_workflow']),
  levelLabel: z.enum(['L1_intern', 'L2_junior', 'L3_middle', 'L4_senior', 'L5_lead']),
  skillTags: z.array(z.string()).default([]),
  weeklyCapacityHours: z.number().min(1).max(60).default(40),
})
```

### 4.3 Security Checklist
- [ ] No AI API keys in frontend code (all in Edge Function env vars)
- [ ] No Supabase service role key in frontend
- [ ] All forms validated with Zod before mutation
- [ ] RLS policies restrict data to project members
- [ ] Edge Functions validate JWT auth header
- [ ] Prompt template editing restricted to admin/owner role
- [ ] Integration config (tokens) encrypted at rest

---

## 5. Audit Trail & Observability

### 5.1 Audit Log Table
```sql
CREATE TABLE audit_logs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id TEXT NOT NULL,
  action TEXT NOT NULL,            -- create_project, approve_step, reject_step, generate_spec, etc.
  resource_type TEXT NOT NULL,     -- project, workflow_run, task, etc.
  resource_id TEXT NOT NULL,
  metadata JSONB DEFAULT '{}',    -- additional context
  created_at TIMESTAMPTZ DEFAULT now()
);

ALTER TABLE audit_logs ENABLE ROW LEVEL SECURITY;
```

### 5.2 Auto-logging
Write audit entries for:
- Project create/update/delete
- Workflow run start/complete/fail
- Approval decisions (approve/reject with comments)
- AI generation runs (with model/cost)
- Task assignment changes
- Integration connect/disconnect
- Schedule approval

---

## 6. Error Handling & UX Polish

### 6.1 Global Error Boundary
```typescript
// src/components/common/error-boundary.tsx
export function ErrorBoundary({ error }: { error: Error }) {
  return (
    <Card className="p-8 text-center">
      <h2>Something went wrong</h2>
      <p>{error.message}</p>
      <Button onClick={() => window.location.reload()}>Retry</Button>
    </Card>
  )
}
```

### 6.2 Loading & Empty States
- Skeleton loaders for all DataTables and detail views
- Empty state illustrations with call-to-action buttons
- Toast notifications for mutations (success/error)

### 6.3 Responsive Layout
- Sidebar collapses on smaller screens
- DataTables switch to card layout on mobile (if needed for tablet usage)

---

## 7. Definition of Done — Phase 7

- [ ] Jira integration: import members, create issues, sync status
- [ ] Firebase integration: fetch crashes and analytics (via Edge Function)
- [ ] Google Drive integration: artifact and skill sync
- [ ] Telegram integration: workflow notifications
- [ ] RLS policies hardened (no more `using (true)`)
- [ ] All forms validated with Zod
- [ ] Audit log table with auto-logging for key actions
- [ ] Error boundaries, loading states, empty states
- [ ] No secrets in frontend code
- [ ] Edge Functions validate auth headers

