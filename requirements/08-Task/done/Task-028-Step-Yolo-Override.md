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
- Workflow steps also need their own YOLO config.
- Step config must take precedence over workflow config when it is explicitly set.

### Current Ask

- Add step-level YOLO override support and enforce `step > workflow` precedence during workflow execution.

### Key Decisions

- `T-1` Store step YOLO as nullable override: `null` inherits workflow, `true` enables YOLO, `false` disables YOLO.
- `T-2` Keep `workflow_runs.yolo_mode` as the workflow-level run default while runtime sends the effective per-step YOLO value to the provider session.
- `T-3` Do not reintroduce YOLO editing on an in-progress run detail page.

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

Add a step-level YOLO override to workflow definitions so each step can inherit the workflow default, force YOLO on, or force YOLO off.

## 2. Parent Links

- coding plan: `requirements/07-Coding-Plan/priority/CP-29-MCP-Proxy-Google-Drive.md`
- tech design: `requirements/06-System-Tech-Design/SD-11-MCP-Connection-Flows.md`, `requirements/06-System-Tech-Design/SD-09-Approval-Gates.md`
- system spec: `requirements/05-System-Specs/SS-04-Workflow.md`, `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md`
- specific upstream ids: `Task-010`, `Task-025`

## 3. Trigger

Workflow-level YOLO alone is not enough for mixed approval workflows. A workflow may generally run with YOLO enabled while one sensitive MCP step still requires confirmation, or a workflow may generally require approvals while one low-risk step can auto-approve.

## 4. Exact Change

- `T-1` Add nullable `yolo_mode` support to `workflow_steps`.
- `T-2` Expose step YOLO override controls in workflow create and edit screens.
- `T-3` Save and load step YOLO override through Supabase and demo gateways.
- `T-4` Resolve effective runtime YOLO as `step.yolo_mode ?? workflow.yolo_mode`.
- `T-5` Keep run detail YOLO read-only.

## 5. Touched Areas

- files: workflow domain model, Supabase mappers/gateway, demo gateway, workflow create/detail routes, workflow start runtime, tests
- modules: `admin-web`, `workflow-engine`
- routes: `/workflows/create`, `/workflows/$workflowId`, `/workflow-runs/$runId`
- tables: `workflow_steps`

## 6. Acceptance Check

- Creating or editing a workflow can set each step YOLO to inherit, enabled, or disabled.
- A step override of enabled sends YOLO true even when workflow YOLO is false.
- A step override of disabled sends YOLO false even when workflow YOLO is true.
- Missing step override preserves workflow-level YOLO behavior.
- Run detail does not allow changing YOLO while a run is in progress.

## 7. Out of Scope

- Per-run YOLO mutation after launch.
- New Codex host approval prompt behavior.
- Changing the Google Drive MCP approval record schema.

## 8. Completion Notes

- result: `implemented step-level YOLO override with nullable inherit/on/off semantics`
- follow-ups: `none`
- upstream docs updated: `not required; this task narrows the existing YOLO behavior`
