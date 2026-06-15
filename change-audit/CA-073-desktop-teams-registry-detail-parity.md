# CA-073 Desktop Teams Registry Detail Parity

## Scope

Desktop settings Teams page rebuilt to match the web `/teams` page for Phase 1 migration review section 6. Three specific gaps were closed: team delete, separate create view, and member detail with editing.

## Completed

### Repository interface (`packages/flowpilot-client-core/src/domain/adminRepositories.ts`)

Added three new methods to `TeamRepository`:
- `updateTeam(teamId: string, name: string): Promise<Team>` — rename a team
- `deleteTeam(teamId: string): Promise<void>` — remove a team
- `updateMember(memberId: string, patch: Partial<...>): Promise<TeamMember>` — edit any member field

### Supabase implementation (`packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts`)

Implemented the three new methods:
- `updateTeam`: `UPDATE teams SET name, updated_at WHERE id`
- `deleteTeam`: `DELETE FROM teams WHERE id`
- `updateMember`: partial `UPDATE team_members` for any of name / email / jira_account_id / role / level_label / skill_tags / weekly_capacity_hours

### UI (`apps/desktop-flowpilot/src/components/settings/TeamsSettings.tsx`)

Full rewrite of the component:

**Layout**: Left panel (team registry list + circular + button) / Right panel (team detail + members panel), consistent with the Projects and Workflows/Steps pages.

**Create view**: Switching to `viewMode === "create"` shows a single team-name input with a back arrow and Save Team button. Saving returns to list view with the new team selected.

**Team detail**: Shows team name in an editable field. Save Team button uses dirty-state tracking (only enabled after name changes). Delete button opens the standard confirmation modal requiring the user to type `delete`.

**Members panel**: Lists all members for the selected team. Each member row is a clickable button. Clicking opens the member detail modal. A circular + button opens the add-member modal.

**Member modal** (detail + add): Full-featured modal with fields for name, email, jira account ID, role (7 options), level (5 options), weekly capacity hours, and skills (comma-separated). In edit mode the Save Member button uses dirty-state tracking. A Remove button is shown in edit mode to immediately remove the member.

### Documentation

- `R3-Phase1-Review-Capture-Issues.md` — section 6 items marked DONE
- `requirements/08-Task/done/Task-042-Desktop-Teams-Registry-Detail-Parity.md` — new task document created

## Verification

- `npx tsc --noEmit` passes in `apps/desktop-flowpilot` — no type errors
- `npx tsc --noEmit` passes in `packages/flowpilot-client-core` — no type errors
- GitNexus impact analysis was not available in this thread; local inspection confirmed the three new repository methods are new additions with no existing callers that could break

## Residual Notes

- Member skill tags use a comma-separated text input. A chip-based picker (matching the MCP/artifact pattern from the same session) could be added as a follow-up.
- `updateTeam` uses an optimistic local state update for the team list name to avoid a full refresh, which is consistent with the existing project-detail save pattern.
- Team-project links are not surfaced from the Teams detail page; they remain managed from the Projects settings page.
- Google Drive tab parity (review item 6 Google Drive + review items 7.x Artifacts) remain as separate tasks.
