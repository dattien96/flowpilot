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

- **Workflow Builder:** A drag-and-drop or list-based UI to construct workflows by combining Steps.
  - Ability to enable/disable steps, re-order them, and add/remove steps.
- **Templates Menu:** Pre-defined templates for Developers, Solo Devs, Leaders, and PMs (e.g., "Bug Fix Flow", "Full End-to-End Flow").
- **Execution Dashboard:** A screen to start a workflow and view its real-time progress across steps. Each step shows: status badge (`PENDING`, `RUNNING`, `WAITING_USER_APPROVAL`, `DONE`, `FAILED`, `SKIPPED`), elapsed time, and a link to view the generated artifact.

## 7. Database Design (Supabase)

### 7.1 Workflow Definition Tables
- `workflows` table: `id`, `project_id`, `name`, `is_template`, `created_at`.
- `workflow_steps` table: `id`, `workflow_id`, `step_type` (enum for the 17 MVP steps), `order_index`, `is_enabled`, `provider_override`, `model_override`, `requires_approval`.

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
- `workflow_runs` table: `id`, `workflow_id`, `status` (PENDING/RUNNING/DONE/FAILED), `provider`, `model`, `started_at`, `finished_at`.
- `workflow_run_steps` table: `id`, `workflow_run_id`, `workflow_step_id`, `status`, `artifact_id`, `prompt_cache_id` (FK → workflow_prompt_cache), `started_at`, `finished_at`, `error_message`.
- `workflow_run_logs` table: `id`, `workflow_run_step_id`, `log_level`, `message`, `timestamp`.

### 7.4 Step Definition Seed Table
- `step_definitions` (seeded/static): `step_type`, `name`, `description`, `required_mcps` (JSON array), `required_skills` (JSON array), `agent_type`.

## 8. Go-Runner Step Execution Algorithm

```
function executeWorkflowRun(runId):
    steps = fetchOrderedSteps(runId)  // from Supabase, ordered by order_index
    
    for each step in steps:
        if step.is_enabled == false:
            skip → mark as SKIPPED
            continue
        
        // 1. Validate prerequisites
        requiredMcps = getStepDefinition(step.step_type).required_mcps
        projectMcps  = getProjectContexts(step.project_id)
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
        if step.requires_approval AND NOT yoloMode:
            updateStatus(step, WAITING_USER_APPROVAL)
            waitForApproval()  // blocks until user approves in UI
        
        // 9. Mark step as DONE
        updateStatus(step, DONE)
        previousArtifacts.append(artifact)
```
