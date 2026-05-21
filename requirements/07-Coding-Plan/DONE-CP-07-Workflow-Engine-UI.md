# CP-07: Workflow Engine UI & Execution Dashboard

**Maps from:** SD-05 (Workflow Engine), SD-09 (Approval Gates), SS-04 (Workflow Spec)
**Phase:** 4
**Depends on:** CP-05

---

## 1. Core Concept

This phase builds the **Workflow Builder** (design-time) and **Execution Dashboard** (run-time) that allow users to construct custom workflows from the 17 MVP steps defined in SS-04, then monitor their execution in real-time.

This phase also owns the workspace workflow-definition index, the dedicated workflow detail/builder page, the create-workflow page, the step-definition catalog and create-step page, and the workspace workflow-run history page.

---

## 2. Database - Canonical Workflow Schema

The MVP skeleton has legacy `workflow_definitions`, `workflow_runs`, `workflow_steps`, `ai_outputs`, `approvals`, and `approval_decisions` tables. CP-07 replaces that runtime model with the canonical SD-05/SD-09 schema below. Later CPs must use `workflows`, definition-time `workflow_steps`, execution-time `workflow_run_steps`, and canonical `artifacts`; they must not add new features to the legacy `ai_outputs` runtime path.

```sql
CREATE TABLE workflows (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID REFERENCES projects(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  description TEXT,
  is_template BOOLEAN NOT NULL DEFAULT false,
  provider_override TEXT,
  model_override TEXT,
  created_by UUID REFERENCES auth.users(id),
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now()
);

-- Definition-time step configuration only.
CREATE TABLE workflow_steps (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  workflow_id UUID NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
  step_type TEXT NOT NULL,
  order_index INT NOT NULL,
  provider_override TEXT,
  model_override TEXT,
  requires_approval BOOLEAN DEFAULT true,
  is_enabled BOOLEAN DEFAULT true,
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now(),
  UNIQUE(workflow_id, order_index)
);

CREATE TABLE workflow_runs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  workflow_id UUID NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  status TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'RUNNING', 'DONE', 'FAILED', 'CANCELED')),
  provider TEXT,
  model TEXT,
  yolo_mode BOOLEAN NOT NULL DEFAULT false, -- persisted per-run for granular control
  started_by UUID REFERENCES auth.users(id),
  started_at TIMESTAMPTZ DEFAULT now(),
  finished_at TIMESTAMPTZ,
  error_message TEXT
);

-- Execution-time step state only.
CREATE TABLE workflow_run_steps (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  workflow_run_id UUID NOT NULL REFERENCES workflow_runs(id) ON DELETE CASCADE,
  workflow_step_id UUID NOT NULL REFERENCES workflow_steps(id) ON DELETE CASCADE,
  step_type TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'RUNNING', 'WAITING_USER_APPROVAL', 'DONE', 'FAILED', 'SKIPPED')),
  artifact_id UUID, -- CP-06 adds FK to artifacts(id) after artifacts table exists. See CP-06 DoD.
  prompt_cache_id UUID,
  rejection_note TEXT,
  retry_count INT NOT NULL DEFAULT 0,
  started_at TIMESTAMPTZ,
  finished_at TIMESTAMPTZ,
  error_message TEXT
);

-- Workflow prompt cache (per SD-05 §7.2)
CREATE TABLE workflow_prompt_cache (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  workflow_id UUID NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
  config_hash VARCHAR NOT NULL UNIQUE,   -- UNIQUE enforced: hash collision must not create duplicate entries
  file_path VARCHAR NOT NULL,
  step_type TEXT NOT NULL,
  provider TEXT NOT NULL,
  is_valid BOOLEAN DEFAULT true,
  created_at TIMESTAMPTZ DEFAULT now(),
  invalidated_at TIMESTAMPTZ
);

CREATE INDEX idx_prompt_cache_hash ON workflow_prompt_cache(config_hash) WHERE is_valid = true;

ALTER TABLE workflow_run_steps
  ADD CONSTRAINT workflow_run_steps_prompt_cache_fk
  FOREIGN KEY (prompt_cache_id) REFERENCES workflow_prompt_cache(id);

-- Enable RLS on core workflow tables
ALTER TABLE workflows ENABLE ROW LEVEL SECURITY;
ALTER TABLE workflow_steps ENABLE ROW LEVEL SECURITY;
ALTER TABLE workflow_runs ENABLE ROW LEVEL SECURITY;
ALTER TABLE workflow_run_steps ENABLE ROW LEVEL SECURITY;
ALTER TABLE workflow_prompt_cache ENABLE ROW LEVEL SECURITY;

-- Read policy: project members can view cache entries for their workflows
CREATE POLICY "prompt_cache_select_project_members"
  ON workflow_prompt_cache FOR SELECT
  USING (
    workflow_id IN (
      SELECT w.id FROM workflows w
      WHERE
        w.project_id IS NULL
        OR w.project_id IN (
          SELECT pt.project_id FROM project_teams pt
          JOIN team_members tm ON tm.team_id = pt.team_id
          WHERE tm.user_id = auth.uid()
        )
    )
  );

-- Read policy: global workflows for everyone, private workflows for project members only
CREATE POLICY "workflows_select_global_or_project_members"
  ON workflows FOR SELECT
  USING (
    project_id IS NULL
    OR project_id IN (
      SELECT pt.project_id FROM project_teams pt
      JOIN team_members tm ON tm.team_id = pt.team_id
      WHERE tm.user_id = auth.uid()
    )
  );

-- Write policy: only members of the owning project can mutate private workflows
CREATE POLICY "workflows_write_private_project_members"
  ON workflows FOR ALL
  USING (
    project_id IS NOT NULL
    AND project_id IN (
      SELECT pt.project_id FROM project_teams pt
      JOIN team_members tm ON tm.team_id = pt.team_id
      WHERE tm.user_id = auth.uid()
    )
  )
  WITH CHECK (
    project_id IS NOT NULL
    AND project_id IN (
      SELECT pt.project_id FROM project_teams pt
      JOIN team_members tm ON tm.team_id = pt.team_id
      WHERE tm.user_id = auth.uid()
    )
  );

-- Read policy for workflow_steps follows the workflow scope
CREATE POLICY "workflow_steps_select_global_or_project_members"
  ON workflow_steps FOR SELECT
  USING (
    workflow_id IN (
      SELECT w.id FROM workflows w
      WHERE
        w.project_id IS NULL
        OR w.project_id IN (
          SELECT pt.project_id FROM project_teams pt
          JOIN team_members tm ON tm.team_id = pt.team_id
          WHERE tm.user_id = auth.uid()
        )
    )
  );

-- Write policy for workflow_steps is limited to private workflows
CREATE POLICY "workflow_steps_write_private_project_members"
  ON workflow_steps FOR ALL
  USING (
    workflow_id IN (
      SELECT w.id FROM workflows w
      WHERE
        w.project_id IS NOT NULL
        AND w.project_id IN (
          SELECT pt.project_id FROM project_teams pt
          JOIN team_members tm ON tm.team_id = pt.team_id
          WHERE tm.user_id = auth.uid()
        )
    )
  );

-- Read/Write policies for workflow_runs
CREATE POLICY "workflow_runs_all_project_members"
  ON workflow_runs FOR ALL
  USING (
    project_id IN (
      SELECT pt.project_id FROM project_teams pt
      JOIN team_members tm ON tm.team_id = pt.team_id
      WHERE tm.user_id = auth.uid()
    )
  );

-- Read/Write policies for workflow_run_steps
CREATE POLICY "workflow_run_steps_all_project_members"
  ON workflow_run_steps FOR ALL
  USING (
    workflow_run_id IN (
      SELECT wr.id FROM workflow_runs wr
      WHERE wr.project_id IN (
        SELECT pt.project_id FROM project_teams pt
        JOIN team_members tm ON tm.team_id = pt.team_id
        WHERE tm.user_id = auth.uid()
      )
    )
  );

-- Workflow execution logs (per SD-05 §7.3)
-- Note: references workflow_run_steps (execution instance), NOT workflow_steps (definition)
CREATE TABLE workflow_run_logs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  workflow_run_step_id UUID NOT NULL REFERENCES workflow_run_steps(id) ON DELETE CASCADE,
  log_level TEXT NOT NULL DEFAULT 'INFO',   -- INFO, WARN, ERROR, DEBUG
  message TEXT NOT NULL,
  created_at TIMESTAMPTZ DEFAULT now()
);

ALTER TABLE workflow_run_logs ENABLE ROW LEVEL SECURITY;

-- Read policy: project members can view logs for their workflow runs
CREATE POLICY "run_logs_select_project_members"
  ON workflow_run_logs FOR SELECT
  USING (
    workflow_run_step_id IN (
      SELECT wrs.id FROM workflow_run_steps wrs
      JOIN workflow_runs wr ON wr.id = wrs.workflow_run_id
      WHERE wr.project_id IN (
        SELECT pt.project_id FROM project_teams pt
        JOIN team_members tm ON tm.team_id = pt.team_id
        WHERE tm.user_id = auth.uid()
      )
    )
  );

-- Step definitions seed table (per SD-05 §7.4)
-- Static/seeded data — the 17 MVP step types
CREATE TABLE step_definitions (
  step_type TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  description TEXT NOT NULL,
  required_mcps JSONB DEFAULT '[]',      -- e.g. ["jira", "firebase"]
  required_skills JSONB DEFAULT '[]',    -- e.g. ["tech_spec_skill"]
  agent_type TEXT NOT NULL DEFAULT 'standard'  -- standard, autonomous (for code/review loop)
);

ALTER TABLE step_definitions ENABLE ROW LEVEL SECURITY;

-- Read policy: any authenticated user can read step definitions (shared catalogue)
CREATE POLICY "step_definitions_select_authenticated"
  ON step_definitions FOR SELECT
  USING (auth.role() = 'authenticated');

-- Write policy: authenticated workflow admins can create or update reusable step definitions
CREATE POLICY "step_definitions_write_authenticated"
  ON step_definitions FOR ALL
  USING (auth.role() = 'authenticated')
  WITH CHECK (auth.role() = 'authenticated');

-- Seed data (insert 17 MVP step types)
INSERT INTO step_definitions (step_type, name, description, required_mcps, required_skills, agent_type) VALUES
  ('business_idea', 'Business Idea', 'Capture and refine a raw business idea', '[]', '["business_analyst_skill"]', 'standard'),
  ('feature_intake', 'Feature Intake', 'Read and structure feature requirements from Jira', '["jira"]', '["feature_intake_skill"]', 'standard'),
  ('business_summary', 'Business Summary', 'Summarize business requirements into a PRD', '[]', '["business_summary_skill"]', 'standard'),
  ('product_spec', 'Product Spec', 'Generate a detailed product specification', '[]', '["product_spec_skill"]', 'standard'),
  ('tech_spec', 'Tech Spec', 'Generate a technical specification', '[]', '["tech_spec_skill"]', 'standard'),
  ('make_plan_coding', 'Make Plan Coding', 'Create a step-by-step coding plan', '[]', '["make_plan_coding_skill"]', 'standard'),
  ('create_architecture', 'Create Architecture', 'Design system architecture and components', '[]', '["create_architecture_skill"]', 'standard'),
  ('tdd', 'TDD', 'Create unit test signatures matching business requirements', '[]', '["tdd_skill"]', 'standard'),
  ('task_breakdown', 'Task Breakdown', 'Break coding plan into developer tasks with master schedule', '[]', '["task_breakdown_skill"]', 'standard'),
  ('code_review_loop', 'Code/Review Loop', 'Autonomously code, test, and review until passing', '[]', '["coding_skill", "review_skill"]', 'autonomous'),
  ('release_readiness', 'Release Readiness', 'Verify release criteria are met', '[]', '["release_readiness_skill"]', 'standard'),
  ('issue_analysis', 'Issue Analysis', 'Analyze crash reports and issues for root cause', '["firebase", "jira"]', '["issue_analysis_skill"]', 'standard'),
  ('analytics_review', 'Analytics Review', 'Review usage data and user behavior insights', '["firebase"]', '["analytics_review_skill"]', 'standard'),
  ('project_analysis', 'Project Analysis', 'Analyze project health and process bottlenecks', '["google_drive"]', '["project_analysis_skill"]', 'standard'),
  ('telegram_notification', 'Telegram Notification', 'Send workflow status to Telegram chat', '["telegram"]', '["notification_skill"]', 'standard'),
  ('code_traceability', 'Code Traceability', 'Trace code/bug back to original Jira ticket', '["jira"]', '["code_traceability_skill"]', 'standard'),
  ('onboarding_walkthrough', 'Onboarding Walkthrough', 'Generate product/codebase summary for new members', '["google_drive"]', '["onboarding_skill"]', 'standard');
```

---

## 2.1 Workflow Prompt Caching — `/built-in-workflow/` (from SD-05 §2.3)

The workflow engine's **core optimization** is caching assembled prompt `.md` files so that identical workflow configurations reuse the same prompt without regeneration.

### 2.1.1 Where Cached Files Live
```
<flowpilot-system-root>/built-in-workflow/<hash>_<step_type>.md
```
Example: `/built-in-workflow/a3f8c2_tech_spec.md`

**NOT** inside `.claude/`, `.codex/`, `.gemini/` — those folders stay clean with project-level instructions and skills only.

### 2.1.2 Hash Computation (Go-Runner)
The Go-Runner generates a deterministic SHA256 from the full workflow step configuration:
```go
// internal/workflow/hash.go
func computeConfigHash(cfg StepConfig) string {
    h := sha256.New()
    h.Write([]byte(cfg.WorkflowID))
    h.Write([]byte(cfg.StepType))
    h.Write([]byte(fmt.Sprintf("%d", cfg.OrderIndex)))
    h.Write([]byte(fmt.Sprintf("%v", cfg.IsEnabled)))
    h.Write([]byte(cfg.Provider))
    h.Write([]byte(cfg.Model))
    for _, skillID := range cfg.SkillIDs {
        h.Write([]byte(skillID))
    }
    for _, contentHash := range cfg.CustomSkillContentHashes {
        h.Write([]byte(contentHash))
    }
    return hex.EncodeToString(h.Sum(nil))[:12] // short hash for filename
}
```

### 2.1.3 Cache-First Execution Loop
```
Go-Runner receives PENDING step
    → Compute config_hash from step configuration
    → Query workflow_prompt_cache WHERE config_hash = ? AND is_valid = true
    → IF cache HIT:
        → Read cached .md file from /built-in-workflow/<hash>_<step_type>.md
        → Inject runtime placeholders (MCP context, user context, previous artifacts)
        → Send to AI provider
    → IF cache MISS:
        → Assemble new .md from step SKILL.md templates + workflow config
        → Save to /built-in-workflow/<hash>_<step_type>.md
        → Insert record into workflow_prompt_cache table
        → Inject runtime placeholders
        → Send to AI provider
```

### 2.1.4 MCP Data Hydration Before Prompt Injection
The workflow engine must fetch MCP-backed data before the final prompt is sent to the AI provider.

This step is separate from prompt assembly:

1. resolve which MCPs are required for the current step
2. read the project integration rows for those MCPs
3. ask the local runner for fresh provider data
4. normalize the returned MCP content into a step-specific runtime payload
5. inject that payload into `{{mcp_context}}`
6. continue with the cached or freshly assembled prompt

Examples:

- `feature_intake` may fetch Jira issue data and related comments
- `project_analysis` may fetch Google Drive project docs
- `telegram_notification` may not need inbound data, only a send action

The fetch step must use current data, not stale cached copies, so prompt execution reflects the latest external context.

### 2.1.5 Runtime Placeholder Injection
The cached `.md` contains structural template sections. At execution time, the Go-Runner fills:
```markdown
# MCP Context
{{mcp_context}}          ← Jira ticket data, Firebase crash logs, etc.

# User Context
{{user_context}}          ← Manual text from context_sources

# Previous Artifacts
{{previous_artifacts}}    ← Output from earlier steps in this run

# Reviewer Feedback (Retry)
{{rejection_note}}        ← Only populated on retry (from workflow_run_steps.rejection_note)
```

This means the **structural template is cached**, but **run-specific data is always fresh**.

### 2.1.6 Cache Invalidation Rules
The cached `.md` is **regenerated** (old record marked `is_valid = false`) when:
- User modifies the workflow (adds/removes/reorders/enables/disables steps)
- A skill file (built-in or custom) is updated
- Provider or model override changes

Any of these changes cause the config_hash to change → new file generated → new DB record.

### 2.1.7 Admin Web Cache Visibility
- **Workflow Builder:** After save, display: "This configuration maps to cache hash: `a3f8c2`"
- **Execution Dashboard:** Step detail shows: "Prompt file: `/built-in-workflow/a3f8c2_tech_spec.md`" (clickable to view the actual assembled prompt)
- **Settings:** "Clear Prompt Cache" button to invalidate all cached files for a workflow

---

## 3. Domain Model Updates

### 3.1 Update Status Constants
```typescript
// src/domain/constant/status.ts (UPDATED)
// Canonical values per SD-09 §2 — must match workflow_run_steps.status in DB exactly
export type WorkflowStepStatus =
  | 'PENDING'
  | 'RUNNING'
  | 'WAITING_USER_APPROVAL'
  | 'DONE'
  | 'FAILED'
  | 'SKIPPED';
```

### 3.2 New Workflow Builder Types
```typescript
// src/domain/model/entity/workflow-builder.ts
export interface WorkflowTemplate {
  id: string;
  name: string;
  description: string;
  persona: 'developer' | 'solo_dev' | 'leader' | 'pm';
  steps: WorkflowTemplateStep[];
}

export interface WorkflowTemplateStep {
  stepType: StepType;
  order: number;
  isEnabled: boolean;
  requiresApproval: boolean;
  providerOverride: string | null;
  modelOverride: string | null;
  requiredMcps: string[];
  requiredSkills: string[];
}

export type StepType =
  | 'business_idea'
  | 'feature_intake'
  | 'business_summary'
  | 'product_spec'
  | 'tech_spec'
  | 'make_plan_coding'
  | 'create_architecture'
  | 'tdd'
  | 'task_breakdown'
  | 'code_review_loop'
  | 'release_readiness'
  | 'issue_analysis'
  | 'analytics_review'
  | 'project_analysis'
  | 'telegram_notification'
  | 'code_traceability'
  | 'onboarding_walkthrough';
```

### 3.3 Prompt Cache Entity
```typescript
// src/domain/model/entity/prompt-cache.ts
export interface PromptCacheEntry {
  id: string;
  workflowId: string;
  configHash: string;
  filePath: string;
  stepType: StepType;
  provider: string;
  isValid: boolean;
  createdAt: string;
  invalidatedAt: string | null;
}
```

---

## 4. UI Components

### 4.1 Workflow Builder (`/projects/:projectId/workflows`)

**Layout:** Split view — left panel with step palette, right panel with configured workflow

**Step Palette:**
- All 17 MVP steps listed as draggable cards with:
  - Step name
  - Description
  - Required MCPs (badge: Jira, Firebase, etc.)
  - Required skills

**Workflow Configuration Panel:**
- Ordered list of steps (drag to reorder)
- Per-step controls:
  - Enable/disable toggle
  - Approval gate toggle
  - Provider override dropdown (Claude / Codex / Gemini / Default)
  - Model override dropdown
- Save as custom workflow
- Load from built-in template

**Built-in Templates** (per SS-04 §5 — all 10 flows, 4 personas):
| Template | Persona | Steps |
|----------|---------|-------|
| Bug Fix Flow | Developer | Traceability → Issue Analysis → Tech Spec → Plan → Code/Review → Release |
| Pre-defined Feature | Developer | Tech Spec → Plan → Architecture → TDD → Code/Review → Release → Notify |
| Bug Traceability | Developer | Code Traceability |
| Onboarding | Developer | Onboarding Walkthrough |
| Full End-to-End | Solo Dev | Business Idea → Feature Intake → Business Summary → Product Spec → Tech Spec → Plan → Arch → TDD → Code/Review → Release → Notify |
| Fast-Track Business | Solo Dev | Product Spec → Tech Spec → Plan → Arch → TDD → Code/Review → Release |
| Task Breakdown | Leader | Tech Spec → Plan → Task Breakdown |
| Root Cause Analysis | Leader | Traceability → Issue Analysis → Task Breakdown → Notify |
| Analytics & Usage | Leader | Analytics Review |
| Product Process & Analysis | PM/Owner | Business Idea → Feature Intake → Business Summary → Product Spec → Project Analysis → Analytics Review |

### 4.2 Workflow Definition Index (`/workflows`)

- List all workflow definitions, not workflow runs
- Show built-in global templates and private project-owned workflows
- Provide filters for scope and owner project
- Make each workflow item navigate to a dedicated workflow detail/builder page
- Add CTA to `/workflows/create`
- Add CTA to `/workflow-steps`
- Add CTA to `/workflow-runs`

### 4.2.1 Workflow Detail / Builder (`/workflows/:workflowId`)

- Show one selected workflow definition on its own page
- Allow step composition, reordering, and override editing for private workflows
- Keep built-in global workflows readable but not directly editable
- Allow launching the selected workflow against the active project context

### 4.3 Create Workflow Page (`/workflows/create`)

- Allow the user to select the owner project for the new private workflow
- Allow the user to add reusable step definitions, order them, and remove them
- Allow a quick link to `/workflow-steps/create`
- Save through the canonical `workflows` + `workflow_steps` path

### 4.4 Step Definition Index (`/workflow-steps`)

- Show the full list of reusable step definitions
- Show `step_type`, description, MCP requirements, skill requirements, and agent type
- Add CTA to `/workflow-steps/create`

### 4.5 Create Step Page (`/workflow-steps/create`)

- Form fields: `step_type`, `name`, `description`, `required_mcps`, `required_skills`, `agent_type`
- Save to `step_definitions`

### 4.6 Workflow Run History (`/workflow-runs`)

- Show all workflow runs across the workspace
- Support filters by workflow type, global-vs-private scope, and project
- Link back to workflow definitions and workflow creation

### 4.6.1 Project Workflow Entry (`/projects/:projectId/workflows`)

- Do not embed the workflow definition list in the project tab
- Do not embed the workflow builder in the project tab
- Show actions to browse definitions, create a private workflow, and open run history
- Show recent run history scoped to the current project

### 4.7 Execution Dashboard (`/projects/:projectId/workflows/:runId`)

**Layout:** Vertical pipeline view showing each step as a card

**Step Card:**
- Step name
- Status badge: `PENDING` (gray) | `RUNNING` (blue pulse) | `WAITING_USER_APPROVAL` (yellow) | `DONE` (green) | `FAILED` (red) | `SKIPPED` (gray strikethrough)
- Elapsed time
- Provider/model used
- Link to artifact (if completed)

**Approval Gate UI:**
- When status = `WAITING_USER_APPROVAL`, show blocking overlay:
  - Generated artifact preview (Markdown render)
  - "Approve & Continue" button → sets status = `DONE`, marks next step `PENDING`
  - "Reject & Retry" button → opens rejection note dialog:
    - Required text input: "What needs to change?"
    - On submit: sets status = `PENDING`, stores `rejection_note`, increments `retry_count`

**YOLO Mode:**
- Persisted as `yolo_mode` (BOOLEAN) on the `workflow_runs` table, allowing granular, per-run execution control.
- Toggle at workflow header level (applies to all remaining steps in the active run).
- When enabled: all approval gates are skipped, pipeline runs continuously.
- **Edge Function Guard:** Toggle changes must go through an Edge Function (`/workflow-runs/toggle-yolo`) which ensures that only project owners, leaders, or the user who started the run can mutate `yolo_mode`.
- Visual indicator: YOLO badge on workflow run header.

---

## 5. Supabase Realtime Integration

```typescript
// src/features/workflows/realtime.ts
import { supabase } from '@/data/supabase/client'

export function subscribeToWorkflowRun(runId: string, onUpdate: (step: WorkflowStep) => void) {
  return supabase
    .channel(`workflow_run_${runId}`)
    .on('postgres_changes', {
      event: 'UPDATE',
      schema: 'public',
      table: 'workflow_run_steps',
      filter: `workflow_run_id=eq.${runId}`,
    }, (payload) => {
      onUpdate(payload.new as WorkflowStep)
    })
    .subscribe()
}
```

Use this in a custom hook:
```typescript
// src/features/workflows/use-workflow-realtime.ts
export function useWorkflowRealtime(runId: string) {
  const queryClient = useQueryClient()

  useEffect(() => {
    const channel = subscribeToWorkflowRun(runId, (updatedStep) => {
      console.log(`[Realtime] Workflow step ${updatedStep.step_type} updated status to: ${updatedStep.status}`)
      // Invalidate the query to refetch latest step statuses
      queryClient.invalidateQueries({ queryKey: ['workflowRun', runId] })
    })
    return () => { supabase.removeChannel(channel) }
  }, [runId, queryClient])
}
```

---

## 6. Reject/Retry Flow (per SD-09 §4)

```
User clicks "Reject & Retry"
    → UI captures rejection_note (required text)
    → Frontend calls Edge Function (NOT direct table write) which updates workflow_run_steps:
        status = 'PENDING'
        rejection_note = user text
        retry_count += 1
      Note: NEVER write execution state to workflow_steps (the definition table).
            workflow_steps is immutable per-run — only workflow_run_steps holds run state.
    → Go-Runner detects workflow_run_steps row is PENDING again
    → Re-assembles prompt with appended rejection context:
        "# Reviewer Feedback (Retry)
         The previous output was rejected. Reason: <rejection_note>
         Please revise your output to address this feedback."
    → AI regenerates → new artifact version saved
    → Step enters WAITING_USER_APPROVAL again
    → Loop until Approved
```

---

## 7. Definition of Done — Phase 4

### Database
- [ ] `workflows.project_id` is nullable so one table can store both global and private workflow definitions
- [ ] `workflow_steps` created for definition-time columns: `provider_override`, `model_override`, `requires_approval`, `is_enabled`
- [ ] `workflow_runs` and `workflow_run_steps` created for execution-time state
- [ ] `workflow_runs` includes `yolo_mode` BOOLEAN column defaulted to false
- [ ] `workflow_run_steps` includes `rejection_note`, `retry_count`, `prompt_cache_id`
- [ ] `workflow_prompt_cache` table created with `config_hash UNIQUE` index and `workflow_id UUID` FK
- [ ] `workflow_run_logs` table created with `workflow_run_step_id UUID` FK → `workflow_run_steps` (not `workflow_steps`)
- [ ] `step_definitions` table seeded with all 17 MVP step types
- [ ] `step_definitions` also supports authenticated workflow-admin writes for adding reusable custom step definitions
- [ ] All 10 built-in workflow templates seeded as `workflows.is_template = true` with ordered `workflow_steps`, using `step_definitions` for step metadata
- [ ] Global workflows use `project_id IS NULL`
- [ ] Private workflows use `project_id = <owning project uuid>`
- [ ] Row Level Security (RLS) enabled and verified on all 6 workflow tables: `workflows`, `workflow_steps`, `workflow_runs`, `workflow_run_steps`, `workflow_prompt_cache`, `workflow_run_logs`
- [ ] RLS policies allow global workflow reads for authenticated users and restrict private workflow writes to owning project members
- [ ] Shared catalogue table `step_definitions` has RLS enabled for authenticated reads and controlled workflow-admin writes
- [ ] CP-06 Artifacts Dependency: `artifact_id` FK dependency to `artifacts(id)` explicitly verified and handled as part of CP-06 alignment

### Domain & Types
- [ ] `WorkflowStepStatus` TypeScript enum uses canonical SD-09 values: `PENDING`, `RUNNING`, `WAITING_USER_APPROVAL`, `DONE`, `FAILED`, `SKIPPED`
- [ ] `WorkflowTemplate` and `WorkflowTemplateStep` entities defined
- [ ] `PromptCacheEntry` entity defined

### UI
- [ ] Workflow Builder: list/create/edit workflows with step configuration (enable/disable, reorder, provider override)
- [ ] Workspace workflow definition index exists at `/workflows`
- [ ] Dedicated create workflow page exists at `/workflows/create`
- [ ] Step definition catalog exists at `/workflow-steps`
- [ ] Dedicated create step page exists at `/workflow-steps/create`
- [ ] Workflow run history page exists at `/workflow-runs`
- [ ] Project workflow page lists global workflows plus private workflows owned by the active project
- [ ] Saving from the project workflow page creates or updates private workflows for the active project only
- [ ] Workflow Builder can load and apply all 10 built-in templates from seeded template workflows
- [ ] Execution Dashboard: real-time step status via Supabase Realtime hook utilizing the `updatedStep` payload
- [ ] Step status badges for all 6 canonical states: `PENDING` (gray) | `RUNNING` (blue pulse) | `WAITING_USER_APPROVAL` (yellow) | `DONE` (green) | `FAILED` (red) | `SKIPPED` (gray strikethrough)
- [ ] Approval Gate UI: "Approve & Continue" and "Reject & Retry" with rejection note capture
- [ ] YOLO mode toggle at workflow level with visual badge on run header
- [ ] YOLO mode status updates go through a secure Edge Function `/workflow-runs/toggle-yolo` with permission checks
- [ ] Prompt file link visible in Execution Dashboard step detail (reads from `workflow_prompt_cache`)
- [ ] Cache hash displayed in Workflow Builder after save
- [ ] Project detail page exposes an entry point to trigger workflows or create a private workflow

### Reject/Retry
- [ ] Reject flow writes to `workflow_run_steps` via Edge Function (never direct client write to `workflow_steps`)
- [ ] Rejection note stored, retry count incremented on each retry
- [ ] Go-Runner appends rejection note to prompt on re-execution

### Go-Runner
- [ ] `computeConfigHash()` implemented as deterministic SHA256 per §2.1.2
- [ ] Cache-first execution loop: check `workflow_prompt_cache` → reuse or generate → save
- [ ] Runtime placeholder injection for `{{mcp_context}}`, `{{user_context}}`, `{{previous_artifacts}}`, `{{rejection_note}}`
- [ ] `/built-in-workflow/` directory created at FlowPilot system root

### Known Dependencies (deferred)
- [ ] User text context input surface for per-run context (SD-05 §3 `# User Context`) — to be addressed in CP-09 or CP-11
- [ ] Prompt Memory sections (`# Selected Working Memory`, `# Source Artifacts`, `# Raw Artifact Excerpts`) defined in SD-05 §3 are intentionally deferred from CP-07 runtime placeholders. These require the Artifact Working Memory system and Context Resolver — to be addressed in a later CP alongside SS-07/SD-07.
