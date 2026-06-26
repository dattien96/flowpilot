# CA-067 Desktop Workflows Steps Registry Detail Parity

## Scope

Record the desktop `/workflows` and `/workflow-steps` parity pass that reworked the `Workflows` settings tab into a combined `Workflows/Steps` registry/detail surface and closed all review items in section 5 of the R3 capture doc.

## Completed

- Renamed the desktop settings navigation entry from `Workflows` to `Workflows/Steps`.
- Rebuilt `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx` so both workflows and step definitions now use a left registry list and right detail editor.
- Replaced the old auto-create workflow behavior with separate in-tab create views for workflows and step definitions.
- Added delete actions for workflows and step definitions, both guarded by exact `delete` confirmation.
- Added dirty-state tracking so the detail Save buttons stay disabled until the current workflow or step definition is actually modified.
- Restored workflow detail fields for model override, reasoning effort override, YOLO mode, and step ordering/editing controls.
- Restored step-definition runtime contract fields for required MCPs, required skills, prompt base, subagent, reasoning effort, agent type, and input/output artifact bindings.
- Extended `packages/flowpilot-client-core` workflow admin contracts with `deleteWorkflow()` and `deleteStepDefinition()`, and implemented both paths in the Supabase admin repository.
- Updated `requirements/10-Refactor/Migrate-Web-To-Desktop/R3-Phase1-Review-Capture-Issues.md` to mark all section-5 workflow items done.

## Verification

- Ran `npm run typecheck` in `apps/desktop-flowpilot`
- Ran `npm run build` in `apps/desktop-flowpilot`
- Build completed successfully; Vite still reports the existing renderer chunk-size warning above 500 kB

## Residual Notes

- GitNexus symbol impact tooling was not available in this thread, so required impact analysis and post-change detect-changes checks could not be executed.
- This slice closes only the workflow/step settings review section; teams, Google Drive, and artifact parity are still separate follow-up work.
- The desktop settings shell still uses internal tab/view state for create/detail transitions rather than full nested desktop routes.

# ---8<--- flowpilot:change-ledger
feature_key: workflow-runtime
source_doc_id: CA-067
change_type: feature
summary: Desktop Workflows Steps Registry Detail Parity
# --->8---
