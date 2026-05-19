# CP-08: Master Schedule & Task Board

**Maps from:** Google Doc §4.6–4.7, §12
**Phase:** 5
**Depends on:** CP-07

---

## 1. Core Concept

This phase implements two interconnected modules:
1. **Master Schedule Generator** — AI reads the coding plan + team members and generates a milestone-based schedule with level-appropriate task assignments.
2. **Task Board** — Tasks are created from schedule items and tracked through a Kanban/table view.

---

## 2. Database Tables (New Migration)

```sql
-- Master Schedules
CREATE TABLE master_schedules (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  coding_plan_id UUID REFERENCES coding_plans(id) ON DELETE SET NULL,
  title TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'draft',    -- draft, generated, reviewed, approved
  created_by TEXT NOT NULL,
  approved_by TEXT,
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now()
);

-- Schedule Items (individual milestone/task entries)
CREATE TABLE schedule_items (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  master_schedule_id UUID NOT NULL REFERENCES master_schedules(id) ON DELETE CASCADE,
  milestone_name TEXT NOT NULL,
  task_title TEXT NOT NULL,
  task_description TEXT,
  suggested_assignee_id UUID REFERENCES team_members(id) ON DELETE SET NULL,
  required_level TEXT NOT NULL,             -- L1_intern .. L5_lead
  estimated_effort_hours NUMERIC(6,1),
  priority TEXT NOT NULL DEFAULT 'medium',  -- low, medium, high, critical
  risk_level TEXT NOT NULL DEFAULT 'low',   -- low, medium, high
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
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  schedule_item_id UUID REFERENCES schedule_items(id) ON DELETE SET NULL,
  title TEXT NOT NULL,
  description TEXT,
  status TEXT NOT NULL DEFAULT 'backlog',   -- backlog, ready, in_progress, blocked, in_review, done
  priority TEXT NOT NULL DEFAULT 'medium',
  assignee_id UUID REFERENCES team_members(id) ON DELETE SET NULL,
  required_level TEXT,
  estimated_effort_hours NUMERIC(6,1),
  jira_issue_key TEXT,
  acceptance_criteria TEXT,
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now()
);

-- Task Dependencies
CREATE TABLE task_dependencies (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  task_id UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  depends_on_task_id UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  UNIQUE(task_id, depends_on_task_id)
);

-- Enable RLS
ALTER TABLE master_schedules ENABLE ROW LEVEL SECURITY;
ALTER TABLE schedule_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE tasks ENABLE ROW LEVEL SECURITY;
ALTER TABLE task_dependencies ENABLE ROW LEVEL SECURITY;
```

---

## 3. Domain Models

```typescript
// src/domain/model/entity/schedule.ts
export interface MasterSchedule {
  id: string;
  projectId: string;
  codingPlanId: string | null;
  title: string;
  status: DocumentStatus;
  createdBy: string;
  approvedBy: string | null;
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
  requiredLevel: LevelLabel;
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
  requiredLevel: LevelLabel | null;
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

Per Google Doc §4.6, the AI must respect seniority when suggesting assignees:

| Level | Suitable Tasks |
|-------|---------------|
| L1 Intern / L2 Junior | Small UI tasks, simple CRUD, documentation, basic test cases |
| L3 Middle | Normal feature implementation, integration work, moderate debugging |
| L4 Senior | Complex architecture, shared modules, security-sensitive logic, performance |
| L5 Lead / Architect | Technical decisions, review ownership, system design, risk management |

**Safety rule:** AI must NOT assign security/architecture tasks to L1/L2 unless an L4/L5 reviewer is attached.

---

## 5. UI Components

### 5.1 Master Schedule Generator (`/projects/:projectId/master-schedule`)

**List View:**
- DataTable of all schedules (title, linked coding plan, status, created date)
- "Generate New Schedule" button

**Generate Flow:**
1. Select a coding plan (dropdown of approved coding plans)
2. System fetches project team members with levels/capacity
3. "Generate Schedule" → calls AI Edge Function (Phase 6)
4. AI returns structured JSON → parsed into `schedule_items`
5. Schedule enters `generated` status → user reviews

**Schedule Detail View:**
- **Timeline/Gantt-style view** grouped by milestone:
  - Milestone header (name, date range)
  - Task rows with: title, assignee avatar, level badge, effort, priority, risk
  - Dependency arrows between related items
- **Table view** (alternative): sortable DataTable of all schedule items
- **Actions:**
  - Edit assignee / level / effort for each item
  - "Approve Schedule" → status = `approved`
  - "Convert to Tasks" → creates entries in `tasks` table

### 5.2 Task Board (`/projects/:projectId/tasks`)

**Kanban View:**
- Columns: Backlog | Ready | In Progress | Blocked | In Review | Done
- Task cards show: title, assignee, priority badge, level badge
- Drag-and-drop between columns

**Table View:**
- DataTable: Title, Status, Assignee, Priority, Level, Effort, Jira Key
- Inline status dropdown for quick updates

**Task Detail (Sheet/Drawer):**
- Full task info with editable fields
- Dependencies list
- Link to parent schedule item
- "Create Jira Issue" button (Phase 7)
- Activity log / comments

### 5.3 Dashboard Widgets (update from Phase 1)

Add these widgets to the main dashboard:
- **Team Capacity Snapshot:** Bar chart showing each member's assigned hours vs weekly capacity
- **Task Status Summary:** Pie/donut chart of task statuses
- **Risk/Blocker Panel:** List of high-risk or blocked tasks
- **Milestone Progress:** Progress bars per milestone

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

- [ ] Database migration: `master_schedules`, `schedule_items`, `tasks`, `task_dependencies`
- [ ] Master Schedule: CRUD, timeline view grouped by milestones
- [ ] Schedule item editing: assignee, level, effort, priority
- [ ] "Convert to Tasks" action: creates tasks from schedule items
- [ ] Task Board: Kanban view with drag-and-drop status changes
- [ ] Task Table: sortable/filterable DataTable alternative
- [ ] Task detail drawer with dependency display
- [ ] Dashboard widgets: capacity, status summary, risk panel
- [ ] Level-based assignment validation (warning if L1/L2 assigned critical task without reviewer)

