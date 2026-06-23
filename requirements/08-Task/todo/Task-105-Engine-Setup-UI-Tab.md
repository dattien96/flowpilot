# Task-105: Engine Setup UI Tab

## Metadata

- Document ID: `Task-105`
- Title: `Engine Setup UI Tab`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-23`
- Last Updated: `2026-06-23`
- Parent Documents: [CP-34: Init / Setup Tool](../../07-Coding-Plan/todo/CP-34-Init-tool.md), [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- Child Documents: `None`
- Related Documents: [Task-104: Runner Engine-Setup Endpoints](./Task-104-Runner-Engine-Setup-Endpoints.md), [Task-106: Bind-Time Auto-Init Orchestration](./Task-106-Bind-Time-Auto-Init-Orchestration.md)
- Replaces: `None`
- Tags: `admin-web, ui, engine-setup, tooling, skill-pack, tanstack-router`

## AI Quick View

### Summary

- Add a dedicated **Engine** tab under each project that shows tooling health, capability tier, skill-pack status, and last init outcome.
- The admin-web gateway calls the runner status/init endpoints with the selected binding's explicit `workingDirectory`.
- The tab gives the user one place to inspect engine readiness and manually re-run initialization.

### Current Ask

- Deliver the Engine tab, its gateway contract, and the project navigation entry for CP-34 P-2.

### Key Decisions

- `T-1` Add a project-level `Engine` tab and route at `projects/$projectId/engine.tsx`.
- `T-2` The loader derives the effective binding from project directory bindings, then calls the runner with that binding's `localPath` as `workingDirectory`.
- `T-3` Manual init uses the same backend flow as bind-time auto-init and refreshes the page state after completion.
- `T-4` When no binding exists or the runner is unavailable, the page stays informative instead of crashing.

### Constraints

- Follow existing admin-web route, gateway, and UI component patterns; no separate settings sub-surface.

### Open Questions

- None blocking.

### Source Refs

- `CP-34 §4.2`; `SS-14 AC-12`, AC-13; gateway `http-local-runner-gateway.ts`; nav `project-section-nav.tsx`; pattern `projects/$projectId/settings.tsx`.

## 1. Goal

A user can see, from the project area, whether the engine is ready for the bound repo and can manually re-run initialization when needed.

## 2. Parent Links

- coding plan: `CP-34` P-2
- tech design: `SD-17` §3.6, §6.4
- system spec: `SS-14` AC-12, AC-13
- specific upstream ids: `P-2`, `AC-12`, `AC-13`

## 3. Trigger

There was no project-facing surface for engine setup, tooling health, or skill-pack state, so users had no way to inspect or repair initialization from the app.

## 4. Exact Change

- `T-1` Extend `LocalRunnerGateway` and `HttpLocalRunnerGateway` with `getEngineStatus(projectId, workingDirectory)` and `initEngine(projectId, { workingDirectory, trigger })`, plus the supporting domain model types.
- `T-2` Add `projects/$projectId/engine.tsx` with a loader that fetches project bindings and engine status for the effective binding.
- `T-3` Render sections for binding summary, tooling health, capability tier, skill-pack state, last init details, and manual actions.
- `T-4` Add the `Engine` nav entry to `project-section-nav.tsx`.
- `T-5` Handle refresh after manual init and keep the page usable when the runner is offline or no binding exists.

## 5. Touched Areas

- files: `apps/admin-web/src/routes/_authenticated/projects/$projectId/engine.tsx`, `apps/admin-web/src/components/project/project-section-nav.tsx`, `apps/admin-web/src/components/project/project-section-nav.test.tsx`, `apps/admin-web/src/domain/gateway/local-runner-gateway.ts`, `apps/admin-web/src/domain/model/entity/local-runner.ts`, `apps/admin-web/src/data/repository/local-runner/http-local-runner-gateway.ts`, `apps/admin-web/src/data/repository/local-runner/http-local-runner-gateway.test.ts`, `apps/admin-web/src/routeTree.gen.ts`
- modules: admin-web project area, local-runner gateway layer
- routes: `/projects/:id/engine`
- tables: none

## 6. Acceptance Check

Detailed DoD checklist:

- [x] Project navigation includes an `Engine` tab that routes to `/projects/:id/engine`.
- [x] The engine page loads project bindings and resolves an effective `workingDirectory` before calling the runner.
- [x] Gateway and domain model types cover tooling status, capability tier, skill-pack state, and last-init details.
- [x] The page renders real sections for binding summary, tooling health, capability tier, skill-pack state, and action results.
- [x] Manual init triggers the runner endpoint, refreshes the page state, and shows the returned result.
- [x] Missing tools are shown as degraded state rather than hidden or treated as fatal.
- [x] The UI stays informative when there is no directory binding or when the runner is unreachable.
- [x] Gateway tests and project-nav tests cover the new Engine tab and endpoint mapping.
- [x] Admin-web builds successfully with the new route wired into the generated route tree.

## 7. Out of Scope

- The runner endpoints (Task-104); bind-time auto-init (Task-106).

## 8. Completion Notes

- result: implemented as a project-level Engine tab backed by the runner engine status/init endpoints.
- follow-ups: Task-106 now feeds this tab via persisted init state from bind-time initialization.
- upstream docs updated: `CP-34` aligned to the shipped UI location and manual test flow.
