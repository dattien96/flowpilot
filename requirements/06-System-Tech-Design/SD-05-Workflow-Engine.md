# FlowPilot Tech Design - Workflow Engine (Core Component)

This is the **most critical technical design document** in FlowPilot. It translates `SS-04-Workflow` into technical implementation details and resolves the fundamental question: **How do we dynamically generate workflow instructions for different AI providers?**

---

## 1. The Core Problem

Today, when a developer uses Claude/Codex/Gemini, they create a static `.md` workflow file (e.g., `CLAUDE.md` or `AGENTS.md`) that contains fixed instructions like "plan → code → test → review". 

FlowPilot sits **on top of** these static files. Our system allows users to dynamically combine, reorder, enable/disable steps to create custom workflows. This creates a fundamental challenge:

> **How do we translate a user's dynamic workflow configuration (stored in Supabase) into the correct `.md` instruction file for whichever AI provider is selected?**

---

## 2. Solution: Dynamic Prompt Assembly by the Go-Runner

The answer is: **The Go-Runner dynamically generates a temporary workflow markdown file at runtime, and feeds it to the AI provider as the prompt/instruction.**

The workflow `.md` files are **NOT pre-generated and saved** inside `.claude/` or `.codex/` folders permanently. Instead:

### 2.1 The Generation Flow

```
User triggers workflow in React UI
        ↓
Supabase stores: workflow_run + ordered workflow_run_steps
  - workflow_runs carries run-level state, project_id, provider/model, yolo_mode, started_by, and error_message
  - workflow_run_steps carries execution-time step state, prompt cache link, artifact link, rejection_note, and retry_count
        ↓
Go-Runner picks up PENDING step
        ↓
Go-Runner assembles the prompt:
  1. Load the Step's built-in SKILL.md template
  2. Load any custom SKILL.md files attached to this step
  3. Load the MCP context data (Jira ticket, Figma link, etc.)
  4. Resolve prompt memory from artifact working memory, previous approved outputs, pinned notes, and retrieval policy
  5. Inject the user's text-based context
        ↓
Go-Runner checks: does a cached .md already exist for this config?
  - YES → reuse /built-in-workflow/<hash>_<step_type>.md
  - NO  → assemble a new .md and save it to /built-in-workflow/
        ↓
Go-Runner invokes the AI provider CLI with this file:
  - Claude: `claude --print < /built-in-workflow/<hash>_<step_type>.md`
  - Codex: `codex exec --prompt-file /built-in-workflow/<hash>_<step_type>.md`
  - Gemini: `gemini < /built-in-workflow/<hash>_<step_type>.md`
        ↓
AI processes and returns output
        ↓
Go-Runner parses output → saves as Artifact → updates step status
        ↓
Go-Runner moves to next PENDING step (or pauses for Approval Gate)
```

### 2.2 What Gets Saved and Where

| Item | Saved? | Where? |
|------|--------|--------|
| Workflow definition (steps, order) | ✅ Yes | Supabase `workflows` + `workflow_steps` tables |
| Step SKILL.md templates (built-in) | ✅ Yes | Bundled in Go-Runner binary or synced from backend |
| Custom SKILL.md files | ✅ Yes | `.claude/skills/`, `.codex/skills/`, `.gemini/skills/` in project dir |
| **Assembled workflow prompt .md** | **✅ Yes (cached)** | **`/built-in-workflow/` folder in system root** |
| Output artifacts | ✅ Yes | Supabase Storage + local `.artifacts/` folder |
| Execution logs | ✅ Yes | Supabase `workflow_run_logs` table |

### 2.3 Workflow File Caching Strategy (`/built-in-workflow/`)

Assembled prompt files are **saved persistently** in the `/built-in-workflow/` folder at the FlowPilot system root — **NOT inside LLM provider folders** (`.claude/`, `.codex/`, `.gemini/`). This gives us:

1. **Reusability:** If the same workflow config (same steps, same order, same enabled/disabled flags) is triggered again, the Go-Runner skips regeneration and reuses the cached `.md` file.
2. **Auditability:** Developers and Leaders can inspect exactly what prompt was sent to the AI for any workflow run.
3. **Debuggability:** When a workflow produces unexpected output, you can read the exact `.md` file that was used.
4. **No LLM folder pollution:** LLM folders (`.claude/`, `.codex/`, `.gemini/`) stay clean with only project-level instructions and skill files. Workflow prompt files live in our own system folder.

#### Cache Key (Config Hash)
The Go-Runner generates a deterministic hash from the workflow configuration to uniquely identify each prompt file:
```
hash = SHA256(
  workflow_id +
  step_type +
  order_index +
  is_enabled flags +
  provider +
  model +
  skill_ids +
  custom_skill_content_hashes
)
```

#### File Naming Convention
```
/built-in-workflow/<hash>_<step_type>.md
```
Example: `/built-in-workflow/a3f8c2_tech_spec.md`

#### Cache Invalidation
The cached `.md` file is **regenerated** when:
- The user modifies the workflow (adds/removes/reorders/enables/disables steps).
- A skill file (built-in or custom) is updated.
- The provider or model override changes.

When any of these change, the config hash changes → the Go-Runner generates a new file and creates a new record in Supabase.

**Important:** MCP context, user text context, and prompt memory are injected at runtime into the cached template. The cached `.md` contains placeholder sections (`# MCP Context`, `# User Context`, `# Selected Working Memory`, `# Source Artifacts`, `# Raw Artifact Excerpts`) that the Go-Runner fills dynamically before passing to the AI provider. This means the structural template is cached, but run-specific data is always fresh.

### 2.4 Why NOT Save in LLM Provider Folders?

1. **Separation of concerns:** LLM folders are for the provider's own configuration. FlowPilot's workflow files belong to our system.
2. **Provider portability:** The same workflow step might use Claude today and Gemini tomorrow. Storing in a provider-specific folder creates coupling.
3. **Clean project root:** Users who commit `.claude/` or `.gemini/` to Git won't accidentally include hundreds of workflow prompt files.

---

## 3. Prompt Assembly Template

When the Go-Runner assembles the prompt for a step, it follows this structure:

```markdown
# System Instruction
You are a [Agent Role] working on project [Project Name].
Provider: [Claude/Codex/Gemini]
Model: [model-version]

# Workflow Context
This is Step [N] of [Total] in the "[Workflow Name]" workflow.
Previous steps completed: [list of completed steps and their artifact summaries]

# Skills & Rules
[Contents of built-in SKILL.md for this step type]
[Contents of any custom SKILL.md files attached]

# MCP Context
[Jira ticket data, if loaded]
[Figma design data, if loaded]
[Other MCP data]

# User Context
[User-provided text, uploaded files, pasted links]

# Selected Working Memory
[Structured summaries, decisions, constraints, and source references selected by the Context Resolver]

# Source Artifacts
[Links/references to raw artifact files used as durable memory]

# Raw Artifact Excerpts
[Only included when required by step policy or when summary memory is insufficient]

# Task
Execute the [Step Name] according to the skills and rules above.
Output your result as a structured Markdown artifact.
```

---

## 4. Provider-Specific Execution

The Go-Runner adapts the execution command based on the resolved provider:

### 4.1 Claude
```bash
# Option A: Pipe prompt via stdin
claude --print --model claude-sonnet-4 < /built-in-workflow/a3f8c2_tech_spec.md

# Option B: Use claude with explicit instruction file
claude --print --system-prompt "$(cat /built-in-workflow/a3f8c2_tech_spec.md)"
```

### 4.2 Codex
```bash
# Use exec mode for non-interactive execution
codex exec --model codex-5.5 --prompt "$(cat /built-in-workflow/a3f8c2_tech_spec.md)"

# Or with full-auto for autonomous coding steps
codex exec --full-auto --model codex-5.3 --prompt "$(cat /built-in-workflow/a3f8c2_tech_spec.md)"
```

### 4.3 Gemini
```bash
# Pipe prompt via stdin
gemini --model gemini-2.5-pro < /built-in-workflow/a3f8c2_tech_spec.md
```

### 4.4 Special Case: Code/Review Loop Step
This step is unique because it requires an **interactive agent** that can write code, compile, run tests, and self-review in a loop. For this step:

1. The Go-Runner spawns a **long-running sub-process** using the provider's interactive/auto mode:
   - Claude: `claude --dangerously-skip-permissions` (or with approved tools)
   - Codex: `codex exec --full-auto`
2. The sub-process receives the TDD signatures + Architecture + Coding Plan as context.
3. The sub-process autonomously codes, compiles, tests, and reviews until exit criteria are met (build passes, tests pass, coverage >= 80%).
4. On exit, the Go-Runner collects the output diff and review report as artifacts.

---

## 5. Persistent Files That DO Live in LLM Folders

While the workflow prompts are temporary, these files ARE saved permanently in the project directory:

### 5.1 Project-Level Instruction File
The Go-Runner generates and maintains a single `CLAUDE.md` / `AGENTS.md` / `GEMINI.md` in the project root. This file contains:
- Project name and description
- Directory structure overview
- Coding conventions
- Links to active skills

This file is **NOT the workflow**. It is the "project onboarding" context that every AI session loads automatically.

### 5.2 Skill Files
Built-in and custom skills live permanently in the provider-specific skill directories:
```
project-root/
├── .claude/skills/planning/SKILL.md
├── .claude/skills/architecture/SKILL.md
├── .claude/skills/tdd/SKILL.md
├── .claude/skills/coding/SKILL.md
├── .claude/skills/review/SKILL.md
├── .codex/skills/...   (same structure for Codex)
├── .gemini/skills/...   (same structure for Gemini)
```

The Go-Runner reads from these folders when assembling the runtime prompt.

---

## 6. UI/UX Design (Frontend - React)

- **Workflow Definition Index:** A workspace page that lists workflow definitions, not workflow runs. It shows built-in global templates and private project-owned workflows, with filters by scope and project.
- **Create Workflow Page:** A dedicated page for adding a new workflow definition by choosing an owner project, selecting step definitions, and ordering them.
- **Workflow Builder Detail Page:** A dedicated workflow detail route owns the composer for one selected workflow definition.
  - Ability to enable/disable steps, re-order them, and add/remove steps.
  - Per-step controls for approval gate, provider override, and model override.
  - After save, display the generated prompt cache hash when available.
- **Step Definition Index:** A workspace page that shows the full list of reusable step definitions.
- **Create Step Page:** A dedicated page for creating reusable step definitions with `step_type`, `name`, `description`, `required_mcps`, `required_skills`, and `agent_type`.
- **Templates Menu:** Pre-defined templates for Developers, Solo Devs, Leaders, and PMs.
- **Execution Dashboard:** A screen to start a workflow and view its real-time progress across steps. Each step shows: status badge (`PENDING`, `RUNNING`, `WAITING_USER_APPROVAL`, `DONE`, `FAILED`, `SKIPPED`), elapsed time, provider/model, prompt file link, and a link to view the generated artifact.
- **Workflow Run History Page:** A workspace page that shows workflow run history and supports filtering by workflow type, global-vs-private scope, and project.
- **Realtime Updates:** The frontend subscribes to `workflow_run_steps` updates for the active run and refreshes dashboard data from the updated step payload.
- **Approval Gate UI:** When a step enters `WAITING_USER_APPROVAL`, show artifact preview plus "Approve & Continue" and "Reject & Retry". Reject requires a rejection note and retries the same `workflow_run_steps` row.
- **YOLO Mode:** Persisted per run on `workflow_runs.yolo_mode`. The dashboard header exposes the toggle and visual badge. Toggle mutations must go through a trusted Edge Function such as `/workflow-runs/toggle-yolo`, limited to project owners, leaders, or the user who started the run.
- **Project Trigger Entry:** The project detail workflow tab acts as an entry page, not as an embedded builder. It must expose buttons to browse workflow definitions, create a new private workflow, and review recent run history for that project.
- **List-to-Builder Flow:** Workflow definition cards in the workspace list must navigate to the dedicated workflow detail/builder route so users can open, inspect, compose, and launch from the item they selected.

### 6.1 Built-In Templates and Private Workflows

Built-in templates are stored as seeded `workflows` rows with `is_template = true`, with their ordered steps stored in `workflow_steps`. Private workflows are also stored in `workflows`, but are owned by one project via `workflows.project_id`. `step_definitions` is the shared catalogue for the 17 MVP step types plus any later custom reusable step definitions, and provides names, descriptions, required MCPs, required skills, and agent type.

| Template | Persona | Steps |
|----------|---------|-------|
| Bug Fix Flow | Developer | Traceability -> Issue Analysis -> Tech Spec -> Plan -> Code/Review -> Release |
| Pre-defined Feature | Developer | Tech Spec -> Plan -> Architecture -> TDD -> Code/Review -> Release -> Notify |
| Bug Traceability | Developer | Code Traceability |
| Onboarding | Developer | Onboarding Walkthrough |
| Full End-to-End | Solo Dev | Business Idea -> Feature Intake -> Business Summary -> Product Spec -> Tech Spec -> Plan -> Arch -> TDD -> Code/Review -> Release -> Notify |
| Fast-Track Business | Solo Dev | Product Spec -> Tech Spec -> Plan -> Arch -> TDD -> Code/Review -> Release |
| Task Breakdown | Leader | Tech Spec -> Plan -> Task Breakdown |
| Root Cause Analysis | Leader | Traceability -> Issue Analysis -> Task Breakdown -> Notify |
| Analytics & Usage | Leader | Analytics Review |
| Product Process & Analysis | PM/Owner | Business Idea -> Feature Intake -> Business Summary -> Product Spec -> Project Analysis -> Analytics Review |

## 7. Database Design (Supabase)

### 7.1 Workflow Definition Tables
- `workflows` table: `id`, `project_id`, `name`, `description`, `is_template`, `provider_override`, `model_override`, `created_by`, `created_at`, `updated_at`.
- `workflow_steps` table: `id`, `workflow_id`, `step_type` (enum for the 17 MVP steps), `order_index`, `is_enabled`, `provider_override`, `model_override`, `requires_approval`, `created_at`, `updated_at`.
- `workflow_steps` stores definition-time configuration only. Execution state must never be written here.
- `workflows.project_id = null` means a global reusable workflow definition.
- `workflows.project_id = <project uuid>` means a private workflow owned by exactly one project.
- Seed the 10 built-in templates from SS-04 as `workflows.is_template = true` plus ordered `workflow_steps` rows, with `project_id = null`.

### 7.2 Workflow Prompt Cache Table
- `workflow_prompt_cache` table:
  - `id` (UUID)
  - `workflow_id` (FK → workflows)
  - `config_hash` (VARCHAR, unique) — SHA256 of the workflow config (steps + order + enabled + provider + model + skills)
  - `file_path` (VARCHAR) — relative path inside `/built-in-workflow/`, e.g. `a3f8c2_tech_spec.md`
  - `step_type` (ENUM) — which step type this prompt is for
  - `provider` (ENUM) — claude / codex / gemini
  - `is_valid` (BOOLEAN, default true) — set to `false` when config changes invalidate this cache entry
  - `created_at` (TIMESTAMP)
  - `invalidated_at` (TIMESTAMP, nullable)

**Usage by Go-Runner:**
1. Before assembling a prompt, query: `SELECT file_path FROM workflow_prompt_cache WHERE config_hash = ? AND is_valid = true`
2. If found → reuse the existing `.md` file at `/built-in-workflow/<file_path>`.
3. If NOT found → assemble new prompt → save to `/built-in-workflow/` → INSERT new record.
4. When user modifies workflow config → UPDATE all matching records: `SET is_valid = false, invalidated_at = NOW()`.

### 7.3 Workflow Execution Tables
- `workflow_runs` table: `id`, `workflow_id`, `project_id`, `status` (`PENDING`/`RUNNING`/`DONE`/`FAILED`/`CANCELED`), `provider`, `model`, `yolo_mode`, `started_by`, `started_at`, `finished_at`, `error_message`.
- `workflow_run_steps` table: `id`, `workflow_run_id`, `workflow_step_id`, `step_type`, `status` (`PENDING`/`RUNNING`/`WAITING_USER_APPROVAL`/`DONE`/`FAILED`/`SKIPPED`), `artifact_id`, `prompt_cache_id` (FK -> workflow_prompt_cache), `rejection_note`, `retry_count`, `started_at`, `finished_at`, `error_message`.
- `workflow_run_logs` table: `id`, `workflow_run_step_id`, `log_level`, `message`, `created_at`.
- `artifact_id` points to canonical artifacts when the artifacts table exists; CP-06 owns final FK alignment.
- `workflow_runs.project_id` is always the actual project where execution happens, even when the selected workflow definition is global.

### 7.4 Step Definition Table
- `step_definitions`: `step_type`, `name`, `description`, `required_mcps` (JSON array), `required_skills` (JSON array), `agent_type`.
- `step_definitions` must seed all 17 MVP step types from SS-04.
- Built-in seeded rows and later custom reusable rows live in the same table.
- The admin workflow UI may create additional reusable step definitions through a dedicated create-step page.

### 7.5 Row-Level Security
- Enable RLS on `workflows`, `workflow_steps`, `workflow_runs`, `workflow_run_steps`, `workflow_prompt_cache`, `workflow_run_logs`, and `step_definitions`.
- Global workflows are readable by authenticated users.
- Private workflows are readable/writable only by members of the owning project team.
- Workflow run tables are scoped to the project team of `workflow_runs.project_id`.
- `step_definitions` allows authenticated read plus controlled admin writes for creating reusable custom step definitions.
- Privileged state transitions, including YOLO toggles and approval/retry updates, must go through Edge Functions or the Go-Runner API so transition rules are validated server-side.

## 8. Go-Runner Step Execution Algorithm

```
function executeWorkflowRun(runId):
    run = fetchWorkflowRun(runId)
    if run.status == CANCELED:
        stop execution

    steps = fetchOrderedSteps(runId)  // from Supabase, ordered by order_index
    
    for each step in steps:
        run = fetchWorkflowRun(runId)  // refresh yolo_mode/cancel state before each step
        if run.status == CANCELED:
            stop execution

        if step.is_enabled == false:
            skip → mark as SKIPPED
            continue

        while step.status == PENDING:
            // 1. Validate prerequisites
            requiredMcps = getStepDefinition(step.step_type).required_mcps
            projectMcps  = getProjectContexts(run.project_id)
            if missing MCP → FAIL step with error "Missing required MCP: Jira"

            // 2. Resolve provider/model
            provider = step.provider_override ?? run.provider ?? project.default_provider
            model    = step.model_override ?? run.model ?? project.default_model

            // 3. Check prompt cache
            configHash = computeHash(step, provider, model, skills)
            cached = queryPromptCache(configHash)  // SELECT FROM workflow_prompt_cache

            if cached != null AND cached.is_valid:
                // 4a. Reuse cached workflow .md (inject runtime context)
                promptFile = "/built-in-workflow/" + cached.file_path
                promptFile = injectRuntimeContext(promptFile, previousArtifacts, mcpData, userContext)
            else:
                // 4b. Assemble new prompt and save to /built-in-workflow/
                prompt = assemblePrompt(step, skills)  // structural template only
                promptFile = saveToBuiltInWorkflow(configHash, step.step_type, prompt)
                insertPromptCache(configHash, promptFile, step.step_type, provider)
                promptFile = injectRuntimeContext(promptFile, previousArtifacts, mcpData, userContext)

            // 5. Resolve prompt memory
            promptMemory = resolveContext(step, mcpData, userContext)
            promptFile = injectPromptMemory(promptFile, promptMemory)

            // 6. Execute via provider CLI
            output = executeProvider(provider, model, promptFile)

            // 7. Parse output → save artifact and working memory
            artifact = parseAndSaveArtifact(output, step)
            generateArtifactMemory(artifact)

            // 8. Check approval gate
            if step.requires_approval AND NOT run.yolo_mode:
                updateStatus(step, WAITING_USER_APPROVAL)
                approval = waitForApproval()  // Edge Function returns approve/reject
                if approval == rejected:
                    store rejection_note, increment retry_count, mark step PENDING
                    step = refetchStep(step.id)
                    continue

            // 9. Mark step as DONE
            updateStatus(step, DONE)
            previousArtifacts.append(artifact)
            break
```
