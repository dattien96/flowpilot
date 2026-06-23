# Task-106: Bind-Time Auto-Init Orchestration

## Metadata

- Document ID: `Task-106`
- Title: `Bind-Time Auto-Init Orchestration`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-23`
- Last Updated: `2026-06-23`
- Parent Documents: [CP-34: Init / Setup Tool — Desktop Engine Page & Project Auto-Init](../../07-Coding-Plan/todo/CP-34-Init-tool.md), [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- Child Documents: `None`
- Related Documents: [Task-104: Runner Engine-Setup Endpoints](./Task-104-Runner-Engine-Setup-Endpoints.md), [Task-105: Desktop Settings Engine Page](./Task-105-Engine-Setup-UI-Tab.md), [Task-096: Commit-History Ledger](./Task-096-Commit-History-Ledger.md), [Task-097: Feature Catalog And Resolver](./Task-097-Feature-Catalog-And-Resolver.md), [CP-31: Auto-Document Process](../../07-Coding-Plan/done/CP-31-Auto-Document-Process.md)
- Replaces: `None`
- Tags: `desktop, bind, orchestration, engine-init, non-fatal`

## AI Quick View

### Summary

- Project bindings still trigger background engine init from desktop project create/save flows.
- This slice only orchestrates project-local engine assets; global tooling lives on the separate `Settings -> Engine` page.
- Bind-time init is async and best-effort, and the last-init result is later shown on the Engine page.

### Current Ask

- Keep CP-34 `P-3` attached to the real desktop binding save flows while the user-facing Engine page stays separate.

### Key Decisions

- `T-1` Auto-init launches from desktop project create and project save flows because those are the only places that own binding data.
- `T-2` Auto-init is async and best-effort; project create/save must succeed even if engine init is partial or fails.
- `T-3` Only project-local state is initialized here: skill-pack sync, ledger, catalog, and last-init state.
- `T-4` Bind-trigger skip logic is project-local and uses `.flowpilot/engine-init.json` plus current skill-pack freshness.
- `T-5` The shared runner init endpoint may target FlowPilot itself when it is intentionally the bound project; isolation is maintained by per-project binding scope.

### Constraints

- Reuse Task-104's runner init path instead of duplicating engine logic in project save flows.

### Open Questions

- None blocking.

### Source Refs

- `CP-34` `P-3`
- `Task-104`
- `Task-105`
- `SD-17 §7.3`
- `SS-14 AC-1`, `AC-12`, `AC-13`

## 1. Goal

Ensure project binding create/save flows leave project-local engine state ready in the background without coupling the global tooling UI to project settings.

## 2. Parent Links

- coding plan: `CP-34`
- tech design: `SD-17`
- system spec: `SS-14`
- specific upstream ids: `P-3`, `AC-1`, `AC-12`, `AC-13`

## 3. Trigger

The runner still cannot infer project bindings on its own, so bind-time engine init had to remain attached to desktop project flows even after the manual Engine UI moved to its own settings page.

## 4. Exact Change

- `T-1` Keep a shared desktop helper that calls the runner init endpoint with `{ workingDirectory, trigger: "bind" }`.
- `T-2` Invoke that helper after binding-bearing save flows in desktop `ProjectsSettings`: project create and project save.
- `T-3` Reuse the Task-104 runner path so bind-trigger init performs tooling check, skill-pack sync, change-ledger build, and feature-catalog build.
- `T-4` Persist last-init outcome for later display on the dedicated Engine page.
- `T-5` Do not re-embed the manual engine UI in project settings; only the background orchestration stays there.

## 5. Touched Areas

- files:
  - `apps/desktop-flowpilot/src/components/settings/ProjectsSettings.tsx`
  - `apps/desktop-flowpilot/src/components/settings/projectEngine.ts`
  - reused Task-104 runner files
- modules:
  - desktop project save flows
  - runner init orchestration
  - skillpack
  - tooling
  - changeledger
  - featurecatalog
- routes:
  - none new
- tables:
  - none

## 6. Acceptance Check

Detailed DoD checklist:

- [x] Desktop project create flow triggers best-effort bind-time engine init for each normalized binding.
- [x] Desktop project save flow triggers best-effort bind-time engine init after bindings are persisted.
- [x] The same bind-time helper is reused across both desktop save paths.
- [x] The helper passes `trigger: "bind"` and explicit binding `localPath` values to the runner.
- [x] Bind-time failures do not fail project save or binding actions.
- [x] The runner-side bind gate skips already-initialized repos when `engine-init.json` exists and the current skill-pack is already installed.
- [x] Bind-trigger init can target FlowPilot itself when FlowPilot is intentionally bound as the project.
- [x] Bind-trigger init persists last-init details that the dedicated Engine page can display later.
- [x] Change-ledger and feature-catalog build run through the reused runner init path.
- [x] Project settings no longer host the manual engine screen; they only host the background orchestration hook.

## 7. Out of Scope

- Runner transport details (Task-104).
- Desktop Engine page composition (Task-105).
- CP-31 normalization internals.

## 8. Completion Notes

- result: implemented through desktop project create/save flows plus the shared runner init endpoint.
- follow-ups: if a future callable normalization entrypoint exists, it can be added to the same runner init path rather than to the desktop save handlers.
- upstream docs updated: `CP-34` now separates the dedicated Engine page from the project-save orchestration path.
