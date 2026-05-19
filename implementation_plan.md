# Implementation Plan - CP-05 Project & Team Management

Date: 2026-05-18

## 1. Objective

Extend the existing admin project area so it can manage teams, team members, and project settings in line with `requirements/07-Coding-Plan/CP-05-Project-Management.md`.

## 2. Execution Boundaries

- This phase keeps the current admin app architecture intact.
- Add project management capabilities on top of the existing project domain instead of replacing the broader app shell.
- Introduce team and member support in the domain and persistence layers.
- Rework the project detail screen into a management layout with tabbed subpages.
- Surface project settings for artifact storage preference and MCP context status only; do not implement full MCP installation.

## 3. Current State To Correct

- `ProjectGateway` only supports list, detail, and create.
- `Project` does not yet include `directoryPath`, `ownerId`, `status`, or artifact storage preference.
- There are no domain entities or gateways for teams or team members.
- The Supabase gateway bundle does not map the new tables required by CP-05.
- The project detail route still prioritizes feature and workflow run cards instead of project management tabs.

## 4. Phase Breakdown

### Phase A - Domain Expansion

1. Update the `Project` entity with the CP-05 fields.
2. Add `Team`, `TeamMember`, and `Integration` entities.
3. Define `MemberRole` and `LevelLabel` as narrow unions.

### Phase B - Gateway Expansion

1. Expand `ProjectGateway` with update/delete operations and project-team listing if needed by the UI.
2. Add a new `TeamGateway` contract covering team CRUD, member CRUD, and project linking.
3. Keep the gateway shapes small and composable so the Supabase and demo implementations can be aligned cleanly.

### Phase C - Supabase Mapping

1. Add table mappings for `teams`, `project_teams`, and `team_members`.
2. Extend project mappings for the new project columns.
3. Add repository methods for team CRUD, member CRUD, and project-team linking.

### Phase D - Project Management UI

1. Rework `/projects/[projectId]` into a tabbed management layout.
2. Add member management at `/projects/[projectId]/members`.
3. Add project settings at `/projects/[projectId]/settings`.
4. Keep other tabs as route placeholders if the detailed content is not yet part of CP-05.

### Phase E - Query And Form Support

1. Introduce project and member query keys for the new routes.
2. Add local form state and validation only where the project management screens need it.
3. Preserve existing project list behavior while adding the new management entry points.

### Phase F - Schema Alignment

1. Prepare the migration shape for the new Supabase tables and project column changes.
2. Ensure naming and enum values match the CP-05 specification.
3. Keep the CP-10 integration storage note as a dependency boundary, not an implementation dependency.

## 5. Verification Sequence

1. Confirm the design covers all CP-05 schema changes.
2. Confirm the gateway interfaces support team CRUD and project linkage.
3. Confirm the route tree covers project management tabs, members, and settings.
4. Confirm the new domain shapes remain compatible with the current project flow.

## 6. Done Criteria

- the CP-05 design artifacts are aligned with the project management requirement
- the implementation scope is clear and bounded
- the next coding phase can proceed without guessing at schema or UI structure

