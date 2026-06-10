# Task-028: Step YOLO Override

## Metadata

- Document ID: `Task-028`
- Title: `Step YOLO Override`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-06-10`
- Last Updated: `2026-06-10`
- Parent Documents: `CP-29-MCP-Proxy-Google-Drive`, `SD-11-MCP-Connection-Flows`, `SD-09-Approval-Gates`, `SS-04-Workflow`, `SS-05-Workflow-Ai-Provider`
- Child Documents: `none`
- Related Documents: `Task-010-Yolo-Mode`, `Task-025-Drive-MCP-Auth-Flow`
- Replaces: `none`
- Tags: `workflow-engine`, `yolo`, `approval`, `mcp`

## AI Quick View

### Summary

- Workflow definitions already carry a workflow-level YOLO default.
- Reusable workflow step definitions and workflow-attached steps need YOLO config.
- Workflow-attached step config must take precedence over workflow config when it is explicitly set.

### Current Ask

- Add reusable step YOLO defaults plus workflow-attached step YOLO override support, and enforce `step > workflow` precedence during workflow execution.

### Key Decisions

- `T-1` Store workflow-attached step YOLO as nullable override: `null` inherits workflow, `true` enables YOLO, `false` disables YOLO.
- `T-2` Store reusable step definition YOLO as a nullable default that is copied when the step is added to a workflow.
- `T-3` Keep `workflow_runs.yolo_mode` as the workflow-level run default while runtime sends the effective per-step YOLO value to the provider session.
- `T-4` Do not reintroduce YOLO editing on an in-progress run detail page.

### Constraints

- Preserve existing workflow-level YOLO behavior for workflows with no step overrides.
- Avoid moving approval control back into Codex host approval prompts.
- Keep this task scoped to workflow and step definition config plus runtime precedence.

### Open Questions

- None.

### Source Refs

- `CP-29-MCP-Proxy-Google-Drive`
- `SD-11-MCP-Connection-Flows`
- `SD-09-Approval-Gates`
- `SS-04-Workflow`
- `SS-05-Workflow-Ai-Provider`

## 1. Goal

Add a reusable step definition YOLO default and a workflow-attached step YOLO override so each workflow step can inherit the workflow default, force YOLO on, or force YOLO off.

## 2. Parent Links

- coding plan: `requirements/07-Coding-Plan/priority/CP-29-MCP-Proxy-Google-Drive.md`
- tech design: `requirements/06-System-Tech-Design/SD-11-MCP-Connection-Flows.md`, `requirements/06-System-Tech-Design/SD-09-Approval-Gates.md`
- system spec: `requirements/05-System-Specs/SS-04-Workflow.md`, `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md`
- specific upstream ids: `Task-010`, `Task-025`

## 3. Trigger

Workflow-level YOLO alone is not enough for mixed approval workflows. A workflow may generally run with YOLO enabled while one sensitive MCP step still requires confirmation, or a workflow may generally require approvals while one low-risk step can auto-approve.

## 4. Exact Change

- `T-1` Add nullable `yolo_mode` support to `step_definitions` and `workflow_steps`.
- `T-2` Expose YOLO default controls in step definition create and detail screens.
- `T-3` Expose step YOLO override controls in workflow create and edit screens.
- `T-4` Save and load YOLO fields through Supabase and demo gateways.
- `T-5` Copy `step_definitions.yolo_mode` into new workflow step instances.
- `T-6` Resolve effective runtime YOLO as `workflow_steps.yolo_mode ?? workflows.yolo_mode`.
- `T-7` Keep run detail YOLO read-only.

## 5. Touched Areas

- files: workflow domain model, Supabase mappers/gateway, demo gateway, workflow step create/detail routes, workflow create/detail routes, workflow start runtime, tests
- modules: `admin-web`, `workflow-engine`
- routes: `/workflows/create`, `/workflows/$workflowId`, `/workflow-runs/$runId`
- tables: `step_definitions`, `workflow_steps`

## 6. Acceptance Check

- Creating or editing a reusable step definition can set its YOLO default to inherit, enabled, or disabled.
- Adding a step to a workflow copies the reusable step definition YOLO default.
- Creating or editing a workflow can override each step YOLO to inherit, enabled, or disabled.
- A step override of enabled sends YOLO true even when workflow YOLO is false.
- A step override of disabled sends YOLO false even when workflow YOLO is true.
- Missing step override preserves workflow-level YOLO behavior.
- Run detail does not allow changing YOLO while a run is in progress.

## 7. Out of Scope

- Per-run YOLO mutation after launch.
- New Codex host approval prompt behavior.
- Changing the Google Drive MCP approval record schema.

## 8. Completion Notes

- result: `implemented reusable step definition YOLO defaults and workflow-attached step YOLO overrides with nullable inherit/on/off semantics`
- follow-ups: `none`
- upstream docs updated: `not required; this task narrows the existing YOLO behavior`
