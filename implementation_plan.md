# Implementation Plan: MCP Management and Project Routing Alignment

## Scope

Align admin-web routes and page responsibilities with the updated architecture:

- global MCP type and instance management
- dedicated MCP Test Console route
- dedicated RUNNER settings page
- dedicated project creation route
- project settings limited to link and unlink behavior
- remote MCP type enablement persisted in the current integration storage rollout

## Current State

Observed in current code:

- `/settings/mcp-servers` already owns much of the global MCP flow
- `/projects/$projectId/settings` already behaves largely as link and unlink only
- the current link and unlink UI still does not match the desired hide/show behavior
- `/projects` still embeds project creation
- `/settings/mcp-servers` still embeds MCP Test Console and runner-wide concerns

## Work Items

### 1. Split MCP routes

Create a dedicated route:

- `/settings/mcp-servers/mcp-connect-test`

Move from the current `mcp-servers.tsx` page:

- test-console state
- test-console mutations
- test-console templates and prompt handling
- recent-run and result panels

Keep on `/settings/mcp-servers` only:

- combined Runner Reachability + Allowlisted MCP inventory
- MCP instances list

### 2. Add RUNNER settings page

Create a new route:

- `/settings/runner`

Move system-wide runner reachability content there when it is not required to explain a specific MCP backend row.

Keep on the MCP page only the runner summary that is necessary to understand whether MCP actions are blocked.

### 3. Normalize MCP inventory behavior by category

Update the MCP inventory section to explicitly model:

- `remote` types
- `local` types

Required behavior:

- each type has one enable or disable surface
- remote types can enable first, then create multiple instances
- local types keep install-oriented UX such as `npx` command hints and `INSTALL`

Persistence follow-up:

- because the current rollout still uses integration persistence, add a new integration-table field for MCP type enablement state for remote types
- this field gates whether instance creation is available for types like Jira
- the schema/migration must make the enablement state explicit before a user can create multiple Jira MCP instances

Copy and labels should clearly distinguish:

- type inventory
- instance list
- project links

### 4. Simplify MCP instances section

Review the instance list on `/settings/mcp-servers` and remove wording that implies an instance is owned by a project.

Expected display:

- label
- type
- summary
- status
- verification or sync timestamp
- error state
- create, edit, verify, remove actions

### 5. Split project creation route

Refactor `/projects` so it becomes list-first and exposes one:

- `ADD project`

Create:

- `/projects/create`

Move existing create-project UI and team assignment flow into the new route.

Update navigation so successful creation returns to the expected project page or project list.

### 6. Keep project settings link-only and hide the invalid action

Review `/projects/$projectId/settings` for stale copy and route targets only.

Expected behavior:

- list currently linked MCP by type
- select an existing global MCP instance
- when no MCP is linked for that type, show only `Link MCP`
- when an MCP is already linked for that type, hide `Link MCP` and show only `Unlink`
- `Create new MCP` navigates to `/settings/mcp-servers`

Do not reintroduce:

- project-owned create
- project-owned edit
- project-owned auth

### 7. Update tests

Adjust route and UI tests for:

- MCP Test Console navigation and dedicated route
- RUNNER settings route
- `/projects` to `/projects/create` flow
- project settings navigation to global MCP page
- project settings hide/show state for `Link MCP` vs `Unlink`

## Suggested Order

1. Add new routes and move navigation targets.
2. Extract MCP Test Console from the general MCP page.
3. Introduce RUNNER page and move runner-only content.
4. Clean up the remaining MCP page structure into the two required sections.
5. Split `/projects` and `/projects/create`.
6. Refresh tests and copy.
7. Add the integration-table migration for remote MCP type enablement.

## Risks

- `mcp-servers.tsx` currently mixes inventory, instance CRUD, and test-console state, so route extraction may require shared helpers or component splitting.
- Existing data models still use `Integration` naming and some project-oriented fields such as `projectId`, which may leak outdated ownership language into the UI.
- Adding machine-level remote-type enablement into the current integration persistence is a staged compromise and must be documented carefully so it does not get mistaken for project ownership.
- Navigation tests will fail until route constants and expected destinations are updated together.

## Done Criteria

- `/settings/mcp-servers` has only the two required sections
- `/settings/mcp-servers/mcp-connect-test` owns MCP testing
- `/settings/runner` exists for system-wide runner settings
- `/projects` only lists projects and exposes `ADD project`
- `/projects/create` owns project creation
- project settings remain link and unlink only, with only the valid action visible per type
- remote MCP type enablement is persisted with the required integration-table schema update
