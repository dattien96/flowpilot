# CP-10: Integrations, Security Hardening & Audit

**Maps from:** SD-03 (Util Tools), SD-04 §4 (MCP Installation), SS-03 (Team), CP-05 (MCP Context)
**Phase:** 7
**Depends on:** CP-09

---

## 1. Core Concept

This final phase hardens the external systems configured in CP-05 (Jira, Firebase, Google Drive, Telegram) and the rest of the application for production use with stricter RLS policies, input validation, and audit trails. CP-10 must not move MCP configuration ownership out of CP-05.

---

## 2. Jira Integration Hardening

### 2.1 Database
```sql
-- Cache Jira members for project syncing
CREATE TABLE jira_members_cache (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
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
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  task_id UUID REFERENCES tasks(id) ON DELETE SET NULL,
  jira_issue_key TEXT NOT NULL,
  jira_status TEXT,
  jira_summary TEXT,
  synced_at TIMESTAMPTZ DEFAULT now(),
  UNIQUE(project_id, jira_issue_key)
);

-- NOTE: The `integrations` table already exists from CP-05.
-- Do NOT re-create or move it here. CP-10 only hardens policies and adds provider-specific sync/cache tables.

-- Enable RLS
ALTER TABLE jira_members_cache ENABLE ROW LEVEL SECURITY;
ALTER TABLE jira_issues_cache ENABLE ROW LEVEL SECURITY;

-- RLS policies: project owner/admin scoped read access
CREATE POLICY "jira_members_cache_select"
  ON jira_members_cache FOR SELECT
  USING (
    project_id IN (
      SELECT p.id FROM projects p
      WHERE p.created_by = auth.uid() OR p.owner_id = auth.uid()
    )
  );

CREATE POLICY "jira_issues_cache_select"
  ON jira_issues_cache FOR SELECT
  USING (
    project_id IN (
      SELECT p.id FROM projects p
      WHERE p.created_by = auth.uid() OR p.owner_id = auth.uid()
    )
  );

CREATE POLICY "integrations_select_project_members"
  ON integrations FOR SELECT
  USING (
    project_id IN (
      SELECT p.id FROM projects p
      WHERE p.created_by = auth.uid() OR p.owner_id = auth.uid()
    )
  );
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

## 3. Provider-Specific Integration Hardening

### 3.1 Firebase
- **Edge Function:** `firebase-fetch-crashes` — pulls crash logs for Issue Analysis step
- **Edge Function:** `firebase-fetch-analytics` — pulls usage data for Analytics Review step
- **Config:** Firebase project ID + service account key (stored encrypted in `integrations`)

### 3.2 Google Drive
- **Edge Function:** `google-drive-sync-artifacts` — uploads artifacts to a configured Drive folder
- **Edge Function:** `google-drive-sync-skills` — syncs `.claude/`, `.codex/`, `.gemini/` skill folders to Drive
- **Config:** OAuth2 tokens (stored encrypted)

### 3.3 Telegram
- **Edge Function:** `telegram-send-notification` — sends workflow status updates to a configured Telegram chat
- **Config:** Bot token + chat ID (stored encrypted)
- **Trigger:** Automatically called when a workflow step enters `WAITING_USER_APPROVAL` or `DONE`

---

## 4. Security Hardening

### 4.1 RLS Policy Audit
All existing permissive policies (`using (true)`) must be tightened:

```sql
-- Example: projects visible only to owner/admin clients
-- projects.created_by and owner_id are UUID (CP-04 baseline) — no ::text cast needed
CREATE POLICY "projects_select_own" ON projects
  FOR SELECT TO authenticated
  USING (
    created_by = auth.uid()
    OR owner_id = auth.uid()
  );

-- Example: features restricted to project owner/admin clients
CREATE POLICY "features_select_project_member" ON features
  FOR SELECT TO authenticated
  USING (
    project_id IN (
      SELECT id FROM projects
      WHERE created_by = auth.uid() OR owner_id = auth.uid()
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
  levelLabel: z.enum(['L1_intern', 'L2_junior', 'L3_middle', 'L4_senior', 'L5_lead']),  // maps to DB team_members.level_label
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
  user_id UUID NOT NULL REFERENCES auth.users(id),
  action TEXT NOT NULL,            -- create_project, approve_step, reject_step, generate_spec, etc.
  resource_type TEXT NOT NULL,     -- project, workflow_run, task, etc.
  resource_id UUID NOT NULL,
  metadata JSONB DEFAULT '{}',    -- additional context
  created_at TIMESTAMPTZ DEFAULT now()
);

ALTER TABLE audit_logs ENABLE ROW LEVEL SECURITY;

-- Users can read their own audit entries; admins can read all entries for their projects
CREATE POLICY "audit_logs_select_own"
  ON audit_logs FOR SELECT
  USING (user_id = auth.uid());

CREATE POLICY "audit_logs_insert_authenticated"
  ON audit_logs FOR INSERT
  WITH CHECK (auth.role() = 'authenticated');
```

> MVP note: CP-09/CP-10 allow authenticated admin/member writes in a few audit/log paths. That is acceptable for the admin-only MVP assumption, but CP-10 is still the phase that should tighten policies before production.

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

