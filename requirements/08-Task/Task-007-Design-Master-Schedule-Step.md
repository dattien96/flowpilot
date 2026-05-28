# Task-007: Master Schedule & Task Board

**Maps from:** SS-03 (Team & Members), SS-04 §3.5.9 (Task Breakdown Step), SD-04 (Project Management DB)
**Phase:** 5
**Depends on:** CP-06, CP-07

---

## 1. Core Concept

This phase implements two interconnected modules:
1. **Master Schedule Generator** — AI reads the coding plan artifact + team members and generates a milestone-based schedule with level-appropriate task assignments (output of the "Task Breakdown" workflow step, SS-04 §3.5.9).
2. **Task Board** — Tasks are created from schedule items and tracked through a Kanban/table view.

**Key design constraint:** A `master_schedule` is the artifact output of the "Task Breakdown" workflow step. Its approval lifecycle is owned by `workflow_run_steps.status` (SD-09 canonical state machine), not a bespoke document status column. The schedule row is a structured representation of that artifact for UI purposes.

---

## 2. Database Tables (New Migration)

```sql
-- Master Schedules
-- Links to the workflow run step that generated it and the coding plan artifact.
-- No standalone status column: approval lifecycle is owned by workflow_run_steps.status (SD-09).
CREATE TABLE master_schedules (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  workflow_run_step_id UUID REFERENCES workflow_run_steps(id) ON DELETE SET NULL,
  coding_plan_artifact_run_id UUID REFERENCES artifact_runs(id) ON DELETE SET NULL,
  title TEXT NOT NULL,
  created_by UUID NOT NULL REFERENCES auth.users(id),
  approved_by UUID REFERENCES auth.users(id),
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now()
);

-- Schedule Items (individual milestone/task entries within a master schedule)
CREATE TABLE schedule_items (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  master_schedule_id UUID NOT NULL REFERENCES master_schedules(id) ON DELETE CASCADE,
  milestone_name TEXT NOT NULL,
  task_title TEXT NOT NULL,
  task_description TEXT,
  suggested_assignee_id UUID REFERENCES team_members(id) ON DELETE SET NULL,
  -- Level aligns to CP-04 team_members.level_label
  required_level_label TEXT NOT NULL CHECK (required_level_label IN ('L1_intern', 'L2_junior', 'L3_middle', 'L4_senior', 'L5_lead')),
  estimated_effort_hours NUMERIC(6,1),
  priority TEXT NOT NULL DEFAULT 'medium' CHECK (priority IN ('low', 'medium', 'high', 'critical')),
  risk_level TEXT NOT NULL DEFAULT 'low' CHECK (risk_level IN ('low', 'medium', 'high')),
  dependency_notes TEXT,
  acceptance_criteria TEXT,
  review_owner_id UUID REFERENCES team_members(id) ON DELETE SET NULL,
  start_date DATE,
  end_date DATE,
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now()
);

-- Tasks (execution-level items created from schedule items)
CREATE TABLE tasks (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  schedule_item_id UUID REFERENCES schedule_items(id) ON DELETE SET NULL,
  title TEXT NOT NULL,
  description TEXT,
  status TEXT NOT NULL DEFAULT 'backlog' CHECK (status IN ('backlog', 'ready', 'in_progress', 'blocked', 'in_review', 'done')),
  priority TEXT NOT NULL DEFAULT 'medium' CHECK (priority IN ('low', 'medium', 'high', 'critical')),
  assignee_id UUID REFERENCES team_members(id) ON DELETE SET NULL,
  required_level_label TEXT CHECK (required_level_label IN ('L1_intern', 'L2_junior', 'L3_middle', 'L4_senior', 'L5_lead')),
  estimated_effort_hours NUMERIC(6,1),
  jira_issue_key TEXT,
  acceptance_criteria TEXT,
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now()
);

-- Task Dependencies (DAG edges — prevents circular deps via app-layer validation)
CREATE TABLE task_dependencies (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  task_id UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  depends_on_task_id UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  UNIQUE(task_id, depends_on_task_id)
);

-- Enable RLS on all tables
ALTER TABLE master_schedules ENABLE ROW LEVEL SECURITY;
ALTER TABLE schedule_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE tasks ENABLE ROW LEVEL SECURITY;
ALTER TABLE task_dependencies ENABLE ROW LEVEL SECURITY;

-- RLS helper: project ownership/admin check (reused across policies)
-- team_members is a planning roster, not an auth identity table.

CREATE POLICY "master_schedules_select"
  ON master_schedules FOR SELECT
  USING (project_id IN (
    SELECT p.id FROM projects p
    WHERE p.created_by = auth.uid() OR p.owner_id = auth.uid()
  ));

CREATE POLICY "master_schedules_insert"
  ON master_schedules FOR INSERT
  WITH CHECK (project_id IN (
    SELECT p.id FROM projects p
    WHERE p.created_by = auth.uid() OR p.owner_id = auth.uid()
  ));

CREATE POLICY "master_schedules_update"
  ON master_schedules FOR UPDATE
  USING (project_id IN (
    SELECT p.id FROM projects p
    WHERE p.created_by = auth.uid() OR p.owner_id = auth.uid()
  ));

CREATE POLICY "schedule_items_select"
  ON schedule_items FOR SELECT
  USING (master_schedule_id IN (
    SELECT ms.id FROM master_schedules ms
    WHERE ms.project_id IN (
      SELECT p.id FROM projects p
      WHERE p.created_by = auth.uid() OR p.owner_id = auth.uid()
    )
  ));

CREATE POLICY "schedule_items_insert_update"
  ON schedule_items FOR ALL
  USING (master_schedule_id IN (
    SELECT ms.id FROM master_schedules ms
    WHERE ms.project_id IN (
      SELECT p.id FROM projects p
      WHERE p.created_by = auth.uid() OR p.owner_id = auth.uid()
    )
  ));

CREATE POLICY "tasks_select"
  ON tasks FOR SELECT
  USING (project_id IN (
    SELECT p.id FROM projects p
    WHERE p.created_by = auth.uid() OR p.owner_id = auth.uid()
  ));

CREATE POLICY "tasks_insert"
  ON tasks FOR INSERT
  WITH CHECK (project_id IN (
    SELECT p.id FROM projects p
    WHERE p.created_by = auth.uid() OR p.owner_id = auth.uid()
  ));

CREATE POLICY "tasks_update"
  ON tasks FOR UPDATE
  USING (project_id IN (
    SELECT p.id FROM projects p
    WHERE p.created_by = auth.uid() OR p.owner_id = auth.uid()
  ));

CREATE POLICY "task_dependencies_select"
  ON task_dependencies FOR SELECT
  USING (task_id IN (
    SELECT t.id FROM tasks t
    WHERE t.project_id IN (
      SELECT p.id FROM projects p
      WHERE p.created_by = auth.uid() OR p.owner_id = auth.uid()
    )
  ));

CREATE POLICY "task_dependencies_insert_delete"
  ON task_dependencies FOR ALL
  USING (task_id IN (
    SELECT t.id FROM tasks t
    WHERE t.project_id IN (
      SELECT p.id FROM projects p
      WHERE p.created_by = auth.uid() OR p.owner_id = auth.uid()
    )
  ));
```

---

## 3. Domain Models

```typescript
// src/domain/model/entity/schedule.ts

// Level vocabulary aligns to CP-04 team_members.level_label
export type LevelLabel = 'L1_intern' | 'L2_junior' | 'L3_middle' | 'L4_senior' | 'L5_lead';

export interface MasterSchedule {
  id: string;
  projectId: string;
  workflowRunStepId: string | null;        // FK → workflow_run_steps (owns approval lifecycle)
  codingPlanArtifactRunId: string | null;  // FK → artifact_runs (the coding plan runtime artifact used as input)
  title: string;
  createdBy: string;                       // UUID → auth.users
  approvedBy: string | null;              // UUID → auth.users
  createdAt: string;
  updatedAt: string;
}

export interface ScheduleItem {
  id: string;
  masterScheduleId: string;
  milestoneName: string;
  taskTitle: string;
  taskDescription: string | null;
  suggestedAssigneeId: string | null;
  requiredLevelLabel: LevelLabel;
  estimatedEffortHours: number | null;
  priority: 'low' | 'medium' | 'high' | 'critical';
  riskLevel: 'low' | 'medium' | 'high';
  dependencyNotes: string | null;
  acceptanceCriteria: string | null;
  reviewOwnerId: string | null;
  startDate: string | null;
  endDate: string | null;
  createdAt: string;
  updatedAt: string;
}
```

```typescript
// src/domain/model/entity/task.ts
export type TaskStatus = 'backlog' | 'ready' | 'in_progress' | 'blocked' | 'in_review' | 'done';

export interface Task {
  id: string;
  projectId: string;
  scheduleItemId: string | null;
  title: string;
  description: string | null;
  status: TaskStatus;
  priority: 'low' | 'medium' | 'high' | 'critical';
  assigneeId: string | null;
  requiredLevelLabel: LevelLabel | null;
  estimatedEffortHours: number | null;
  jiraIssueKey: string | null;
  acceptanceCriteria: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface TaskDependency {
  id: string;
  taskId: string;
  dependsOnTaskId: string;
}
```

---

## 4. Level-Based Task Assignment Rules

The system uses the CP-04 five-tier `level_label` vocabulary: `L1_intern | L2_junior | L3_middle | L4_senior | L5_lead`. The AI must respect seniority when suggesting assignees:

| Level Label | Role | Suitable Tasks |
|---|---|---|
| `L1_intern` | Intern | Documentation, simple QA, small isolated UI or content tasks |
| `L2_junior` | Junior Developer | Small UI tasks, simple CRUD, basic test cases |
| `L3_middle` | Middle Developer | Normal feature implementation, integration work, moderate debugging |
| `L4_senior` | Senior Developer | Complex modules, security-sensitive logic, performance, critical debugging |
| `L5_lead` | Lead / Architect | Architecture, system design, review ownership, risk management |

**Safety rule:** AI must NOT assign security or architecture tasks to `L1_intern` or `L2_junior` unless an `L4_senior` or `L5_lead` reviewer is attached (`review_owner_id` must reference that senior reviewer).

> **Note:** SS-03 and SD-04 may describe a simplified junior/mid/senior vocabulary, but the implemented CP-04 data model uses the richer L1-L5 `level_label`. CP-08 follows the implemented model.

---

## 5. UI Components

### 5.1 Master Schedule Generator (`/projects/:projectId/master-schedule`)

**List View:**
- DataTable of all schedules (title, linked coding plan, workflow run step status badge, created date)
- "Generate New Schedule" button — triggers the Task Breakdown workflow step (Phase 6)

**Generate Flow:**
1. Select an approved coding plan artifact run (filtered by the coding plan artifact definition binding and `workflow_run_steps.status = 'DONE'`)
2. System fetches project team members with levels and capacity
3. "Generate Schedule" → triggers AI via Task Breakdown workflow step (Phase 6 — button disabled in this phase)
4. AI returns structured JSON → parsed into `schedule_items` rows
5. Corresponding `workflow_run_steps.status` transitions to `WAITING_USER_APPROVAL` (approval gate, SD-09)

**Schedule Detail View:**
- **Timeline/Gantt-style view** grouped by milestone:
  - Milestone header (name, date range)
  - Task rows with: title, assignee avatar, level badge, effort, priority, risk
  - Dependency arrows between related items
- **Table view** (alternative): sortable DataTable of all schedule items
- **Actions:**
  - Edit assignee / level / effort for each item (inline or sheet)
  - "Approve Schedule" → calls Edge Function to set `workflow_run_steps.status = 'DONE'`
  - "Reject & Retry" → rejection note flow per SD-09 §4
  - "Convert to Tasks" → creates entries in `tasks` table from approved `schedule_items`

### 5.2 Task Board (`/projects/:projectId/tasks`)

**Kanban View:**
- Columns: Backlog | Ready | In Progress | Blocked | In Review | Done
- Task cards show: title, assignee avatar, priority badge, level badge
- Drag-and-drop between columns → optimistic update + Supabase write

**Supabase Realtime (collaborative updates):**
```typescript
// src/features/tasks/use-tasks-realtime.ts
export function useTasksRealtime(projectId: string) {
  const queryClient = useQueryClient()
  useEffect(() => {
    const channel = supabase
      .channel(`tasks_project_${projectId}`)
      .on('postgres_changes', {
        event: '*',
        schema: 'public',
        table: 'tasks',
        filter: `project_id=eq.${projectId}`,
      }, () => {
        queryClient.invalidateQueries({ queryKey: taskKeys.byProject(projectId) })
      })
      .subscribe()
    return () => { supabase.removeChannel(channel) }
  }, [projectId, queryClient])
}
```

**Table View:**
- DataTable: Title, Status, Assignee, Priority, Level, Effort, Jira Key
- Inline status dropdown for quick updates

**Task Detail (Sheet/Drawer):**
- Full task info with editable fields
- Dependencies list (blocks / blocked-by)
- Link to parent schedule item
- "Create Jira Issue" button (Phase 7 — disabled in this phase)
- Activity log / comments

### 5.3 Dashboard Widgets (update from Phase 1)

Add these widgets to the main dashboard:
- **Team Capacity Snapshot:** Bar chart showing each member's assigned hours vs weekly capacity
- **Task Status Summary:** Pie/donut chart of task statuses
- **Risk/Blocker Panel:** List of high-risk (`risk_level = 'high'`) or blocked (`status = 'blocked'`) items
- **Milestone Progress:** Progress bars per milestone (completed tasks / total tasks per milestone)

---

## 6. TanStack Query Hooks

```typescript
// src/features/master-schedule/queries.ts
export const scheduleKeys = {
  byProject: (projectId: string) => ['masterSchedules', projectId] as const,
  detail: (id: string) => ['masterSchedule', id] as const,
  items: (scheduleId: string) => ['scheduleItems', scheduleId] as const,
}

// src/features/tasks/queries.ts
export const taskKeys = {
  byProject: (projectId: string) => ['tasks', projectId] as const,
  detail: (id: string) => ['task', id] as const,
  dependencies: (taskId: string) => ['taskDependencies', taskId] as const,
}
```

---

## 7. Definition of Done — Phase 5

### Database
- [ ] `master_schedules` table: `project_id UUID`, `workflow_run_step_id UUID`, `coding_plan_artifact_run_id UUID REFERENCES artifact_runs(id)`
- [ ] `schedule_items` table with `required_level_label CHECK (IN ('L1_intern', 'L2_junior', 'L3_middle', 'L4_senior', 'L5_lead'))`
- [ ] `tasks` table: `project_id UUID`, `required_level_label CHECK (IN ('L1_intern', 'L2_junior', 'L3_middle', 'L4_senior', 'L5_lead'))`
- [ ] `task_dependencies` table with `UNIQUE(task_id, depends_on_task_id)`
- [ ] RLS SELECT + INSERT + UPDATE policies on all four tables (project-member scoped)

### Domain & Types
- [ ] `LevelLabel = 'L1_intern' | 'L2_junior' | 'L3_middle' | 'L4_senior' | 'L5_lead'` type defined (aligned to CP-04)
- [ ] `MasterSchedule` entity with `workflowRunStepId` and `codingPlanArtifactRunId` fields
- [ ] `Task`, `TaskDependency`, `ScheduleItem` entities defined

### UI
- [ ] Master Schedule list view with workflow run step status badge
- [ ] Schedule detail: Gantt/timeline view grouped by milestone
- [ ] Schedule detail: table view alternative
- [ ] "Approve" and "Reject & Retry" buttons wired to Edge Function (SD-09 approval gate)
- [ ] "Convert to Tasks" creates `tasks` rows from approved `schedule_items`
- [ ] Task Board: Kanban with drag-and-drop status changes
- [ ] Task Board: table view with inline status dropdown
- [ ] Task detail drawer with dependency display and parent schedule link
- [ ] Supabase Realtime subscription for live collaborative task updates
- [ ] Dashboard widgets: capacity, status summary, risk panel, milestone progress

### Business Rules
- [ ] Level-based assignment validation: warn if `L1_intern` or `L2_junior` is assigned a high-risk task without an `L4_senior` or `L5_lead` `review_owner_id`
- [ ] "Generate Schedule" dropdown filters to coding-plan `artifact_runs` only, resolved through the coding plan artifact definition binding
- [ ] "Create Jira Issue" button visible but disabled (Phase 7 dependency)
