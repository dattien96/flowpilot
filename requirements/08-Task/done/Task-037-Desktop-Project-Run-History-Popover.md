# Task-037 Desktop Project Run History Popover

## Metadata

- Document ID: `Task-037`
- Title: `Desktop Project Run History Popover`
- Phase: `task`
- Status: `done`
- Owner: `Codex`
- Reviewers: `TBD`
- Created: `2026-06-12`
- Last Updated: `2026-06-12`
- Parent Documents: `04-08-Phase8-Cutover-And-Live-Acceptance`
- Child Documents: ``
- Related Documents: `Task-036-Desktop-Provider-Accounts-Sidebar`
- Replaces: ``
- Tags: `desktop-flowpilot`, `local-runner`, `run-history`, `ui`

## AI Quick View

### Summary

- Added a project-scoped run-history endpoint to the local runner.
- Added desktop client/store support for loading only the selected project's runs.
- Added a `History` control beside `New run`; it opens a popover directly below the control and can reopen a listed run.

### Current Ask

- Show the current project's run history beside the desktop `New run` control.

### Key Decisions

- `T-1` Keep history project-scoped at the runner boundary so the desktop cannot accidentally mix projects.
- `T-2` Use the existing in-memory interactive run state for this slice.
- `T-3` Anchor the panel to the history control instead of introducing a full-screen modal.

### Constraints

- Ignore web-admin surfaces for this task.
- Preserve the existing run lifecycle, SSE stream, approval, question, and interrupt routes.

### Open Questions

- None for this slice.

### Source Refs

- `04-08-Phase8-Cutover-And-Live-Acceptance`
- `Task-036`

## 1. Goal

Allow desktop users to inspect and reopen previous runs for the selected project from the header beside the `New run` action.

## 2. Parent Links

- coding plan: `04-08-Phase8-Cutover-And-Live-Acceptance`
- tech design:
- system spec:
- specific upstream ids: `04-08`

## 3. Trigger

The desktop app could start a new run and reconnect the current run, but it did not expose a compact way to see previous runs for the selected project.

## 4. Exact Change

- `T-1` Added `GET /client/projects/{projectId}/workflow-runs` to the local runner.
- `T-2` Tracked created/updated timestamps plus last prompt/result/error on interactive runs.
- `T-3` Extended desktop contracts and runner clients with `listRunHistory(projectId)`.
- `T-4` Added desktop store actions to load history, toggle the popover, and reopen a historical run by replaying its event stream.
- `T-5` Added a header `History` control beside `New run` with a project-filtered popover.

## 5. Touched Areas

- files: `apps/local-runner/internal/runner/interactive_service.go`, `apps/local-runner/internal/runner/interactive_handlers.go`, `apps/local-runner/internal/runner/interactive_service_test.go`, `apps/desktop-flowpilot/src/types/contract.ts`, `apps/desktop-flowpilot/src/client/HttpWsRunnerClient.ts`, `apps/desktop-flowpilot/src/client/MockRunnerClient.ts`, `apps/desktop-flowpilot/src/state/store.ts`, `apps/desktop-flowpilot/src/components/RunStatus.tsx`, `apps/desktop-flowpilot/src/styles.css`
- modules: `local-runner`, `desktop-flowpilot`
- routes: `/client/projects/{projectId}/workflow-runs`
- tables:

## 6. Acceptance Check

- The desktop header has a `History` control beside `New run`.
- Pressing `History` opens a project-scoped run list below the control.
- Runs from other projects are excluded.
- Clicking a listed run resumes/reopens it and replays its timeline.
- Focused runner test and desktop typecheck pass.

## 7. Out of Scope

- Web-admin history UI.
- Persistent run-history storage across runner restarts.
- Deleting, renaming, or archiving historical runs.

## 8. Completion Notes

- result: Implemented and verified locally.
- follow-ups: Back the history endpoint with persisted workflow-run storage when Phase 8 moves beyond in-memory runner history.
- upstream docs updated: `Task-037`
