# FlowPilot Tech Design - Artifact Management

This document translates `SS-07-Workflow-Artifact` into technical implementation details.

## 1. Design Goal

Artifacts must follow the same pattern as workflows and workflow steps:

- reusable definition model
- runtime execution model

This means the system should separate:

1. `artifact_definitions`
2. `artifact_runs`

The runner then links workflow steps to artifact definitions and creates artifact-run records during execution.

## 2. Data Model

### 2.1 Artifact Definitions

`artifact_definitions` stores reusable artifact templates.

Recommended fields:

- `id`
- `artifact_key`
- `name`
- `description`
- `default_file_name`
- `local_output_root_path`
- `local_input_lookup_path`
- `remote_sync_root_path`
- `created_at`
- `updated_at`

Example row:

```text
artifact_key             = plan_artifact
name                     = Plan Artifact
description              = Main plan document produced by planning step
default_file_name        = Plan.md
local_output_root_path   = /root/plan_architect
local_input_lookup_path  = /root/plan_architect
remote_sync_root_path    = /artifacts/plan_architect
```

### 2.2 Artifact Runs

`artifact_runs` stores real generated artifact instances created by workflow execution.

Recommended fields:

- `id`
- `artifact_definition_id`
- `project_id`
- `workflow_run_id`
- `workflow_run_step_id`
- `artifact_name`
- `local_path`
- `remote_path`
- `remote_url`
- `sync_status`
- `version`
- `status`
- `created_by`
- `approved_by`
- `rejection_note`
- `created_at`
- `updated_at`

### 2.3 Step configuration links

Workflow step definitions or workflow-step configuration should store definition-time bindings, not runtime artifact instances.

Recommended structure:

- `step_input_artifact_definitions`
- `step_output_artifact_definitions`

Suggested binding fields:

- `id`
- `step_type`
- `artifact_definition_id`
- `order_index`
- `is_required` for inputs
- `created_at`
- `updated_at`

If the first implementation persists only one primary input binding and one primary output binding in code, that is an interim storage shortcut rather than the intended product model.

## 3. Runtime Path Resolution

Artifact definitions should not store machine-specific final file paths for each run.

Instead:

- definition stores reusable root paths
- runner resolves final path per run

Recommended runtime path format:

```text
{local_output_root_path}/{projectId}/{workflowRunId}/{stepType}/{default_file_name}
```

Example:

```text
/root/plan_architect/project_123/run_456/make_plan_coding/Plan.md
```

The same pattern should apply to remote sync targets:

```text
{remote_sync_root_path}/{projectId}/{workflowRunId}/{stepType}/{default_file_name}
```

## 4. Runner Execution Rules

### 4.1 Output artifact flow

If a step has one or more output artifact bindings:

1. load each bound artifact definition
2. resolve local runtime path for each artifact definition
3. generate each local file
4. insert one artifact-run row per produced artifact
5. set `sync_status = local_only`

If the step does not define output artifacts:

- do not create artifact-run row
- allow execution to complete normally

### 4.2 Input artifact flow

If a step has one or more input artifact bindings:

1. load each bound artifact definition
2. query `artifact_runs` for each matching artifact definition within the current workflow run
3. prefer the latest valid row by version and update time for each requested definition
4. verify each resolved file exists at `artifact_runs.local_path`
5. load resolved file contents as input context

If a required file is missing:

- fail the step
- set workflow step status to `FAILED`
- write error explaining that required input artifact could not be resolved

If the step has no input artifact definitions:

- skip artifact input lookup

### 4.3 Why lookup should use DB first

The runner may validate the file on disk, but the primary lookup should use `artifact_runs`, not a raw folder scan.

Reason:

- faster lookup
- explicit lineage
- better auditability
- avoids ambiguity when multiple files exist

The local file check still matters, but it should happen after finding the expected artifact-run row.

## 5. UI/UX Design

### 5.1 Settings -> Artifacts

This page should own global artifact infrastructure:

- storage driver config
- sync health
- last validation
- backup tools

### 5.2 Project -> Artifacts

This page should list project artifact runs grouped by:

1. workflow run
2. workflow step
3. artifact run

Each item should show:

- artifact definition name
- runtime artifact name
- local path
- remote path or URL
- sync status
- timestamps

### 5.3 Step create/edit

Workflow step create/edit pages should expose:

- input artifact definition multi-select or add-list
- output artifact definition multi-select or add-list

Artifact definition create/edit pages should expose:

- name
- description
- default file name
- local output root path
- local input lookup path
- remote sync root path

## 6. Sync Model

Artifact sync is separate from execution.

Suggested sync states:

- `local_only`
- `queued`
- `syncing`
- `synced`
- `failed`

Sync targets may include:

- Supabase Storage
- Google Drive

Workflow execution must not depend on sync success.

## 7. Predefined Artifact Definitions

Built-in workflow steps that produce files should seed built-in artifact definitions.

Examples:

- `business_idea_artifact`
- `feature_intake_artifact`
- `tech_spec_artifact`
- `coding_plan_artifact`
- `architecture_artifact`

Steps that only send notifications or side effects may have no predefined output artifact definitions.

## 8. Relationship to Approvals and Memory

Artifact approval state belongs to the runtime artifact-run layer, not the artifact definition layer.

Artifact memory generation should consume artifact-run outputs after local file creation succeeds.

That means:

- artifact definition = reusable contract
- artifact run = runtime output record
- artifact memory = searchable summary/index layer

## 9. Context Slot Integration

Workflow step definitions declare their context needs through a dedicated `step_context_slots` table — one row per slot, not a JSONB column.

This allows the Admin UI to display slots as a list with dropdowns, not raw JSON editors.

These two mechanisms serve different purposes:

| Mechanism | Type | Resolver | Behavior |
|---|---|---|---|
| `step_input_artifact_definitions` | Deterministic | `artifact.required` | Must resolve. Fails step if missing. |
| `step_context_slots` table | Flexible | Any resolver type | Optional or required per slot row. |

### 9.1 step_context_slots table

```sql
CREATE TABLE step_context_slots (
  id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  step_definition_id  UUID NOT NULL REFERENCES workflow_step_definitions(id) ON DELETE CASCADE,
  name                TEXT NOT NULL,
  resolver            TEXT NOT NULL,     -- enum: project.brief | run.input | step.previous.brief
                                         --       step.n.brief | artifact.required | semantic.search
                                         --       drive.file | mcp.context | static
  priority            INT  NOT NULL DEFAULT 2, -- 1=always | 2=if budget | 3=trim first
  max_tokens          INT  NOT NULL DEFAULT 1000,
  required            BOOLEAN NOT NULL DEFAULT FALSE,
  order_index         INT  NOT NULL DEFAULT 0,
  resolver_config     JSONB NOT NULL DEFAULT '{}', -- resolver-specific fields (see 9.2)
  created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(step_definition_id, name)
);

CREATE INDEX step_context_slots_step_def_idx ON step_context_slots(step_definition_id);
```

Slot priority controls token budget allocation:
- Priority 1 slots are always included first (e.g. project brief, run input).
- Priority 2 slots are included if budget allows (e.g. previous step, MCP data).
- Priority 3 slots are trimmed or dropped when budget is exceeded (e.g. semantic search).

### 9.2 resolver_config JSONB (per resolver type)

The `resolver_config` column stores only the resolver-specific parameters. Its shape depends on the `resolver` value and is validated by the Admin UI form, not hand-edited by users.

| Resolver | resolver_config fields | UI controls shown |
|---|---|---|
| `project.brief` | `{}` | None |
| `run.input` | `{}` | None |
| `step.previous.brief` | `{}` | None |
| `step.n.brief` | `{ "step_index": 2 }` | Step index number input |
| `artifact.required` | `{ "artifact_key": "tech_spec_artifact" }` | Artifact definition dropdown |
| `semantic.search` | `{ "query_template": "...", "top_k": 3, "use_hyperrag": false }` | Template text input, top_k slider, HyperRAG toggle |
| `drive.file` | `{ "file_path": "docs/arch.md" }` | File path text input |
| `mcp.context` | `{ "mcp_provider": "jira" }` | MCP provider dropdown |
| `static` | `{ "content": "Always respond in English." }` | Multi-line text input |

### 9.3 Template variables for semantic.search

The `query_template` in `resolver_config` supports runtime variable interpolation:

| Variable | Source |
|---|---|
| `{{step.description}}` | Step definition description |
| `{{run.feature_name}}` | Workflow run intake field |
| `{{run.goal}}` | Workflow run intake field |
| `{{project.name}}` | Project record name |
| `{{artifact.type}}` | Current step's primary output artifact key |

### 9.4 Admin UI — Step Context Slots panel

The step create/edit page must include a **Context Slots** panel:

- Lists existing slots as rows (name, resolver type badge, priority badge, max_tokens, required indicator)
- **Add Slot** button opens a form:
  - Name (text input)
  - Resolver (dropdown — all 9 resolver types)
  - Priority (dropdown: `1 — Always` / `2 — If budget` / `3 — Trim first`)
  - Max tokens (number input)
  - Required (checkbox)
  - Resolver-specific fields that appear **conditionally** based on selected resolver (see 9.2)
- Rows are reorderable (drag or up/down arrows set `order_index`)
- Delete slot removes the row
- No raw JSON editing exposed to users

### 9.5 Runner processing order

For each step execution, the runner loads `step_context_slots` ordered by `priority ASC, order_index ASC` and resolves each slot:

1. Resolve all `step_input_artifact_definitions` (deterministic, fail-fast).
2. Resolve Priority 1 slots in `order_index` order.
3. Resolve Priority 2 slots in `order_index` order.
4. Resolve Priority 3 slots in `order_index` order.
5. Pack resolved items into the prompt within the token budget.
6. Write audit records to `workflow_prompt_context_items` (one row per resolved slot).



