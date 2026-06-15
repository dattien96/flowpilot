# Task-041 Desktop Workflows Steps Registry Detail Parity

## Metadata

- Document ID: `Task-041`
- Title: `Desktop Workflows Steps Registry Detail Parity`
- Phase: `task`
- Status: `done`
- Owner: `Codex`
- Reviewers: `TBD`
- Created: `2026-06-15`
- Last Updated: `2026-06-15`
- Parent Documents: `requirements/07-Coding-Plan/done/CP-07-Workflow-Engine-UI.md`, `requirements/07-Coding-Plan/done/CP-15-03-Step-Definition-Execution-Contract.md`, `requirements/06-System-Tech-Design/SD-05-Workflow-Engine.md`, `requirements/05-System-Specs/SS-04-Workflow.md`, `requirements/05-System-Specs/SS-08-Approve-Gate.md`
- Child Documents: ``
- Related Documents: `requirements/10-Refactor/Migrate-Web-To-Desktop/R3-Phase1-Review-Capture-Issues.md`, `requirements/10-Refactor/Migrate-Web-To-Desktop/R3-Phase1-Checklist.md`, `change-audit/CA-067-desktop-workflows-steps-registry-detail-parity.md`, `requirements/08-Task/done/Task-040-Desktop-Projects-Registry-Detail-Parity.md`
- Replaces: ``
- Tags: `desktop-flowpilot`, `workflow`, `step-definition`, `desktop-migration`, `settings`

## AI Quick View

### Summary

- Reworked the desktop `Workflows` settings tab into a `Workflows/Steps` registry/detail surface closer to the web `/workflows` and `/workflow-steps` pages.
- Replaced auto-create behavior with separate in-tab create views for both workflows and step definitions.
- Added delete flows, workflow reasoning/YOLO controls, and the missing step-definition runtime contract fields.

### Current Ask

- Finish all unresolved review items under section `5. Workflows page: refer: /workflows + /workflow-steps in web` and mark them done once the desktop parity gaps are closed.

### Key Decisions

- `T-1` Keep the parity work inside the existing desktop settings shell instead of adding full nested desktop routes.
- `T-2` Extend the shared admin workflow repository with `deleteWorkflow()` and `deleteStepDefinition()` so the desktop delete actions use the real Supabase-backed path.
- `T-3` Use dirty-state snapshots so workflow and step detail views only enable Save after the user changes data.

### Constraints

- Stay inside section 5 of the Phase 1 migration review.
- Reuse the desktop settings layout patterns already introduced for the `Projects` page.
- GitNexus impact tooling was not available in this thread, so the required symbol impact step was replaced with careful local inspection.

### Open Questions

- Should workflow and step create/detail subviews become URL-addressable later, or remain local settings-shell state until a broader desktop router pass happens?

### Source Refs

- `CP-07 section 6`
- `CP-15-03 section 2`
- `CP-15-03 section 4.4`
- `CP-15-03 section 4.5`
- `SS-04 section 3.1`
- `SS-04 section 3.2`
- `SS-04 section 3.3`
- `SS-04 section 3.4`
- `SS-08 section 3`
- `R3 review item 5`

## 1. Goal

Bring the desktop `Workflows` settings surface into parity with the web workflow-definition and step-definition pages for the review scope in section 5, including create/detail flow, delete actions, workflow YOLO/reasoning controls, and the full step-definition execution contract fields.

## 2. Parent Links

- coding plan: `requirements/07-Coding-Plan/done/CP-07-Workflow-Engine-UI.md`, `requirements/07-Coding-Plan/done/CP-15-03-Step-Definition-Execution-Contract.md`
- tech design: `requirements/06-System-Tech-Design/SD-05-Workflow-Engine.md`
- system spec: `requirements/05-System-Specs/SS-04-Workflow.md`, `requirements/05-System-Specs/SS-08-Approve-Gate.md`
- specific upstream ids: `CP-07 section 6`, `CP-15-03 section 2`, `CP-15-03 section 4.4`, `CP-15-03 section 4.5`, `SS-04 section 3.1`, `SS-04 section 3.2`, `SS-04 section 3.3`, `SS-04 section 3.4`, `SS-08 section 3`, `R3 review item 5.1-5.5`

## 3. Trigger

The Phase 1 desktop migration review still listed the workflow/step settings area as incomplete. The existing desktop page auto-created empty workflows, mixed create and edit in one view, lacked workflow and step delete actions, missed workflow reasoning and YOLO controls, and only exposed a small subset of the step-definition fields required by the web product and step execution contract.

## 4. Exact Change

- `T-1` Renamed the desktop settings navigation label from `Workflows` to `Workflows/Steps`.
- `T-2` Rebuilt `WorkflowsSettings.tsx` into left registry / right detail surfaces for both workflows and step definitions.
- `T-3` Replaced auto-create behavior with separate in-tab create views for workflows and step definitions, entered from a circular `+` action and exited with a top-left back icon.
- `T-4` Added workflow delete and step-definition delete flows guarded by exact `delete` confirmation.
- `T-5` Added dirty-state tracking so workflow and step detail Save actions are only enabled after data changes.
- `T-6` Restored workflow detail controls for model override, reasoning effort override, YOLO mode, and step-level ordering/editing.
- `T-7` Restored step-definition detail fields for required MCPs, required skills, prompt base, subagent, reasoning effort, input artifact definitions, and output artifact definitions.
- `T-8` Extended the shared desktop admin workflow repository contract and Supabase implementation with `deleteWorkflow(workflowId)` and `deleteStepDefinition(stepType)`.
- `T-9` Marked all section-5 review items as done in `R3-Phase1-Review-Capture-Issues.md`.

## 5. Touched Areas

- files: `apps/desktop-flowpilot/src/components/SettingsShell.tsx`, `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx`, `apps/desktop-flowpilot/src/styles.css`, `packages/flowpilot-client-core/src/domain/adminRepositories.ts`, `packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts`, `requirements/10-Refactor/Migrate-Web-To-Desktop/R3-Phase1-Review-Capture-Issues.md`
- modules: `desktop-flowpilot`, `flowpilot-client-core`
- routes: `desktop Settings > Workflows/Steps`
- tables: `workflows`, `workflow_steps`, `step_definitions`, `step_input_artifact_definitions`, `step_output_artifact_definitions`

## 6. Acceptance Check

- The desktop settings nav shows `Workflows/Steps`.
- The workflow main page is left list / right detail and no longer auto-creates a workflow row when Create is pressed.
- The step-definition main page is left list / right detail and uses a separate create view.
- Workflow and step detail pages expose delete actions.
- Workflow detail includes reasoning effort and YOLO controls.
- Step detail includes required MCPs, required skills, prompt base, subagent, reasoning effort, input artifact definitions, and output artifact definitions.
- Workflow and step detail Save buttons stay disabled until data changes.
- `npm run typecheck` passes in `apps/desktop-flowpilot`.
- `npm run build` passes in `apps/desktop-flowpilot`.

## 7. Out of Scope

- Workflow run-history page parity beyond the existing desktop surface.
- Teams page parity from review section 6.
- Artifact-page parity from review section 7.
- Converting settings-shell subviews into full desktop route entries.

## 8. Completion Notes

- result: Implemented and verified locally; the section-5 workflow/step review items were marked done.
- follow-ups: Team-page, Google Drive, and artifact-page parity remain separate review slices.
- upstream docs updated: `R3-Phase1-Review-Capture-Issues.md`, `Task-041`, `CA-067`
