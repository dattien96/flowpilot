# Task-179: Settings Flow Pack Authoring UI

## Metadata

- Document ID: `Task-179`
- Title: `Settings Flow Pack Authoring UI`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-01`
- Last Updated: `2026-07-01`
- Parent Documents: `CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor`
- Child Documents: `Task-180`
- Related Documents: `Task-175`, `Task-177`, `Task-178`
- Replaces: `N/A`
- Tags: `settings-ui, flow-mode, builtin-flow, custom-flow`

## AI Quick View

### Summary

- Add UI affordances for built-in and custom flows.
- Built-ins are read-only and cloneable; user flows are editable.
- Flow Mode becomes generic from the user's perspective: choose or create a flow definition, then run it.

### Current Ask

- Implement Settings UI and client contract updates for built-in flow catalog, clone, and custom flow editing.

### Key Decisions

- `T-1` Built-in flow definitions cannot be edited directly.
- `T-2` User-created cloned flows are fully editable and no longer tied to pack update overwrites.
- `T-3` UI should display behavior IDs and config in a structured way, not hide them behind semantic Go names.

### Constraints

- Depends on `Task-175` for mirror metadata.
- Do not add arbitrary code execution or script upload.
- Do not expose run-data storage changes.

### Open Questions

- Full visual graph editor can be deferred if a form-based editor is faster for MVP.

### Source Refs

- `requirements/07-Coding-Plan/todo/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md`
- `apps/local-runner/internal/agentpack/flow-pack/manifest.yaml`
- `apps/local-runner/internal/agentpack/flow-pack/flows/review-loop.yaml`
- `apps/local-runner/internal/agentpack/flow-pack/flows/rag-harness.yaml`

## 1. Goal

Expose built-in and custom flow definitions in Settings so users can select built-ins, clone them, and author custom flows without modifying runner code.

## 2. Parent Links

- coding plan: `requirements/07-Coding-Plan/todo/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md`
- tech design: `requirements/06-System-Tech-Design/done/SD-19-Agent-Orchestration-Runtime.md`
- system spec: `requirements/05-System-Specs/done/SS-16-Agent-Orchestration.md`
- specific upstream ids: `Task-175`, `Task-177`, `Task-178`

## 3. Trigger

The generic flow engine only becomes useful if users can choose built-ins or create their own definitions from UI rather than asking developers to edit YAML or Go code.

## 4. Exact Change

- `T-1` Add Settings section: `Flows`.
- `T-2` Add built-in flow list:
  - name
  - pack ID/version
  - description
  - selectable modes
  - read-only badge
  - clone action
  - view definition action
- `T-3` Add user flow list:
  - create
  - edit
  - duplicate
  - archive/delete if supported
  - run in Flow Mode
- `T-4` Add clone flow action:
  - creates user-owned editable copy
  - removes mirror overwrite metadata
  - retains source attribution for audit/debugging.
- `T-5` Add flow editor MVP:
  - nodes
  - edges
  - policies
  - behavior refs
  - agent refs
  - prompt/context refs
  - declared control faces
- `T-6` Add validation UI:
  - unknown node refs
  - unknown behavior refs
  - invalid join policy
  - missing agent ref for delegate behavior
  - cap/policy errors
- `T-7` Add API/client contract fields for built-in metadata.
- `T-8` Add tests for read-only built-in display, clone, and user-flow edit.

## 5. Touched Areas

- files:
  - Settings UI components
  - flow contract/client files
  - runner/gateway endpoints for flow catalog and clone
  - definition store client
- modules:
  - desktop/web UI
  - runner HTTP API
  - definition store
- routes:
  - list flows
  - get flow definition
  - clone built-in flow
  - create/update user flow
  - validate flow
- tables:
  - workflow definition tables only

## 6. Acceptance Check

- Built-in Review Loop and RAG Harness appear as read-only.
- Built-ins cannot be edited directly.
- Clone creates editable user flow.
- User can create a custom flow with arbitrary node names and behavior refs.
- Validation errors show before run.
- Flow Mode can run selected custom or built-in flow by `flowRef`.

## 7. Out of Scope

- Pixel-perfect graph editor.
- Marketplace or remote pack installation.
- Arbitrary custom Go behavior upload.

## 8. Completion Notes

- result: implemented (reusing the existing Workflows/Steps screen, per explicit user direction)
- notes: **the earlier "backend implemented; UI not started" state is superseded — see [CA-160](../../../change-audit/CA-160-flow-definitions-migrated-to-workflows-table.md).** The `GET/POST/PUT /client/flows*` local-runner endpoints from the prior pass (CA-159) were dead code: `WorkflowsSettings.tsx` talks directly to Supabase via `getAdminUseCases()` → `SupabaseAdminRepository`, never through local-runner, and were removed. Instead: built-in flows mirror into the existing `workflows`/`workflow_steps` tables (extended with `is_builtin`/`editable`/`cloneable`/pack-identity/policy/node columns), and `WorkflowsSettings.tsx` itself now shows a "Built-in" badge, hides Save/Delete for non-editable rows (replaced with "Clone"), gates the step editor read-only for built-ins, and exposes the new per-step flow-engine fields (Node ID, Behavior ID, Agent ref, Depends on, Join mode, Cohort) — a form-based MVP, matching this task's own "defer a full graph editor" allowance.
- follow-ups: no live browser verification was possible (the desktop app requires real Supabase sign-in with no demo/bypass mode); the migration needs a real apply pass against a live Supabase project.
- upstream docs updated: [CP-42](../../../07-Coding-Plan/todo/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md) progress notes and [CA-160](../../../change-audit/CA-160-flow-definitions-migrated-to-workflows-table.md)
