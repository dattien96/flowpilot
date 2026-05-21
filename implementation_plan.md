# Workflow Artifact Implementation Plan

## Goal
Implement the artifact model described in:
- `requirements/05-System-Specs/SS-07-Workflow-Artifact.md`
- `requirements/06-System-Tech-Design/SD-08-Artifact-Management.md`
- `requirements/07-Coding-Plan/CP-06-Document-Workflow.md`

The current codebase still uses a temporary JSON-column shortcut on `step_definitions`. This plan replaces that shortcut with a real artifact schema and wires the workflow editor, runtime mapping, and artifact UI to the new model.

## Target Model

### Definition layer
Create a reusable artifact catalog table:
- `artifact_definitions`
  - `id`
  - `key` or `name`
  - `description`
  - `local_path_template`
  - `remote_path_template`
  - timestamps

Create step-to-artifact binding tables:
- `step_input_artifact_definitions`
- `step_output_artifact_definitions`

These tables let a step bind multiple input and multiple output artifact definitions.

### Runtime layer
Create a runtime artifact table:
- `artifact_runs`
  - `id`
  - `artifact_definition_id`
  - `workflow_id`
  - `workflow_run_id`
  - `workflow_run_step_id`
  - resolved local path
  - resolved remote path
  - status / sync state
  - timestamps

The runner writes local artifacts first. Remote storage is a sync target, not a runtime dependency.

## Code Changes

### Database / Supabase
- Add a new migration that creates the artifact definition, artifact binding, and artifact run tables.
- Remove the temporary `input_artifact_definitions` / `output_artifact_definitions` JSONB columns from the final schema path.
- Add indexes and foreign keys for workflow/run/step lookups.
- Add RLS policies consistent with the existing workflow tables.

### Domain model
- Replace the `StepDefinition` artifact arrays with explicit input/output artifact binding collections.
- Add artifact definition and artifact run entities.
- Extend workflow run step state to reference generated artifact runs instead of the old generic artifact id.

### Supabase gateway and mappers
- Update `workflow-engine-mappers.ts` to map the new definition and runtime tables.
- Update `SupabaseWorkflowEngineGateway` to:
  - list/save artifact definitions
  - save step artifact bindings
  - create and fetch artifact runs
  - keep workflow/step CRUD behavior intact

### UI
- Update workflow step create/edit pages so artifact bindings are selected from persisted artifact definitions instead of a hardcoded catalog.
- Add the artifact management page under settings for global artifact/storage configuration.
- Add the project artifact browser tab that groups artifacts by workflow run.
- Keep local-first sync actions available from the UI.

### Runtime / runner flow
- Resolve input artifact requirements per step from the bound artifact definitions.
- Fail the step when a required input artifact is missing locally.
- Create one or more artifact run rows when a step produces output artifacts.
- Keep remote sync optional and separate from execution.

## Validation

Run the focused test suites for:
- workflow engine mappers
- workflow engine gateway
- workflow step create/detail pages
- artifact selector / artifact UI
- any new artifact repository or use case tests

Then run `gitnexus_detect_changes()` before commit to verify the blast radius matches the expected workflow/artifact areas.
