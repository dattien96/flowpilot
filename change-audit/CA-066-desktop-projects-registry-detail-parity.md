# CA-066 Desktop Projects Registry Detail Parity

## Scope

Record the desktop `/projects` parity pass that reworked the `Projects` settings tab to match the web registry/detail model more closely and close all review items in section 4 of the R3 capture doc.

## Completed

- Reworked `apps/desktop-flowpilot/src/components/settings/ProjectsSettings.tsx` into a left project registry and right detail surface.
- Moved project creation out of the main registry view into a separate in-tab create page triggered by a circular `+` button.
- Added collapsible detail panels for overview, directory bindings, teams, MCP, run history, and artifacts, with the first panel expanded by default.
- Renamed the project integration panel to `MCP` and added quick navigation actions into related Teams, Workflows, Artifacts, and MCP settings tabs.
- Added project-level artifact summaries using both local runner artifacts and persisted artifact runs.
- Added a guarded delete-project modal that enables the destructive action only after exact `delete` confirmation.
- Extended `packages/flowpilot-client-core` project admin contract with `deleteProject()` and implemented it in the Supabase admin repository.
- Updated `requirements/10-Refactor/Migrate-Web-To-Desktop/R3-Phase1-Review-Capture-Issues.md` to mark all section-4 project items done.

## Verification

- Ran `npm run typecheck` in `apps/desktop-flowpilot`
- Ran `npm run build` in `apps/desktop-flowpilot`
- Build completed successfully; Vite still reports the existing renderer chunk-size warning above 500 kB

## Residual Notes

- GitNexus symbol impact tooling was not available in this thread, so required impact analysis and post-change detect-changes checks could not be executed.
- This slice closes only the project-page review section; workflows, teams, Google Drive, and broader artifact parity are still separate follow-up work.
- The desktop settings shell still uses in-tab view state rather than full nested routes for project create/detail subviews.

# ---8<--- flowpilot:change-ledger
feature_key: project-nav
source_doc_id: CA-066
change_type: feature
summary: Desktop Projects Registry Detail Parity
# --->8---
