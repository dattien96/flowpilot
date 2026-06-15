# Task-040 Desktop Projects Registry Detail Parity

## Metadata

- Document ID: `Task-040`
- Title: `Desktop Projects Registry Detail Parity`
- Phase: `task`
- Status: `done`
- Owner: `Codex`
- Reviewers: `TBD`
- Created: `2026-06-15`
- Last Updated: `2026-06-15`
- Parent Documents: `requirements/07-Coding-Plan/done/CP-15-02-Project-Launch-And-Workspace-UX.md`, `requirements/06-System-Tech-Design/SD-04-Project-Management.md`, `requirements/05-System-Specs/SS-01-Project.md`
- Child Documents: ``
- Related Documents: `requirements/10-Refactor/Migrate-Web-To-Desktop/R3-Phase1-Review-Capture-Issues.md`, `requirements/10-Refactor/Migrate-Web-To-Desktop/R3-Phase1-Checklist.md`, `change-audit/CA-066-desktop-projects-registry-detail-parity.md`
- Replaces: ``
- Tags: `desktop-flowpilot`, `projects`, `desktop-migration`, `settings`

## AI Quick View

### Summary

- Reworked desktop `Projects` settings into a left registry / right detail surface closer to web `/projects`.
- Moved project creation into a separate in-tab create view triggered by a circular `+` button.
- Added collapsible detail panels for overview, directory bindings, teams, MCP, run history, and artifacts, plus a guarded delete-project modal.

### Current Ask

- Finish all unresolved review items under section `4. Page Project: /projects in web` and mark them done once the desktop parity gaps are closed.

### Key Decisions

- `T-1` Keep the project parity work scoped to the desktop settings `Projects` tab rather than introducing a brand-new desktop route system.
- `T-2` Add quick navigation buttons from project detail panels into the existing Teams, Workflows, Artifacts, and MCP settings tabs.
- `T-3` Extend the shared desktop admin project repository with `deleteProject()` so the delete flow uses the real Supabase-backed path.

### Constraints

- Stay inside the project review scope from `R3-Phase1-Review-Capture-Issues.md` section 4.
- Reuse existing desktop settings patterns and current admin-use-case wiring where possible.
- GitNexus impact tooling was not available in this thread, so the required symbol impact step was replaced with careful local inspection.

### Open Questions

- Should the desktop settings shell grow real nested routing later so create/detail subviews and cross-tab deep-linking become URL-addressable?

### Source Refs

- `CP-15-02`
- `SD-04 section 3.4`
- `R3 review item 4`

## 1. Goal

Bring the desktop `Projects` settings tab closer to the web `/projects` behavior by separating create from registry, making project detail panel-based, exposing artifacts and quick navigation, and restoring guarded delete.

## 2. Parent Links

- coding plan: `requirements/07-Coding-Plan/done/CP-15-02-Project-Launch-And-Workspace-UX.md`
- tech design: `requirements/06-System-Tech-Design/SD-04-Project-Management.md`
- system spec: `requirements/05-System-Specs/SS-01-Project.md`
- specific upstream ids: `CP-15-02 section 5.1`, `CP-15-02 section 5.2`, `CP-15-02 section 5.3`, `SD-04 section 3.4`, `R3 review item 4.1-4.7`

## 3. Trigger

The Phase 1 desktop migration review still listed seven unresolved `/projects` parity gaps. The existing desktop `ProjectsSettings` mixed create and registry on one page, lacked artifact visibility, lacked cross-tab navigation helpers, and did not provide the required guarded delete flow.

## 4. Exact Change

- `T-1` Rebuilt the `Projects` settings surface into a left registry list and right project-detail column.
- `T-2` Removed the embedded create form from the main registry view and moved create into a separate in-tab view behind a circular `+` action.
- `T-3` Added collapsible project detail panels with the first panel expanded by default.
- `T-4` Renamed the project integration panel to `MCP` and added quick-navigation actions for Teams, Workflows/Run History, MCP tabs, and Artifacts.
- `T-5` Added artifact-run and local-artifact summaries for the selected project.
- `T-6` Added a modal delete flow that requires the user to type exactly `delete` before the destructive action is enabled.
- `T-7` Extended the desktop admin project repository contract and Supabase implementation with `deleteProject(projectId)`.
- `T-8` Marked all section-4 review items as done in `R3-Phase1-Review-Capture-Issues.md`.

## 5. Touched Areas

- files: `apps/desktop-flowpilot/src/components/settings/ProjectsSettings.tsx`, `apps/desktop-flowpilot/src/components/SettingsShell.tsx`, `apps/desktop-flowpilot/src/styles.css`, `packages/flowpilot-client-core/src/domain/adminRepositories.ts`, `packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts`, `requirements/10-Refactor/Migrate-Web-To-Desktop/R3-Phase1-Review-Capture-Issues.md`
- modules: `desktop-flowpilot`, `flowpilot-client-core`
- routes: `desktop Settings > Projects`
- tables: `projects`, `project_workspace_bindings`, `project_teams`

## 6. Acceptance Check

- Desktop `Projects` no longer shows the create form on the main registry view.
- The registry view shows projects on the left and selected-project detail on the right.
- Project detail sections are collapsible and default to the first section expanded.
- The project MCP section is labeled `MCP`.
- Project detail includes artifact summaries for the selected project.
- Teams, Run History, MCP, and Artifacts each expose a quick-navigation button to the related desktop settings area.
- Deleting a project requires exact `delete` confirmation in a modal before the action is enabled.
- `npm run typecheck` passes in `apps/desktop-flowpilot`.
- `npm run build` passes in `apps/desktop-flowpilot`.

## 7. Out of Scope

- Full workflow-page parity from review section 5.
- Team-page parity from review section 6.
- Artifact-page parity beyond the project-level artifact summary needed for section 4.
- Replacing the desktop settings shell with a full nested-router implementation.

## 8. Completion Notes

- result: Implemented and verified locally; the section-4 `/projects` review items were marked done.
- follow-ups: The Workflows, Teams, Google Drive, and Artifacts review sections remain open and should be handled as separate task slices.
- upstream docs updated: `R3-Phase1-Review-Capture-Issues.md`, `Task-040`, `CA-066`
