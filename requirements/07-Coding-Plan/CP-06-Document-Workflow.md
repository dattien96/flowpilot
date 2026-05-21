# CP-06: Document Workflow - Business Logic -> Tech Spec -> Coding Plan

**Maps from:** SS-04 (Workflow Steps), SS-07 (Artifacts), SS-08 (Approval Gates), SS-09 (Artifact Memory), SD-05 (Workflow Engine), SD-08 (Artifact Management), SD-09 (Approval Gates), SD-10 (Context Resolver)
**Phase:** 3
**Depends on:** CP-07

---

## 1. Core Concept

This phase implements the document pipeline:

```text
Business Logic -> Tech Spec -> Coding Plan
```

The document pipeline must use the same pattern as workflows and steps:

- reusable definition layer
- runtime execution layer

For artifacts this means:

1. `artifact_definitions`
2. `artifact_runs`

The runner should not treat raw artifact files as anonymous files. It should treat them as runtime instances of predefined artifact definitions.

---

## 2. Product Model

### 2.1 Artifact Definition

Artifact definitions are reusable templates that describe:

- artifact name
- description
- default file name
- local output root path
- local input lookup path
- remote sync root path

Example:

```text
artifact_key            = plan_artifact
name                    = Plan Artifact
description             = Planning output document
default_file_name       = Plan.md
local_output_root_path  = /root/plan_architect
local_input_lookup_path = /root/plan_architect
remote_sync_root_path   = /artifacts/plan_architect
```

### 2.2 Artifact Run

Artifact runs are real generated artifact instances created during workflow execution.

Each artifact run must reference:

- artifact definition
- project
- workflow run
- workflow step run
- resolved local path
- resolved remote path
- sync state

### 2.3 Workflow Step binding

Each workflow step definition may bind:

- zero or more input artifact definitions
- zero or more output artifact definitions

Rules:

- if one or more required input artifact definitions exist, runner must resolve all required local files before executing the step
- if a required input artifact cannot be resolved, the step fails
- if one or more output artifact definitions exist, runner must create artifact-runs after generation
- if output artifact definitions are empty, the step may still run and produce no persistent file

This is required because some built-in steps such as `Telegram Notification Step` do not need output artifacts.

Important implementation note:

- the product model supports multiple input and output artifact bindings now
- the first code slice may temporarily persist only one primary input binding and one primary output binding internally
- that temporary storage shortcut must not be treated as the long-term product constraint

---

## 3. Database Plan

### 3.1 Artifact Definitions Table

```sql
CREATE TABLE artifact_definitions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  artifact_key TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  description TEXT NOT NULL,
  default_file_name TEXT NOT NULL,
  local_output_root_path TEXT NOT NULL,
  local_input_lookup_path TEXT NOT NULL,
  remote_sync_root_path TEXT,
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now()
);
```

### 3.2 Artifact Runs Table

```sql
CREATE TABLE artifact_runs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  artifact_definition_id UUID NOT NULL REFERENCES artifact_definitions(id) ON DELETE RESTRICT,
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  workflow_run_id UUID NOT NULL REFERENCES workflow_runs(id) ON DELETE CASCADE,
  workflow_run_step_id UUID NOT NULL REFERENCES workflow_run_steps(id) ON DELETE CASCADE,
  artifact_name TEXT NOT NULL,
  local_path TEXT NOT NULL,
  remote_path TEXT,
  remote_url TEXT,
  sync_status TEXT NOT NULL DEFAULT 'local_only'
    CHECK (sync_status IN ('local_only', 'queued', 'syncing', 'synced', 'failed')),
  version INT NOT NULL DEFAULT 1,
  status TEXT NOT NULL DEFAULT 'DONE'
    CHECK (status IN ('PENDING', 'RUNNING', 'WAITING_USER_APPROVAL', 'DONE', 'FAILED', 'SKIPPED')),
  created_by UUID REFERENCES auth.users(id),
  approved_by UUID REFERENCES auth.users(id),
  rejection_note TEXT,
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now()
);
```

### 3.3 Step Definition Extension

Extend the step-definition layer to support multi-bindings:

- `step_input_artifact_definitions`
- `step_output_artifact_definitions`

Recommended columns:

- `id UUID PRIMARY KEY`
- `step_type TEXT NOT NULL`
- `artifact_definition_id UUID NOT NULL`
- `order_index INT NOT NULL DEFAULT 0`
- `is_required BOOLEAN NOT NULL DEFAULT true` for inputs
- `created_at TIMESTAMPTZ DEFAULT now()`
- `updated_at TIMESTAMPTZ DEFAULT now()`

These references allow workflow steps to bind to reusable artifact templates without limiting a step to only one input or one output artifact.

---

## 4. Built-in Artifact Definitions for the document flow

This phase must seed predefined artifact definitions for the core document pipeline:

| artifact_key | default_file_name | used by step |
|---|---|---|
| `business_logic_artifact` | `BusinessLogic.md` | Business Idea / Business Summary flow |
| `tech_spec_artifact` | `TechSpec.md` | Tech Spec Step |
| `coding_plan_artifact` | `CodingPlan.md` | Make Plan Coding Step |

Other predefined steps may seed additional artifact definitions when they produce durable outputs.

Example:

- `task_breakdown_artifact`
- `architecture_artifact`
- `root_cause_analysis_artifact`

---

## 5. Runtime Behavior

### 5.1 Output flow

When a step defines one or more output artifact definitions:

1. load each artifact definition
2. resolve final local path per artifact definition
3. generate output file locally for each produced artifact
4. create one artifact-run row per produced artifact
5. mark `sync_status = local_only`

Example runtime output path:

```text
/root/plan_architect/{projectId}/{workflowRunId}/{stepType}/Plan.md
```

### 5.2 Input flow

When a step defines one or more input artifact definitions:

1. load each artifact definition
2. query `artifact_runs` for the latest matching artifact for each requested definition in the current workflow run
3. verify each required file exists at `artifact_runs.local_path`
4. load the file contents

If any required artifact-run does not exist, or the local file is missing, fail the step.

### 5.3 No-input steps

If a step does not define input artifacts:

- skip artifact lookup
- continue normally

### 5.4 No-output steps

If a step does not define output artifacts:

- do not create artifact-run
- continue normally

This supports steps such as `Telegram Notification Step`.

---

## 6. UI Scope

### 6.1 Settings -> Artifacts

Add a new settings page:

- `/settings/artifacts`

This page should manage:

- artifact definition catalog
- storage driver config
- sync defaults
- backup and validation controls

### 6.2 Project -> Artifacts

Add a new project tab:

- `/projects/$projectId/artifacts`

This page should show artifact runs grouped by:

1. workflow run
2. workflow step
3. artifact run

Each artifact run should show:

- artifact definition name
- runtime artifact file name
- local path
- remote URL if synced
- sync status
- timestamps

Actions:

- view file content
- sync artifact
- sync all artifacts in the run

### 6.3 Step create/edit

Workflow step create/edit pages must change from free-text artifact fields to artifact-definition linking:

- input artifact definition multi-select or add-list
- output artifact definition multi-select or add-list

Artifact definition pages must allow defining:

- name
- description
- default file name
- local output root path
- local input lookup path
- remote sync root path

---

## 7. Sync Model

Execution must remain local-first.

Remote sync is a separate concern used for:

- persistent backup
- stakeholder sharing
- audit access outside local machine

Suggested sync states:

- `local_only`
- `queued`
- `syncing`
- `synced`
- `failed`

For MVP:

- manual sync per artifact run
- manual sync per workflow run

Remote sync must not block workflow progression.

---

## 8. Tests

### 8.1 Step definition tests

- create/edit step saves one or more input artifact definition bindings
- create/edit step saves one or more output artifact definition bindings
- input artifacts may be empty
- output artifacts may be empty for side-effect-only steps
- required input artifact bindings fail when any required artifact is unresolved

### 8.2 Artifact definition tests

- create artifact definition
- edit artifact definition
- validate required root paths and file name

### 8.3 Artifact run tests

- output step creates artifact-run with resolved local path
- input step resolves the correct prior artifact-run
- missing input artifact file fails the step
- sync updates remote metadata and sync state

### 8.4 Project artifact browser tests

- lists artifact runs grouped by workflow run
- shows local path and remote status
- sync action works at artifact level and run level

---

## 9. Definition of Done

### Data Layer

- [ ] `artifact_definitions` table exists
- [ ] `artifact_runs` table exists
- [ ] step-definition artifact binding tables exist for multi-input and multi-output relationships

### Workflow Engine

- [ ] output artifact definitions create runtime artifact-run rows
- [ ] input artifact definitions resolve prior local files
- [ ] missing required input artifact fails the step
- [ ] no-output steps can run without artifact-run creation

### UI

- [ ] new `Settings -> Artifacts` page exists
- [ ] project artifact browser exists and groups by workflow run
- [ ] step create/edit pages use artifact-definition multi-selects or add-lists
- [ ] artifact definition create/edit UI exists

### Sync

- [ ] artifact-run tracks `local_path`, `remote_path`, `remote_url`, `sync_status`
- [ ] manual sync is possible per artifact and per run
- [ ] remote sync does not block workflow execution

### Document Flow

- [ ] business logic step uses predefined artifact definition when needed
- [ ] tech spec step uses predefined artifact definition
- [ ] coding plan step uses predefined artifact definition
