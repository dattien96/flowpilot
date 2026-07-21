# CA-378: Flow/Workflow mode locked to YOLO=true

## Summary

Product decision (BUG-299): Flow/Workflow-mode launches (including the built-in Review Loop) always run YOLO=true; YOLO=off is only supported in Normal Chat, which has no hub/cohort/gate layer to race against. Motivated by a live test (case A3) where a cohort sibling's approval-required tool call never surfaced a visible approval card while the user was focused on another sibling's transcript, eventually failing with a transient provider error, followed by the hub's own stall-timeout firing on the retry round.

Traced why a built-in workflow could even reach `yoloMode=false`: `SyncBuiltins`'s mirror-sync insert never sets `yolo_mode` (a deliberate, pure per-installation admin setting kept out of the pack schema so a resync never clobbers an admin's choice), so a first-time insert fell through to the column's own Postgres default — `false` (`20260610160000_add_workflow_yolo_mode.sql`). Separately, the desktop Settings workflow-edit form's YOLO checkbox was the one field that did not respect the same `workflowDetailReadOnly` guard every sibling field already had.

Two-layer fix: (1) desktop Settings UI — all three yoloMode checkboxes (step-definition, create-workflow, edit-workflow) now render checked+disabled unconditionally, and every draft-construction path forces `yoloMode: true` regardless of what is loaded/defaulted; (2) new migration flips the `workflows.yolo_mode` / `step_definitions.yolo_mode` column defaults to `true`, so future first-time inserts start correct without requiring a Settings visit first. Confirmed the ChatInput composer's own toggle needs no change — already hidden for Workflow-mode via its existing `isChatMode` gate.

## Verification

- New additive test file `WorkflowsSettings.yolo-locked.test.ts`, 5 tests, all pass — confirms every draft-construction path (new workflow, new step, loading an existing workflow regardless of persisted value) yields `yoloMode=true`.
- Verified via a temporary, uncommitted alias-resolution harness (same technique as BUG-297) since this environment's `npm run test:phase1` is blocked by unrelated pre-existing stale fixtures.
- `tsc --noEmit`: no new errors from either changed file.
- Migration syntax-reviewed against existing conventions; not executed in this sandbox (no live Supabase available) — must be applied by the user.
- additive-tests-only honored: no existing test modified.

## Files

- `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx`: all three yoloMode checkboxes locked checked+disabled; `createEmptyStepDraft`, `mapWorkflowToDraft`, `createEmptyWorkflowDraft`, and the existing-step-select path all force `yoloMode: true`.
- `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.yolo-locked.test.ts`: additive tests.
- `supabase/migrations/20260720150000_default_flow_yolo_mode_true.sql`: new migration, column defaults `workflows.yolo_mode`/`step_definitions.yolo_mode` → `true`. Does not touch existing rows.

# ---8<--- flowpilot:change-ledger
feature_key: chat-ui
source_doc_id: BUG-299
change_type: feature
summary: Flow/Workflow-mode launches are now locked to YOLO=true across the desktop Settings UI and the underlying database column defaults; YOLO=off remains available only in Normal Chat.
# --->8---
