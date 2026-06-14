# R3 Phase 1 Checklist

Source of truth:

- `requirements/10-Refactor/Note/R3-Migrate-Web-To-Desktop.md`

Audit date:

- `2026-06-14`

## Phase 1 Audit DOD

Use this as the signoff list for Phase 1. Mark an item done only when the repo
state proves it.

### 1. Shared Core And Boundaries

- [x] `packages/flowpilot-client-core` exists
- [x] Desktop imports shared core as `@flowpilot/client-core`
- [x] Shared repositories exist for runner/runtime/Supabase admin data
- [x] Shared core has no React/Electron imports in current domain/data files
- [x] Directory validation is behind shared `DirectoryRepository`
- [x] Provider/model mapping logic is in shared core instead of desktop-only UI
- [ ] Admin Web consumes shared core during gradual migration
      web-side; ignored in current desktop-only pass
- [x] Shared core unit tests exist
- [x] Import-boundary checks exist

### 2. Desktop Shell / Auth / Bootstrap

- [x] Bootstrap loading state exists
- [x] Unauthenticated login/settings split exists
- [x] Supabase settings reachable before auth
- [x] Login blocks when Supabase is not configured
- [x] Login screen includes `Database Settings`
- [x] Logout uses shared auth use case
- [x] Invalid Supabase screen state is explicitly modeled
- [x] Desktop UI/state transition tests cover auth/bootstrap transitions

### 3. Settings Navigation And Shell

- [x] Desktop has a dedicated Settings shell
- [x] Settings shell uses left navigation and detail area
- [x] `Open Admin Web` is replaced by `Settings`
- [x] Dedicated settings entries exist for Projects, Workflows, Teams, Artifacts,
      AI Providers, Google Drive, Jira MCP, Runner, and Supabase
- [ ] Desktop shell parity is proven against all selected Admin Web routes

### 4. Project Migration

- [x] Project list/create screen exists
- [x] Project detail settings exist
- [x] Linked teams section exists
- [x] Run history section exists
- [x] Directory bindings editor exists
- [x] Provider session idle TTL field exists
- [x] Create requires at least one directory binding
- [x] Duplicate binding paths are rejected in desktop validation
- [x] Fallback validation preserves configured order in shared helper logic
- [x] Fallback path selection tests exist
- [x] Idle TTL save/load tests exist
- [ ] Chat launch is proven to use resolved binding path end-to-end

### 5. Workflow / Step Migration

- [x] Workflow list/create/edit screen exists
- [x] Workflow step list/create/edit screen exists
- [ ] Shared use case coverage exists for workflow behavior
- [ ] Existing tests were ported or reused

### 6. Team Migration

- [x] Team list/create/edit flow exists
- [x] Project detail can load linked teams
- [x] Team changes are reflected after reload

### 7. Artifact Migration

- [x] Generated / Storage / Catalog tabs exist
- [x] Generated tab can display local artifacts and artifact runs
- [x] Catalog can create/edit artifact definitions
- [x] Storage driver settings screen exists
- [x] Storage behavior is per-project parity with desktop Phase 1 scope
- [x] Storage validation before enabling sync is implemented
- [ ] Shared-core auto-sync / provider-switch parity is proven

### 8. AI Provider Migration

- [x] AI providers screen exists
- [x] Supported model add/edit enablement exists
- [x] Desktop no longer owns provider mapping logic by itself
- [x] Model mapping behavior is covered by tests
- [x] New supported model flow is proven in workflow/step config

### 9. MCP / Google Drive / Jira Migration

- [x] Desktop has dedicated Google Drive settings navigation
- [x] Desktop has dedicated Jira MCP settings navigation
- [x] Generic MCP integration create/test flow exists
- [x] Dedicated Google Drive setup parity exists for desktop Phase 1 scope
- [x] Dedicated Jira MCP setup parity exists for desktop Phase 1 scope
- [ ] Chat/workflow setup can later select MCP config from migrated flow

### 10. Runner Status Migration

- [x] Full runner diagnostics screen exists
- [x] Header runner status trigger exists
- [x] Header opens runner health modal
- [x] Modal shows base URL, version, workspace, platform, started time, and error
- [x] Offline runner blocking/recovery behavior is enforced in desktop shell/navigation

### 11. Verification And Cutover Readiness

- [x] Phase 1 audit note exists in `Note-Understanding.md`
- [x] Phase 1 checklist file exists
- [x] Detailed per-route source mapping artifact exists
- [x] Shared core unit tests exist
- [x] Desktop UI/state transition tests for screen transitions exist
- [x] Runner API contract tests exist
- [ ] Admin Web migrated routes still pass through shared core
      web-side; ignored in current desktop-only pass
- [ ] Manual smoke checklist is written and executed

## Milestone Breakdown

### P1.0 Inventory And Contracts

- [x] Route inventory reviewed against the migration matrix
- [x] Current repo audit recorded in `Note-Understanding.md`
- [ ] Per-route source file / repository / endpoint / table mapping written down
- [ ] Per-route acceptance checklist completed in detail

### P1.1 Shared Client Core Scaffold

- [x] `packages/flowpilot-client-core` exists
- [x] Desktop imports shared core as `@flowpilot/client-core`
- [x] Shared repositories exist for runner/runtime/Supabase admin data
- [x] Shared core has no React/Electron imports in current domain/data files
- [ ] Admin Web consumes shared core during gradual migration
      web-side; ignored in current desktop-only pass
- [x] Shared core unit tests exist

### P1.2 Desktop Shell / Bootstrap / Login

- [x] Bootstrap loading state exists
- [x] Unauthenticated login/settings split exists
- [x] Supabase settings reachable before auth
- [x] Login blocks when Supabase is not configured
- [x] Logout uses shared auth use case
- [x] Invalid Supabase screen state is explicitly modeled
- [x] Desktop UI/state transition tests cover auth/bootstrap transitions

### P1.3 Project Migration

- [x] Project list/create screen exists
- [x] Project detail settings exist
- [x] Directory validation goes through shared `DirectoryRepository`
- [x] Create requires at least one directory binding
- [x] Duplicate binding paths are rejected in desktop validation
- [x] Idle TTL field exists
- [x] Fallback path selection tests exist
- [x] Idle TTL save/load tests exist
- [ ] Chat launch is proven to use resolved binding path end-to-end

### P1.4 Workflow And Step Migration

- [x] Workflow list/create/edit screen exists
- [x] Workflow step list/create/edit screen exists
- [ ] Shared use case coverage exists for workflow behavior
- [ ] Existing tests were ported or reused

### P1.5 Team Migration

- [x] Team list/create/edit flow exists
- [x] Project detail can load linked teams
- [x] Team changes are reflected after reload

### P1.6 Artifact Migration

- [x] Generated / Storage / Catalog tabs exist
- [x] Generated tab can display local artifacts and artifact runs
- [x] Storage behavior is per-project parity with desktop Phase 1 scope
- [x] Storage validation before enabling sync is implemented
- [ ] Shared-core auto-sync / provider-switch parity is proven

### P1.7 AI Provider Migration

- [x] AI providers screen exists
- [x] Supported model add/edit enablement exists
- [x] Provider key mapping moved out of desktop UI and into shared core
- [x] Model mapping behavior is covered by tests

### P1.8 MCP And Google Drive Migration

- [x] Generic MCP settings flow exists
- [x] Dedicated Google Drive settings navigation exists
- [x] Dedicated Jira MCP settings navigation exists
- [x] Dedicated Google Drive setup parity exists for desktop Phase 1 scope
- [x] Dedicated Jira MCP setup parity exists for desktop Phase 1 scope
- [ ] Chat/workflow setup can later select MCP config from migrated flow

### P1.9 Runner Status Migration

- [x] Full runner diagnostics screen exists
- [x] Header runner status trigger exists
- [x] Header opens runner health modal
- [x] Modal shows base URL, version, workspace, platform, started time, and error
- [x] Offline runner blocking/recovery behavior is enforced in desktop shell/navigation

### P1.10 Verification And Cutover Readiness

- [x] Phase 1 checklist file exists
- [x] Shared core unit tests exist
- [x] Desktop UI/state transition tests for screen transitions exist
- [x] Runner API contract tests exist
- [x] Import-boundary checks exist
- [ ] Admin Web migrated routes still pass through shared core
      web-side; ignored in current desktop-only pass
- [ ] Manual smoke checklist is written and executed

## Current Signoff Position

Phase 1 is still not ready for final signoff.

Highest remaining blockers:

1. Missing proof for end-to-end project binding resolution during chat launch
2. Missing workflow/step shared-use-case coverage
3. Missing shared-core artifact auto-sync / provider-switch parity proof
4. Manual smoke checklist is written but not yet executed
