# CP-04: Workflow Engine UI & Execution Dashboard

**Maps from:** SD-05 (Workflow Engine), SD-09 (Approval Gates), SS-04 (Workflow Spec)
**Phase:** 4
**Depends on:** CP-03

---

## 1. Core Concept

This phase builds the **Workflow Builder** (design-time) and **Execution Dashboard** (run-time) that allow users to construct custom workflows from the 17 MVP steps defined in SS-04, then monitor their execution in real-time.

---

## 2. Database — Align Existing Schema

The existing migration already has `workflow_definitions`, `workflow_runs`, `workflow_steps`, `ai_outputs`, `approvals`, and `approval_decisions`. We need to extend them to support the new features:

```sql
-- Extend workflow_steps for prompt caching, rejection, and skip support
ALTER TABLE workflow_steps ADD COLUMN IF NOT EXISTS rejection_note TEXT;
ALTER TABLE workflow_steps ADD COLUMN IF NOT EXISTS retry_count INT DEFAULT 0;
ALTER TABLE workflow_steps ADD COLUMN IF NOT EXISTS prompt_cache_id TEXT;
ALTER TABLE workflow_steps ADD COLUMN IF NOT EXISTS provider_override TEXT;
ALTER TABLE workflow_steps ADD COLUMN IF NOT EXISTS model_override TEXT;
ALTER TABLE workflow_steps ADD COLUMN IF NOT EXISTS requires_approval BOOLEAN DEFAULT true;
ALTER TABLE workflow_steps ADD COLUMN IF NOT EXISTS is_enabled BOOLEAN DEFAULT true;

-- Workflow prompt cache (per SD-05 §7.2)
CREATE TABLE workflow_prompt_cache (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  workflow_definition_id TEXT NOT NULL REFERENCES workflow_definitions(id),
  config_hash VARCHAR NOT NULL,
  file_path VARCHAR NOT NULL,
  step_type TEXT NOT NULL,
  provider TEXT NOT NULL,
  is_valid BOOLEAN DEFAULT true,
  created_at TIMESTAMPTZ DEFAULT now(),
  invalidated_at TIMESTAMPTZ
);

CREATE INDEX idx_prompt_cache_hash ON workflow_prompt_cache(config_hash) WHERE is_valid = true;

-- Enable RLS
ALTER TABLE workflow_prompt_cache ENABLE ROW LEVEL SECURITY;
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

### 2.1.4 Runtime Placeholder Injection
The cached `.md` contains structural template sections. At execution time, the Go-Runner fills:
```markdown
# MCP Context
{{mcp_context}}          ← Jira ticket data, Firebase crash logs, etc.

# User Context
{{user_context}}          ← Manual text from context_sources

# Previous Artifacts
{{previous_artifacts}}    ← Output from earlier steps in this run

# Reviewer Feedback (Retry)
{{rejection_note}}        ← Only populated on retry (from workflow_steps.rejection_note)
```

This means the **structural template is cached**, but **run-specific data is always fresh**.

### 2.1.5 Cache Invalidation Rules
The cached `.md` is **regenerated** (old record marked `is_valid = false`) when:
- User modifies the workflow (adds/removes/reorders/enables/disables steps)
- A skill file (built-in or custom) is updated
- Provider or model override changes

Any of these changes cause the config_hash to change → new file generated → new DB record.

### 2.1.6 Admin Web Cache Visibility
- **Workflow Builder:** After save, display: "This configuration maps to cache hash: `a3f8c2`"
- **Execution Dashboard:** Step detail shows: "Prompt file: `/built-in-workflow/a3f8c2_tech_spec.md`" (clickable to view the actual assembled prompt)
- **Settings:** "Clear Prompt Cache" button to invalidate all cached files for a workflow

---

## 3. Domain Model Updates

### 3.1 Update Status Constants
```typescript
// src/domain/constant/status.ts (UPDATED)
export type WorkflowStepStatus =
  | 'pending'
  | 'running'
  | 'waiting_approval'
  | 'completed'
  | 'rejected'
  | 'failed'
  | 'skipped';   // ← NEW per SD-09
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
  workflowDefinitionId: string;
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

**Built-in Templates** (per SS-04 §5):
| Template | Persona | Steps |
|----------|---------|-------|
| Bug Fix Flow | Developer | Traceability → Issue Analysis → Tech Spec → Plan → Code/Review → Release |
| Pre-defined Feature | Developer | Tech Spec → Plan → Architecture → TDD → Code/Review → Release → Notify |
| Full End-to-End | Solo Dev | Business Idea → Feature Intake → Business Summary → Product Spec → Tech Spec → Plan → Arch → TDD → Code/Review → Release → Notify |
| Fast-Track Business | Solo Dev | Product Spec → Tech Spec → Plan → Arch → TDD → Code/Review → Release |
| Task Breakdown | Leader | Tech Spec → Plan → Task Breakdown |
| Root Cause Analysis | Leader | Traceability → Issue Analysis → Task Breakdown → Notify |

### 4.2 Execution Dashboard (`/projects/:projectId/workflows/:runId`)

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
- Toggle at workflow level (not per-step in YOLO mode)
- When enabled: all approval gates are skipped, pipeline runs continuously
- Visual indicator: YOLO badge on workflow run header

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
      table: 'workflow_steps',
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
    const channel = subscribeToWorkflowRun(runId, () => {
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
    → Frontend writes to workflow_steps:
        status = 'pending'
        rejection_note = user text
        retry_count += 1
    → Go-Runner detects step is PENDING again
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

- [ ] Workflow Builder: list/create/edit workflows with step configuration
- [ ] Built-in workflow templates loaded from seed data
- [ ] Execution Dashboard: real-time step status via Supabase Realtime
- [ ] Step status badges with all 7 states (PENDING, RUNNING, WAITING_APPROVAL, COMPLETED, REJECTED, FAILED, SKIPPED)
- [ ] Approval Gate UI: approve/reject with rejection note capture
- [ ] YOLO mode toggle at workflow level
- [ ] Reject/Retry loop: rejection note stored, retry count incremented
- [ ] Supabase Realtime subscription for live updates
- [ ] `workflow_prompt_cache` table created with `config_hash` index
- [ ] Go-Runner: `computeConfigHash()` function implemented
- [ ] Go-Runner: cache-first execution loop (check cache → reuse or generate → save)
- [ ] Go-Runner: runtime placeholder injection for MCP/user context and rejection notes
- [ ] `/built-in-workflow/` directory created at system root
- [ ] Admin Web: cache hash display in Workflow Builder + prompt file link in Execution Dashboard
