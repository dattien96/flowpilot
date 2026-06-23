# Task-105: Engine Setup UI Tab

## Metadata

- Document ID: `Task-105`
- Title: `Desktop Engine Setup Settings Section`
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
- Tags: `desktop, ui, engine-setup, tooling, skill-pack, electron, react`

## AI Quick View

### Summary

- Add a dedicated **Engine Setup** section inside the desktop app's `Settings -> Projects` surface for the selected project.
- The desktop renderer calls the runner status/init endpoints with the selected binding's explicit `workingDirectory`.
- The section gives the user one place to inspect engine readiness and manually re-run initialization.

### Current Ask

- Deliver the desktop Engine Setup section and its runner-backed fetch/init helpers for CP-34 P-2.

### Key Decisions

- `T-1` Put the user-facing engine UI inside desktop `ProjectsSettings`, because engine state is bound-project data and the desktop settings page already owns project binding management.
- `T-2` Resolve the effective binding from the saved desktop project bindings, then call the runner with that binding's `localPath` as `workingDirectory`.
- `T-3` Manual init uses the same backend flow as bind-time auto-init and refreshes the section state after completion.
- `T-4` When no binding exists or the runner is unavailable, the section stays informative instead of crashing.

### Constraints

- Follow existing desktop settings panel patterns; no new desktop navigation mode is required.

### Open Questions

- None blocking.

### Source Refs

- `CP-34 §4.2`; `SS-14 AC-12`, AC-13; desktop settings `ProjectsSettings.tsx`; runner helper `projectEngine.ts`.

## 1. Goal

A user can see, from the desktop settings area, whether the engine is ready for the bound repo and can manually re-run initialization when needed.

## 2. Parent Links

- coding plan: `CP-34` P-2
- tech design: `SD-17` §3.6, §6.4
- system spec: `SS-14` AC-12, AC-13
- specific upstream ids: `P-2`, `AC-12`, `AC-13`

## 3. Trigger

There was no desktop project settings surface for engine setup, tooling health, or skill-pack state, so users had no way to inspect or repair initialization from the desktop app.

## 4. Exact Change

- `T-1` Add a desktop-local engine helper module that fetches runner engine status/init responses using `RUNNER_URL`.
- `T-2` Extend `ProjectsSettings.tsx` with a new `Engine Setup` collapsible section for the selected project.
- `T-3` Render sections for binding summary, tooling health, capability tier, skill-pack state, last init details, and manual actions.
- `T-4` Handle refresh after manual init and keep the section usable when the runner is offline or no binding exists.
- `T-5` Reuse the same status model for both foreground refresh/init and background bind-trigger reloads.

## 5. Touched Areas

- files: `apps/desktop-flowpilot/src/components/settings/ProjectsSettings.tsx`, `apps/desktop-flowpilot/src/components/settings/projectEngine.ts`
- modules: desktop settings project area
- routes: none new
- tables: none

## 6. Acceptance Check

Detailed DoD checklist:

- [x] Desktop `Settings -> Projects` includes an `Engine Setup` section for the selected project.
- [x] The engine section loads project bindings and resolves an effective `workingDirectory` before calling the runner.
- [x] Desktop-local helper types cover tooling status, capability tier, skill-pack state, and last-init details.
- [x] The section renders real areas for binding summary, tooling health, capability tier, skill-pack state, and action results.
- [x] Manual init triggers the runner endpoint, refreshes the page state, and shows the returned result.
- [x] Missing tools are shown as degraded state rather than hidden or treated as fatal.
- [x] The UI stays informative when there is no directory binding or when the runner is unreachable.
- [x] Desktop build and typecheck succeed with the new Engine Setup section.

## 7. Out of Scope

- The runner endpoints (Task-104); bind-time auto-init (Task-106).

## 8. Completion Notes

- result: implemented as a desktop project settings Engine Setup section backed by the runner engine status/init endpoints.
- follow-ups: Task-106 now feeds this section via persisted init state from bind-time initialization.
- upstream docs updated: `CP-34` aligned to the desktop settings location and manual test flow.
