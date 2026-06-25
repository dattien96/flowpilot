# Task-105: Desktop Settings Engine Page

## Metadata

- Document ID: `Task-105`
- Title: `Desktop Settings Engine Page`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-23`
- Last Updated: `2026-06-23`
- Parent Documents: [CP-34: Init / Setup Tool — Desktop Engine Page & Project Auto-Init](../../07-Coding-Plan/todo/CP-34-Init-tool.md), [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- Child Documents: `None`
- Related Documents: [Task-104: Runner Engine-Setup Endpoints](./Task-104-Runner-Engine-Setup-Endpoints.md), [Task-106: Bind-Time Auto-Init Orchestration](./Task-106-Bind-Time-Auto-Init-Orchestration.md)
- Replaces: `None`
- Tags: `desktop, ui, engine, settings, tooling, skill-pack, electron, react`

## AI Quick View

### Summary

- The desktop app now has one top-level `Settings -> Engine` page.
- That page explicitly separates machine-global tooling checks from project-local skill-pack, capability, and last-init state.
- The older embedded Engine Setup panel was removed from `ProjectsSettings`.

### Current Ask

- Deliver the desktop Engine page and its runner-backed helper layer for CP-34 `P-2`.

### Key Decisions

- `T-1` Put the user-facing engine UI in its own settings menu entry because tooling is global and should not be hidden inside one project detail screen.
- `T-2` Keep project-local engine inspection on the same page through project and binding selectors.
- `T-3` Reuse one desktop helper module for global tooling fetch, project status fetch, manual init, and bind-trigger auto-init calls.
- `T-4` Keep the page informative when there are no projects, no bindings, or the runner is unavailable.

### Constraints

- Follow existing desktop settings-shell navigation and panel patterns.

### Open Questions

- None blocking.

### Source Refs

- `CP-34` `P-2`
- `Task-104`
- `SS-14 AC-12`, `AC-13`

## 1. Goal

Give the desktop app one clear Engine page where users can inspect global tooling and then inspect or re-sync project-local engine state for any saved project binding.

## 2. Parent Links

- coding plan: `CP-34`
- tech design: `SD-17`
- system spec: `SS-14`
- specific upstream ids: `P-2`, `AC-12`, `AC-13`

## 3. Trigger

After the scope split was clarified, the engine surface could no longer live only inside project details because tooling is machine-global while only skill-pack and init state are project-local.

## 4. Exact Change

- `T-1` Add a dedicated `Engine` settings section to `SettingsShell`.
- `T-2` Add `EngineSettings.tsx` for the new page.
- `T-3` Extend `projectEngine.ts` with a global tooling fetch helper and response mapping.
- `T-4` Render a `Global Tooling` panel driven by `GET /client/engine/tooling/status`.
- `T-5` Render a `Project Skill Pack` panel with project and binding selectors plus project-local engine details.
- `T-6` Remove the earlier embedded `Engine Setup` collapsible section from `ProjectsSettings`.

## 5. Touched Areas

- files:
  - `apps/desktop-flowpilot/src/components/SettingsShell.tsx`
  - `apps/desktop-flowpilot/src/components/settings/EngineSettings.tsx`
  - `apps/desktop-flowpilot/src/components/settings/ProjectsSettings.tsx`
  - `apps/desktop-flowpilot/src/components/settings/projectEngine.ts`
- modules:
  - desktop settings shell
  - desktop engine settings
- routes:
  - none new in desktop
- tables:
  - none

## 6. Acceptance Check

Detailed DoD checklist:

- [x] Desktop settings navigation includes a top-level `Engine` menu entry.
- [x] `Settings -> Engine` renders a `Global Tooling` section backed by the runner global tooling endpoint.
- [x] `Settings -> Engine` renders a project selector and binding selector for project-local engine state.
- [x] Desktop helper types cover global tooling status, project capability tier, skill-pack state, and last-init details.
- [x] The page renders real sections for global tooling, project capability, project tooling summary, skill-pack state, and last init results.
- [x] Manual init triggers the runner endpoint, refreshes local page state, and shows the returned result.
- [x] Missing tools are shown as degraded state rather than hidden or treated as fatal.
- [x] The UI stays informative when there is no directory binding or when the runner is unreachable.
- [x] The older embedded engine panel is removed from `Settings -> Projects`.
- [x] Desktop build and typecheck succeed with the dedicated Engine page.

## 7. Out of Scope

- Runner endpoint implementation (Task-104).
- Bind-time background auto-init from project save flows (Task-106).

## 8. Completion Notes

- result: implemented as a dedicated desktop Engine page backed by runner status/init endpoints.
- follow-ups: Task-106 keeps project save flows feeding last-init state that this page can read later.
- upstream docs updated: `CP-34` now points to `Settings -> Engine` instead of an embedded project settings panel.
