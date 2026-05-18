# CP-06: AI Orchestration — Prompt Templates & Execution Logs

**Maps from:** SD-06 (AI Provider Integration), SD-07 (Skill/Agent Runtime), SD-08 (Artifact Management), Google Doc §4.9–4.10
**Phase:** 6
**Depends on:** CP-04, CP-05

---

## 1. Core Concept

This phase wires the actual AI execution pipeline. It includes:
1. **AI Prompt Template Registry** — manage reusable prompts for each workflow step type.
2. **Supabase Edge Functions** — server-side AI orchestration (never expose API keys to the browser).
3. **AI Execution Log Viewer** — full auditability for every AI call.
4. **Artifact Management** — storage, versioning, and annotation of generated outputs.
5. **Embedding Support** — shared `generate-embedding` Edge Function used by artifact memory and prompt context retrieval.

---

## 2. Database Tables

### 2.1 AI Prompt Templates
```sql
CREATE TABLE ai_prompt_templates (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  description TEXT,
  category TEXT NOT NULL,           -- business_logic_review, tech_spec_generation, coding_plan_generation,
                                    -- schedule_generation, task_breakdown, risk_detection, test_plan, code_review
  input_schema JSONB DEFAULT '{}',
  output_schema JSONB DEFAULT '{}',
  template_content TEXT NOT NULL,   -- The actual prompt markdown template
  model_preference TEXT,            -- claude, codex, gemini, or null (use project default)
  version INT NOT NULL DEFAULT 1,
  status TEXT NOT NULL DEFAULT 'active',  -- active, archived
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now()
);

ALTER TABLE ai_prompt_templates ENABLE ROW LEVEL SECURITY;
```

### 2.2 AI Runs (already exists as `ai_call_logs`, extend for full audit)
The existing `ai_call_logs` table covers provider/model/tokens/cost. We add an `ai_runs` table for higher-level tracking:

```sql
CREATE TABLE ai_runs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  run_type TEXT NOT NULL,             -- tech_spec_generation, schedule_generation, etc.
  input_payload JSONB NOT NULL,       -- The full prompt input that was sent
  output_payload JSONB,               -- The structured output received (or raw text)
  model_name TEXT NOT NULL,
  triggered_by TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'running',  -- running, success, failed
  error_message TEXT,
  prompt_template_id UUID REFERENCES ai_prompt_templates(id),
  workflow_run_id TEXT REFERENCES workflow_runs(id),
  workflow_step_id TEXT REFERENCES workflow_steps(id),
  created_at TIMESTAMPTZ DEFAULT now(),
  completed_at TIMESTAMPTZ
);

ALTER TABLE ai_runs ENABLE ROW LEVEL SECURITY;
```

### 2.3 Artifact Annotations (from SD-08)
The existing `ai_outputs` table stores content. Add annotations for human collaboration:

```sql
CREATE TABLE artifact_annotations (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  ai_output_id TEXT NOT NULL REFERENCES ai_outputs(id) ON DELETE CASCADE,
  user_id TEXT NOT NULL,
  highlight_range TEXT,               -- e.g., "L15-L22" for line ranges
  note_text TEXT NOT NULL,
  created_at TIMESTAMPTZ DEFAULT now()
);

ALTER TABLE artifact_annotations ENABLE ROW LEVEL SECURITY;
```

---

## 3. Supabase Edge Functions

### 3.1 `generate-tech-spec`
```
POST /functions/v1/generate-tech-spec

Body: {
  projectId: string,
  businessLogicDocId: string,
  promptTemplateId?: string,
  modelOverride?: string
}

Response: {
  aiRunId: string,
  outputId: string,
  content: string
}
```

### 3.2 `generate-coding-plan`
```
POST /functions/v1/generate-coding-plan

Body: {
  projectId: string,
  technicalSpecId: string,
  promptTemplateId?: string,
  modelOverride?: string
}
```

### 3.3 `generate-master-schedule`
```
POST /functions/v1/generate-master-schedule

Body: {
  projectId: string,
  codingPlanId: string,
  teamMembers: TeamMember[],       // fetched from project_teams → team_members
  sprintLength?: number,
  deadlines?: { name: string, date: string }[],
  promptTemplateId?: string
}

Response: {
  aiRunId: string,
  scheduleItems: ScheduleItem[]    // structured JSON, not markdown
}
```

### 3.4 Edge Function Pattern
All Edge Functions follow the same pattern:
1. Validate auth (Supabase Auth JWT)
2. Fetch input data from DB
3. Load prompt template (or use default)
4. Assemble the prompt
5. Call AI provider API (API keys stored in Supabase Vault/env)
6. Parse response
7. Store result in `ai_runs` + `ai_outputs`
8. Return response

```typescript
// supabase/functions/generate-tech-spec/index.ts
import { serve } from 'https://deno.land/std@0.177.0/http/server.ts'
import { createClient } from 'https://esm.sh/@supabase/supabase-js@2'

serve(async (req) => {
  const supabase = createClient(
    Deno.env.get('SUPABASE_URL')!,
    Deno.env.get('SUPABASE_SERVICE_ROLE_KEY')!
  )

  // 1. Validate auth
  const authHeader = req.headers.get('Authorization')!
  const { data: { user } } = await supabase.auth.getUser(authHeader.replace('Bearer ', ''))

  // 2. Fetch inputs, assemble prompt, call AI...
  // 3. Store result in ai_runs + ai_outputs
  // 4. Return response
})
```

### 3.5 `generate-embedding`
This shared Edge Function supports artifact working memory search in CP-09.

```
POST /functions/v1/generate-embedding

Body: {
  text: string
}

Response: {
  embedding: number[],
  dimensions: number
}
```

Implementation requirements:
- use `new Supabase.ai.Session("gte-small")`
- call `session.run(text, { mean_pool: true, normalize: true })`
- validate method and input
- support CORS preflight
- return dimensions for debugging and migration validation

Reference implementation from a previous working Supabase project:

```typescript
// supabase/functions/generate-embedding/index.ts
import { serve } from "https://deno.land/std@0.168.0/http/server.ts";

// CORS headers for cross-origin requests
const corsHeaders = {
  "Access-Control-Allow-Origin": "*",
  "Access-Control-Allow-Headers":
    "authorization, x-client-info, apikey, content-type",
};

serve(async (req: Request) => {
  // Handle CORS preflight requests
  if (req.method === "OPTIONS") {
    return new Response("ok", { headers: corsHeaders });
  }

  try {
    // Only accept POST requests
    if (req.method !== "POST") {
      return new Response(
        JSON.stringify({ error: "Method not allowed" }),
        {
          status: 405,
          headers: { ...corsHeaders, "Content-Type": "application/json" },
        }
      );
    }

    // Parse request body
    const { text } = await req.json();

    // Validate input
    if (!text || typeof text !== "string") {
      return new Response(
        JSON.stringify({ error: "Missing or invalid 'text' field" }),
        {
          status: 400,
          headers: { ...corsHeaders, "Content-Type": "application/json" },
        }
      );
    }

    console.log(`Generating embedding for text: ${text.substring(0, 50)}...`);

    // Create session and generate embedding
    // Note: Session is created per-request to avoid cold start caching issues
    const session = new Supabase.ai.Session("gte-small");

    const result = await session.run(text, {
      mean_pool: true,
      normalize: true,
    });

    // Debug: Log result type and structure
    console.log(`Result type: ${typeof result}, constructor: ${result?.constructor?.name}`);

    // Ensure result is an array
    let embedding: number[];

    if (!result) {
      throw new Error("session.run() returned null or undefined");
    }

    if (Array.isArray(result)) {
      embedding = result;
    } else if (typeof result === "object" && result !== null) {
      // Convert TypedArray (Float32Array, etc.) to regular array
      try {
        embedding = Array.from(result as any);
      } catch (e) {
        // Fallback: try to extract numeric values
        embedding = Object.values(result).filter((v): v is number => typeof v === "number");
      }
    } else {
      throw new Error(`Unexpected result type: ${typeof result}`);
    }

    if (!embedding || embedding.length === 0) {
      throw new Error("Empty embedding array");
    }

    console.log(`Generated embedding with ${embedding.length} dimensions`);
    console.log(`First 5 values: ${embedding.slice(0, 5)}`);

    // Return response
    return new Response(
      JSON.stringify({
        embedding: embedding,
        dimensions: embedding.length,
      }),
      {
        status: 200,
        headers: { ...corsHeaders, "Content-Type": "application/json" },
      }
    );
  } catch (error) {
    console.error("Error generating embedding:", error);
    return new Response(
      JSON.stringify({
        error: "Failed to generate embedding",
        details: error instanceof Error ? error.message : String(error),
      }),
      {
        status: 500,
        headers: { ...corsHeaders, "Content-Type": "application/json" },
      }
    );
  }
});
```

---

## 4. UI Components

### 4.1 AI Prompt Templates (`/settings/prompt-templates`)
- DataTable: Name, Category (badge), Model Preference, Version, Status
- Create/Edit dialog:
  - Name, Description
  - Category (select from predefined list)
  - Template Content (large Markdown editor with placeholder syntax: `{{business_logic}}`, `{{tech_spec}}`, `{{team_members}}`)
  - Input/Output Schema (JSON editor)
  - Model Preference (dropdown)
- Version history: show previous versions for each template

### 4.2 AI Execution Logs (`/ai-runs`)
- DataTable: Run Type, Project, Model, Status (badge), Tokens, Cost, Triggered By, Date
- Click row → detail view:
  - Full input payload (collapsible JSON/Markdown)
  - Full output payload
  - Token usage breakdown
  - Cost estimate
  - Linked workflow run/step (clickable)
- Filter by: project, status, model, date range
- "Retry/Regenerate" button → re-runs with same input

### 4.3 Artifact Viewer (updated from Phase 3)
- Markdown rendering of `ai_outputs.content_markdown`
- **Version History:** Sidebar dropdown to select previous versions
- **Annotations:** Click to highlight text range → add note → saved to `artifact_annotations`
- **Compare versions:** Side-by-side diff of two artifact versions

### 4.4 AI Generation Trigger Buttons
Wire the previously-disabled "Generate from..." buttons (Phase 3):
- Business Logic page → "Ask AI to Review" calls Edge Function
- Tech Spec page → "Generate from Business Logic" calls `generate-tech-spec`
- Coding Plan page → "Generate from Tech Spec" calls `generate-coding-plan`
- Master Schedule page → "Generate Schedule" calls `generate-master-schedule`

All calls go through:
```typescript
// src/features/ai-runs/mutations.ts
export function useGenerateTechSpec() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (input: GenerateTechSpecInput) => {
      const { data, error } = await supabase.functions.invoke('generate-tech-spec', { body: input })
      if (error) throw error
      return data
    },
    onSuccess: (data, variables) => {
      queryClient.invalidateQueries({ queryKey: techSpecKeys.byProject(variables.projectId) })
      queryClient.invalidateQueries({ queryKey: ['aiRuns'] })
    },
  })
}
```

---

## 5. Go-Runner ↔ Admin Web Communication

For **workflow execution** (Phase 4), the Go-Runner drives the pipeline:
- The Go-Runner polls `workflow_steps` for `PENDING` steps.
- It calls the AI provider directly (not through Edge Functions — it has local access).
- It writes results back to `ai_outputs` and updates `workflow_steps.status`.
- The Admin Web receives updates via Supabase Realtime.

For **document-level AI** (this phase), the Admin Web calls Edge Functions directly:
- Edge Functions handle the AI call securely.
- Results are written to `ai_runs` + relevant document table.

This dual-path design keeps the workflow engine independent from the admin UI.

---

## 6. Definition of Done — Phase 6

- [ ] Database migration: `ai_prompt_templates`, `ai_runs`, `artifact_annotations`
- [ ] At least 4 Supabase Edge Functions: tech-spec, coding-plan, schedule, business-review
- [ ] Shared Edge Function: generate-embedding
- [ ] Prompt Template CRUD with Markdown editor
- [ ] AI Execution Log viewer with filtering and detail drill-down
- [ ] All "Generate from..." buttons functional
- [ ] Artifact viewer with version history and annotations
- [ ] Go-Runner ↔ Admin Web integration via Supabase Realtime (read path)
