# CA-039: Supabase Runtime Config And Setup Flow

## Scope

- Local-runner Supabase workspace config persistence and validation in `apps/local-runner/internal/runner`.
- Local-runner HTTP endpoints in `apps/local-runner/internal/cli/root.go`.
- Admin-web runtime config resolution, browser/server Supabase bootstrap, and auth/login integration in `apps/admin-web/src`.
- Setup and authenticated settings UI for Supabase configuration.

## Completed

- Added workstation-local Supabase config persistence under `.flowpilot/settings/supabase-config.json` while keeping the service-role key in the local secret store.
- Added local-runner `GET`/`PUT`/`POST validate`/`DELETE` support for `/supabase-config`.
- Added browser-safe runtime config APIs under `apps/admin-web/src/app/api/runtime/supabase-config`.
- Implemented runtime precedence as `saved workspace config > env fallback > demo`.
- Updated browser and server Supabase bootstrap paths to resolve runtime config instead of relying only on env.
- Updated auth/login flows to use runtime status and preserved demo-mode behavior.
- Added `/setup/supabase` and `/settings/supabase`, plus the settings navigation entry.
- Added explicit browser cache resets after setup save so a previously resolved demo bundle cannot survive into the next login/bootstrap cycle.
- Added rollback behavior so service-role secret writes are reverted if config persistence fails.

## Verification

- Passed `go test ./internal/runner -run 'TestSupabaseWorkspaceConfig'` from `apps/local-runner`.
- Passed `go test ./internal/cli ./internal/runner -run 'TestSupabaseWorkspaceConfig'` from `apps/local-runner`.
- Passed `npm test -- src/routes/setup.supabase.test.tsx src/features/auth/auth-provider.test.tsx src/features/auth/require-auth.test.ts src/data/repository/browser-factory.test.ts src/app/api/runtime/supabase-config/_shared.test.ts` from `apps/admin-web`.
- Passed `npm run test -- route-map` from `apps/admin-web`.
- Passed `npm run build` from `apps/admin-web`.
- Passed `git diff --check`.
- Ran GitNexus impact analysis before edits on the high-risk bootstrap symbols and kept the final implementation within the expected Supabase/auth/runtime scope.

## Residual Notes

- Full `go test ./...` in `apps/local-runner` still has unrelated environment-dependent failures outside this task.
- The admin-web production build still emits the existing `node:path` / `node:fs` browser-compatibility warnings and the existing large-chunk warning.

# ---8<--- flowpilot:change-ledger
feature_key: supabase-config
source_doc_id: CA-039
change_type: feature
summary: Supabase Runtime Config And Setup Flow
# --->8---
