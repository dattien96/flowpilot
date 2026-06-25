# Task-104: Runner Engine-Setup Endpoints

## Metadata

- Document ID: `Task-104`
- Title: `Runner Engine-Setup Endpoints`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-23`
- Last Updated: `2026-06-23`
- Parent Documents: [CP-34: Init / Setup Tool — Desktop Engine Page & Project Auto-Init](../../07-Coding-Plan/todo/CP-34-Init-tool.md), [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- Child Documents: `None`
- Related Documents: [Task-101: Flow Skill Pack Install](./Task-101-Flow-Skill-Pack-Install.md), [Task-102: Tooling Check And Capability Profile](./Task-102-Tooling-Check-And-Capability-Profile.md), [Task-105: Desktop Settings Engine Page](./Task-105-Engine-Setup-UI-Tab.md)
- Replaces: `None`
- Tags: `local-runner, http, engine-setup, tooling, skill-pack`

## AI Quick View

### Summary

- The runner now exposes both machine-global tooling status and project-scoped engine status/init over HTTP.
- Global tooling is checked live through `GET /client/engine/tooling/status`.
- Project status/init still use explicit `workingDirectory` because the runner does not own project binding records.

### Current Ask

- Deliver the final runner contract used by the desktop `Settings -> Engine` page and by bind-time auto-init.

### Key Decisions

- `T-1` `GET /client/engine/tooling/status` returns only machine-global tooling checks (`gitnexus`, `rtk`, `node`).
- `T-2` `GET /client/projects/{projectId}/engine/status?workingDirectory=...` returns project-local engine state, with `skill_pack` derived from the current repo rather than from a persisted tooling file.
- `T-3` `POST /client/projects/{projectId}/engine/init` runs tooling check, skill-pack sync, ledger build, and catalog build, then returns fresh project status.
- `T-4` Bind-trigger skip logic keys off project-local `.flowpilot/engine-init.json` plus current skill-pack freshness.
- `T-5` The runner accepts any intentionally bound target directory, including FlowPilot itself when used as a project.

### Constraints

- Keep the transport inside the existing runner HTTP surface and reuse the existing packages instead of duplicating engine logic.

### Open Questions

- None blocking.

### Source Refs

- `CP-34` `P-1`
- `SD-17 §6.4`, `§7.3`, `§9`
- `SS-14 AC-1`, `AC-12`, `AC-13`

## 1. Goal

Expose the engine setup lifecycle over HTTP with the correct scope split: global tooling for the machine, project init/status for the selected repo.

## 2. Parent Links

- coding plan: `CP-34`
- tech design: `SD-17`
- system spec: `SS-14`
- specific upstream ids: `P-1`, `AC-1`, `AC-12`, `AC-13`

## 3. Trigger

The desktop app needed one global tooling source and one project-local engine source after the UX moved from an embedded project panel to a dedicated Engine page.

## 4. Exact Change

- `T-1` Register `GET /client/engine/tooling/status` and return live machine-global tooling state.
- `T-2` Register `GET /client/projects/{projectId}/engine/status?workingDirectory=<abs-path>` and return project-local engine state.
- `T-3` Register `POST /client/projects/{projectId}/engine/init` with body `{ workingDirectory, trigger }`.
- `T-4` Persist `.flowpilot/engine-init.json` after init runs so the desktop page can show last-init details.
- `T-5` Keep bind-trigger skip gating inside the runner without special-casing FlowPilot's own workspace.
- `T-6` Add runner tests for the global tooling endpoint in addition to the existing project-init/status scenarios.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/interactive_handlers.go`
  - `apps/local-runner/internal/runner/engine_setup.go`
  - `apps/local-runner/internal/runner/engine_setup_test.go`
  - `apps/local-runner/internal/tooling/check.go`
  - `apps/local-runner/internal/tooling/tooling_test.go`
- modules:
  - `runner`
  - `tooling`
  - `skillpack`
  - `changeledger`
  - `featurecatalog`
- routes:
  - `GET /client/engine/tooling/status`
  - `GET /client/projects/{projectId}/engine/status`
  - `POST /client/projects/{projectId}/engine/init`
- tables:
  - none

## 6. Acceptance Check

Detailed DoD checklist:

- [x] `GET /client/engine/tooling/status` is registered and returns live machine-global tooling rows.
- [x] `GET /client/projects/{projectId}/engine/status` is registered and returns real project engine state for the provided `workingDirectory`.
- [x] `POST /client/projects/{projectId}/engine/init` accepts `{ workingDirectory, trigger }` and returns fresh status after the init flow completes.
- [x] The init flow runs tooling check, skill-pack install/re-sync, change-ledger build, and feature-catalog build from one runner path.
- [x] The runner persists `.flowpilot/engine-init.json` with step-level results for later UI display.
- [x] Project status derives `skill_pack` from current repo state instead of treating tooling as a purely persisted per-project file.
- [x] Missing global tools degrade capability/status without turning the whole init flow into a 500.
- [x] The runner accepts the FlowPilot repo when it is intentionally used as the bound target project.
- [x] Bind-trigger calls skip when `engine-init.json` already exists and the bundled skill-pack is current.
- [x] Runner tests cover happy path, runner-workspace targeting, bind-trigger skip behavior, and the global tooling endpoint.

## 7. Out of Scope

- Desktop page composition (Task-105).
- Bind-time auto-init orchestration in project save flows (Task-106).
- Auto-installing external binaries.

## 8. Completion Notes

- result: implemented as one global tooling endpoint plus the existing project status/init endpoints, with no special rejection for FlowPilot when it is intentionally bound as the target.
- follow-ups: Task-105 consumes both scopes on the dedicated Engine page; Task-106 reuses the init endpoint from project create/save flows.
- upstream docs updated: `CP-34` now reflects the split between global tooling and project-local engine state.
