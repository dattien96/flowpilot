# CP-05 Task Breakdown

## Status

- Phase 1 `Planning`: complete
- Phase 2 `Architecture`: complete
- Phase 3 `TDD`: complete
- Phase 4 `Coding`: pending
- Phase 5 `Review`: pending

## Execution Order

1. Add the CP-05 migration after `supabase/migrations/20260519070000_projects_uuid_baseline.sql`.
2. Update integration domain contracts and add a dedicated integration gateway.
3. Extend the demo and Supabase gateway bundles plus `browser-factory`.
4. Extend local-runner domain types, HTTP gateway, and Go HTTP server contract for integration test/retry.
5. Replace the hard-coded MCP placeholder in `apps/admin-web/src/routes/_authenticated/projects/$projectId/settings.tsx`.
6. Add tests for route behavior and gateway logic.
7. Run targeted verification and GitNexus change detection before any commit.

## Coding Tasks

### 1. Migration

- Create `supabase/migrations/<new-timestamp>_cp05_project_mcp_context.sql`.
- Own the `integrations` table in CP-05.
- Include:
  - `project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE`
  - `type`, `label`, `config_encrypted`, `status`, `last_synced_at`, `last_error`
  - indexes on `project_id` and `(project_id, type)`
  - authenticated MVP RLS policies only

### 2. Domain Contracts

- Update `apps/admin-web/src/domain/model/entity/integration.ts`.
- Add `apps/admin-web/src/domain/model/payload/integration-payload.ts`.
- Add `apps/admin-web/src/domain/gateway/integration-gateway.ts`.
- Extend:
  - `apps/admin-web/src/domain/model/entity/local-runner.ts`
  - `apps/admin-web/src/domain/gateway/local-runner-gateway.ts`

### 3. Repository Layer

- Update `apps/admin-web/src/data/repository/browser-factory.ts`.
- Update `apps/admin-web/src/data/repository/supabase/supabase-gateway-bundle.ts`.
- Update `apps/admin-web/src/data/repository/demo/demo-gateway-bundle.ts`.
- Update `apps/admin-web/src/data/repository/demo/demo-store.ts`.
- Keep `ProjectGateway` unchanged.

### 4. Local Runner Contract

- Update `apps/admin-web/src/data/repository/local-runner/http-local-runner-gateway.ts`.
- Update:
  - `apps/local-runner/internal/runner/types.go`
  - `apps/local-runner/internal/runner/runner.go`
  - `apps/local-runner/internal/cli/root.go`
- Add a runner endpoint for project integration connection attempts.

### 5. Project Settings UI

- Replace placeholder MCP state in `apps/admin-web/src/routes/_authenticated/projects/$projectId/settings.tsx`.
- Preserve the existing storage-preference and project-team-link sections.
- Add:
  - integration list
  - add/edit panel
  - delete action
  - test/retry action
  - status badges and detail rows for `lastSyncedAt` and `lastError`

### 6. Tests

- Add route behavior tests for the project settings page.
- Add demo repository tests for integration CRUD behavior.
- Add Supabase repository tests for integration row mapping and mutation payloads.
- Add local-runner HTTP gateway contract tests.

## Review Checklist

- No new full-featured integration management UI outside project settings.
- `ProjectGateway` remains focused on project CRUD only.
- `awaiting_oauth` is represented end to end.
- Demo mode and Supabase mode expose the same integration contract.
- Local-runner connection trigger is a separate orchestration contract, not a DB write method.
