# Task-042 Desktop Teams Registry Detail Parity

## Metadata

- Document ID: `Task-042`
- Title: `Desktop Teams Registry Detail Parity`
- Phase: `task`
- Status: `done`
- Owner: `Codex`
- Reviewers: `TBD`
- Created: `2026-06-15`
- Last Updated: `2026-06-15`
- Parent Documents: `requirements/05-System-Specs/SS-03-Project-Team.md`
- Child Documents: ``
- Related Documents: `requirements/10-Refactor/Migrate-Web-To-Desktop/R3-Phase1-Review-Capture-Issues.md`, `requirements/10-Refactor/Migrate-Web-To-Desktop/R3-Phase1-Checklist.md`, `change-audit/CA-073-desktop-teams-registry-detail-parity.md`, `requirements/08-Task/done/Task-040-Desktop-Projects-Registry-Detail-Parity.md`, `requirements/08-Task/done/Task-041-Desktop-Workflows-Steps-Registry-Detail-Parity.md`
- Replaces: ``
- Tags: `desktop-flowpilot`, `teams`, `team-members`, `desktop-migration`, `settings`

## AI Quick View

### Summary

- Rebuilt the desktop Teams settings page from a flat inline form into a proper left-registry / right-detail layout consistent with the Projects and Workflows/Steps pages.
- Added team delete flow guarded by an exact `delete` confirmation, matching the pattern already used for Projects and Workflows.
- Added a separate create view for new teams (back button + save, no inline form).
- Added a member detail modal: clicking any member opens a modal with all fields editable (name, email, jira, role, level, skills, capacity) plus save and remove actions.

### Current Ask

- Close all open items under section `6. Page Teams` in `R3-Phase1-Review-Capture-Issues.md`.

### Key Decisions

- `T-1` Keep the team create flow as a separate in-tab view (same pattern as Projects and Workflows) rather than an inline form.
- `T-2` Implement member detail as a modal overlay; this matches the picker-modal pattern already established for artifact definitions and MCPs.
- `T-3` Add `updateTeam` and `updateMember` alongside `deleteTeam` so the detail view has full CRUD coverage, not just create/delete.
- `T-4` Track dirty state on team name so the Save button is only enabled after a change.

### Constraints

- Stay inside section 6 of the Phase 1 migration review.
- Reuse desktop settings layout patterns (`.project-layout`, `.project-registry-panel`, `.project-detail-column`, delete modal) already established by Tasks 040 and 041.
- GitNexus impact tooling was not available in this thread; careful local inspection was used instead.

### Open Questions

- Should team-project link management surface in the Teams detail view, or remain only on the Projects detail page?

### Source Refs

- `SS-03 section 6–7`
- `R3 review item 6`
- `Task-040`, `Task-041` (layout precedent)

## 1. Goal

Bring the desktop Teams settings surface into parity with the web `/teams` page for the review scope in section 6: registry/detail split layout, separate create view, team delete, and a full member detail modal with edit capability.

## 2. Parent Links

- coding plan: _(no dedicated CP; governed by the Phase 1 migration review)_
- tech design: _(no dedicated SD for desktop migration)_
- system spec: `requirements/05-System-Specs/SS-03-Project-Team.md`
- specific upstream ids: `SS-03 section 6`, `SS-03 section 7`, `R3 review item 6`

## 3. Trigger

Section 6 of the Phase 1 desktop migration review listed three gaps in the Teams page: missing delete team, missing separate create flow, and missing member detail page. This task closes all three.

## 4. Exact Change

- `T-1` Added `updateTeam(teamId, name)` and `deleteTeam(teamId)` to `TeamRepository` interface and implemented both in `supabaseAdminRepository`.
- `T-2` Added `updateMember(memberId, patch)` to `TeamRepository` interface and implemented in `supabaseAdminRepository`.
- `T-3` Rewrote `TeamsSettings.tsx` into left-registry / right-detail layout matching the Projects and Workflows/Steps pattern.
- `T-4` Replaced inline team-create form with a separate create view (back button + Save Team button, returns to list after save).
- `T-5` Added team delete flow: Delete button on team detail opens a confirmation modal requiring the user to type `delete`.
- `T-6` Added member list on the team detail panel: each member is a clickable row that opens a member detail modal.
- `T-7` Member detail modal exposes all member fields (name, email, jira account ID, role, level, weekly capacity, skills) with dirty-state tracking for the Save button plus a Remove button.
- `T-8` Marked section-6 review items as done in `R3-Phase1-Review-Capture-Issues.md`.

## 5. Touched Areas

- files: `apps/desktop-flowpilot/src/components/settings/TeamsSettings.tsx`, `packages/flowpilot-client-core/src/domain/adminRepositories.ts`, `packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts`, `requirements/10-Refactor/Migrate-Web-To-Desktop/R3-Phase1-Review-Capture-Issues.md`
- modules: `desktop-flowpilot`, `flowpilot-client-core`
- routes: `desktop Settings > Teams`
- tables: `teams`, `team_members`

## 6. Acceptance Check

- The Teams page uses a left list / right detail layout; no inline create form is visible on the main page.
- The + button opens a dedicated create view with a back arrow and Save Team button.
- The team detail panel shows the team name in an editable field; Save Team is only enabled after the name changes.
- A Delete button on the team detail opens a modal requiring the user to type `delete`; confirming removes the team and returns to the first remaining team.
- The member list on the detail panel shows each member as a clickable row.
- Clicking a member opens a modal with all fields editable; Save Member is only enabled after a change; Remove removes the member immediately.
- `npx tsc --noEmit` passes in both `apps/desktop-flowpilot` and `packages/flowpilot-client-core`.

## 7. Out of Scope

- Team-project link management from within the Teams page (project links are managed from the Projects page).
- Skill tag chip-based editing (comma-separated input is used; chip UI can be added as a follow-up).
- Artifact and Google Drive page parity from review sections 7 and beyond.
- Converting settings-shell subviews into full desktop route entries.

## 8. Completion Notes

- result: Implemented and verified via TypeScript check; section-6 review items marked done.
- follow-ups: Member skill tags could adopt the chip-picker pattern used for MCPs/artifacts. Google Drive and Artifact page parity remain as separate review slices.
- upstream docs updated: `R3-Phase1-Review-Capture-Issues.md`, `Task-042`, `CA-073`
