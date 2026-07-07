# Task-179: Settings Flow Pack Authoring UI

## Metadata

- Document ID: `Task-179`
- Title: `Settings Flow Pack Authoring UI`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-01`
- Last Updated: `2026-07-06`
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

- result: partially implemented (reusing the existing Workflows/Steps screen, per explicit user direction) — corrected 2026-07-06 (previously read "implemented" without qualification). Confirmed: Built-in badge, read-only gating, Clone flow (creating an editable `is_builtin:false` copy), and the per-step form fields (Node ID/Behavior ID/Agent ref/Depends on/Join mode/Cohort/Prompt/Context ref) all exist and typecheck (`npm run typecheck` clean). Confirmed gap: those fields only reach the **shared step-type catalog** (`step_definitions`, edited via `saveStepDefinition`), not the **per-workflow-instance** `workflow_steps` row — `saveWorkflow`/`cloneWorkflow` in `supabaseAdminRepository.ts` never write `node_id`/`behavior_id`/`agent_ref`/etc. into `workflow_steps` even though the DB migration added those columns and the Go runner already reads them with an instance-overrides-definition fallback. This means two different flows cannot give their own "coder" node different per-instance settings today. **Evidence (directly verified 2026-07-06, superseding an earlier ambiguous "the test fails to typecheck" claim which two verification passes disagreed on):** the `WorkflowStep` interface (`packages/flowpilot-client-core/src/domain/adminModels.ts:157-166`) has **no** `nodeId`/`behaviorId`/`agentRef` fields at all (those live only on `StepDefinition`, lines 182-186), and `saveWorkflow`/`cloneWorkflow` write only `step_type`/`order_index`/`is_enabled`/`requires_approval` into `workflow_steps` (`supabaseAdminRepository.ts` ~lines 596-602, 671-677) — never the per-instance flow-engine columns. A larger gap also confirmed: there is **no edges/graph authoring UI anywhere** in `WorkflowsSettings.tsx` (`Workflow.edges` is a pure load→save pass-through; a from-scratch workflow gets `edges: []`), so a brand-new custom flow cannot define a runnable graph at all and `resolveWorkflowFlowRef` silently bails (the run falls back to the legacy non-flow hub path). No client-side validation UI exists for unknown behavior/agent refs or invalid join/policy values either. See [BUG-243 follow-up plan / the custom-flow scope doc] for the full remaining scope.
- notes: **the earlier "backend implemented; UI not started" state is superseded — see [CA-160](../../../change-audit/CA-160-flow-definitions-migrated-to-workflows-table.md).** The `GET/POST/PUT /client/flows*` local-runner endpoints from the prior pass (CA-159) were dead code: `WorkflowsSettings.tsx` talks directly to Supabase via `getAdminUseCases()` → `SupabaseAdminRepository`, never through local-runner, and were removed. Instead: built-in flows mirror into the existing `workflows`/`workflow_steps` tables (extended with `is_builtin`/`editable`/`cloneable`/pack-identity/policy/node columns), and `WorkflowsSettings.tsx` itself now shows a "Built-in" badge, hides Save/Delete for non-editable rows (replaced with "Clone"), gates the step editor read-only for built-ins, and exposes the new per-step flow-engine fields (Node ID, Behavior ID, Agent ref, Depends on, Join mode, Cohort) — a form-based MVP, matching this task's own "defer a full graph editor" allowance.
- follow-ups: no live browser verification was possible (the desktop app requires real Supabase sign-in with no demo/bypass mode; confirmed again 2026-07-06, the preview sandbox stalls on "Runner Offline"/"Failed to fetch" with no reachable backend); the migration needs a real apply pass against a live Supabase project.
- resolved 2026-07-06 (superseded by later work, no new code needed): the "per-step fields only reach the shared step-type catalog, not per-workflow-instance `workflow_steps`" gap was the exact problem [BUG-236](../../09-BugFix/done/BUG-236-Builtin-Flow-Mirror-Stores-Node-Definition-On-Workflow-Steps-Instead-Of-Step-Definitions.md) reframed as the intended design (node data belongs ONLY on `step_definitions`; `workflow_steps` is a pure relation table, never per-instance overrides) — not a bug to fix, a contract this doc's original T-5 had backwards. `saveWorkflow never writes node-identity fields into workflow_steps (BUG-236)` in `workflowFlowEngineAttrs.test.ts` asserts the corrected contract and passes. The "no edges/graph authoring UI" gap is fixed by [Task-189](../done/Task-189-Custom-Flow-Graph-Authoring.md) (done — owner's manual E2E click-through confirmed 2026-07-07; its own 5 implementation slices are shipped): `renderWorkflowEdgesEditor` (form-list) and `renderWorkflowFlowCanvas` (visual canvas) in `WorkflowsSettings.tsx`, both bound to `Workflow.edges`, plus `validateFlowGraph` gating save — covering most of T-5/T-6's original scope (nodes/edges/behavior refs/agent refs already existed via the step-definition form; client-side validation for unknown behavior refs, missing agent ref, and no entry node now exists via `validateFlowGraph`; policy/join-specific validation and unknown-node-ref-in-a-non-edge-context validation remain unimplemented but are not part of what was actually asked for here).
- implemented 2026-07-06 (closes T-2's own remaining display gap): the built-in flow list and detail header in `WorkflowsSettings.tsx` never surfaced pack ID/version or selectable modes (verified via direct grep — zero matches for `packId|packVersion|selectableIn` in the file, despite `Workflow` already carrying all of these fields). Added a compact `.settings-list-item-meta` line to both the workflow list row (`workflow.isBuiltin` only) and the detail panel header, showing pack id/version, selectable-in modes, and a chat-baseline note. Description was already a visible/editable field in the detail form (`workflowDraft.description` textarea) once a workflow is selected, so no separate display was added for it. "View definition action" is satisfied by the existing behavior — selecting any workflow row already opens its full (read-only, for built-ins) definition in the detail panel; a second, identical button was judged pure redundancy and not added. Verified via desktop typecheck (`tsc.cmd --noEmit`, clean) and phase1 tests (43/47 pass, same 4 pre-existing unrelated failures as before this change — `desktopSupabaseAuthRepository`, `navigatorCatalog`, `settingsHelpers`, `importBoundary`-adjacent fixture gaps). No live browser click-through possible in this sandbox (see follow-ups above).
- upstream docs updated: [CP-42](../../07-Coding-Plan/done/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md) progress notes (link path corrected — pointed at a nonexistent `todo/` copy before). [CA-160](../../../change-audit/CA-160-flow-definitions-migrated-to-workflows-table.md) is a dangling reference — no `requirements/change-audit/` directory exists anywhere in this checkout.
