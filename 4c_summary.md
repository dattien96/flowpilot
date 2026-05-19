# CP-05 Project MCP Context - 4C Summary

## Context

- Active implementation surface: `apps/admin-web/src/routes/_authenticated/projects/$projectId/settings.tsx`
- Current page already owns:
  - artifact storage preference
  - MCP placeholder content
  - project-team link management
- Current data access pattern in admin-web:
  - route `loader` reads via `createGatewayBundle()`
  - route-local `useMutation()` performs writes
  - `router.invalidate()` refreshes the page after mutation
- Existing domain/repository state:
  - `apps/admin-web/src/domain/model/entity/integration.ts` exists but is incomplete for CP-05
  - there is no dedicated integration gateway yet
  - `apps/admin-web/src/data/repository/browser-factory.ts` already composes project, team, workflow, and local-runner gateways
- Migration baseline:
  - `20260519070000_projects_uuid_baseline.sql` is present and must be applied before CP-05
  - `integrations.project_id` should reference `projects(id)` as `UUID`
- Local runner state:
  - current HTTP API supports `/health`, `/providers`, `/skills`, `/flows`, `/storage-driver`, `/backup`, `/artifacts`, `/execute`
  - there is no MCP integration trigger endpoint yet

## Command

- Design CP-05 on top of the existing project settings route, not a new settings surface.
- Scope must cover:
  - `integrations` table migration owned by CP-05
  - domain model updates for `label`, `status`, `lastError`
  - gateway contract for project integration CRUD
  - demo and Supabase repository support
  - project settings UI for list/add/edit/delete/retry connection
  - local-runner trigger contract for connection test/retry
  - route-behavior and gateway-logic tests

## Constraints

- Do not create a duplicate integration manager under `/_authenticated/settings/integrations`.
- `team_members` remains planning-only and not auth-bound; CP-05 must not try to re-scope it.
- CP-10 owns later hardening; CP-05 should only add MVP-safe authenticated RLS for `integrations`.
- `ProjectGateway` is high-risk to widen:
  - GitNexus impact on `ProjectGateway`: `CRITICAL`
  - direct blast radius includes both gateway bundles and multiple project/context/dashboard usecases
- `ProjectSettingsContent` and current `Integration` entity are low-risk extension points.
- UI component inventory is small (`Button`, `Badge`); modal/drawer UX will need a lightweight route-local implementation.
- Current admin-web project flows still use direct gateway calls rather than dedicated TanStack Query feature hooks; CP-05 should follow that pattern.

## Criteria

- Reuse `apps/admin-web/src/routes/_authenticated/projects/$projectId/settings.tsx` as the only CP-05 UI surface.
- Create a CP-05 migration after the UUID baseline migration.
- Persist and display:
  - provider `type`
  - user `label`
  - `status` including `awaiting_oauth`
  - `last_synced_at`
  - `last_error`
- Keep persistence and orchestration separated:
  - integration CRUD in a repository gateway
  - connection test/retry in the local-runner gateway
- Provide concrete test targets for:
  - settings route behavior
  - demo repository logic
  - Supabase repository mapping
  - local-runner HTTP contract

## Options

### Option A: Extend `ProjectGateway`

- Pros:
  - fewer new files
  - route can stay on one gateway name
- Cons:
  - `ProjectGateway` is already `CRITICAL` blast-radius
  - mixes MCP integration lifecycle into generic project CRUD
  - forces both demo and Supabase bundles to grow a shared high-risk interface

### Option B: Add `IntegrationGateway` and keep test/retry on `LocalRunnerGateway`

- Pros:
  - isolates CP-05 persistence from shared project CRUD
  - fits existing bundle composition pattern (`projectGateway`, `teamGateway`, `localRunnerGateway`, etc.)
  - keeps orchestration responsibility with the local runner instead of the database gateway
- Cons:
  - introduces one new gateway file and one new payload contract file

### Option C: Build the full manager in `/_authenticated/settings/integrations`

- Pros:
  - route already exists
- Cons:
  - violates the "no duplicate settings surface" constraint
  - breaks the CP-04 requirement that project settings is the active surface

## Recommendation

- Choose **Option B**.
- Keep CP-05 storage and UI work anchored to the project settings route.
- Add a new `IntegrationGateway` for CRUD.
- Extend `LocalRunnerGateway` with a connection trigger contract for `test` and `retry`.
- Leave `ProjectGateway` focused on project CRUD and project metadata.
