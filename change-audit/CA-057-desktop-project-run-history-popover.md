# CA-057: Desktop Project Run History Popover

## Summary

- Added a local-runner history endpoint scoped to a project:
  `GET /client/projects/{projectId}/workflow-runs`.
- Extended interactive run state with created/updated timestamps and latest prompt/result/error summary fields.
- Added desktop `RunnerClient.listRunHistory(projectId)` support in both HTTP and mock clients.
- Added desktop store actions for loading/toggling history and reopening a listed run.
- Added a `History` header control beside `New run` with a popover anchored below it.

## Changed Files

- `AGENTS.md`
- `CLAUDE.md`
- `apps/local-runner/internal/runner/interactive_service.go`
- `apps/local-runner/internal/runner/interactive_handlers.go`
- `apps/local-runner/internal/runner/interactive_service_test.go`
- `apps/desktop-flowpilot/src/types/contract.ts`
- `apps/desktop-flowpilot/src/client/HttpWsRunnerClient.ts`
- `apps/desktop-flowpilot/src/client/MockRunnerClient.ts`
- `apps/desktop-flowpilot/src/state/store.ts`
- `apps/desktop-flowpilot/src/components/RunStatus.tsx`
- `apps/desktop-flowpilot/src/styles.css`
- `requirements/08-Task/done/Task-037-Desktop-Project-Run-History-Popover.md`

## Verification

- `go test ./internal/runner -run TestProjectRunHistoryFiltersRunsByProject`
- `go test ./internal/runner -run 'Test(CatalogAndRegistry|ProjectRunHistoryFiltersRunsByProject)'`
- `npm run typecheck` in `apps/desktop-flowpilot`
- `npx gitnexus status` reports the index is up to date after `npx gitnexus analyze`.
- Current-agent review loop completed with no actionable findings.

## Residual Risk

- History is in-memory and resets with the local runner process, matching the current interactive-run state boundary.
- `npx gitnexus detect_changes --scope all` is documented in `AGENTS.md`, but this installed CLI returns `unknown command`; scope was checked with `git status --short` and `git diff --name-only` instead.
