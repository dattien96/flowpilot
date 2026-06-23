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
- Parent Documents: [CP-34: Init / Setup Tool](../../07-Coding-Plan/todo/CP-34-Init-tool.md), [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- Child Documents: `None`
- Related Documents: [Task-104: Runner Engine-Setup Endpoints](./Task-104-Runner-Engine-Setup-Endpoints.md), [Task-096: Commit-History Ledger](./Task-096-Commit-History-Ledger.md), [Task-097: Feature Catalog And Resolver](./Task-097-Feature-Catalog-And-Resolver.md), [CP-31: Auto-Document Process](../../07-Coding-Plan/done/CP-31-Auto-Document-Process.md)
- Replaces: `None`
- Tags: `local-runner, bind, orchestration, engine-init, non-fatal`

## AI Quick View

### Summary

- Make a freshly bound project engine-ready automatically from the desktop app flows that actually create or update project bindings.
- Bind-time init is best-effort and async: it calls the runner init endpoint with `trigger=bind`, allowing the bind itself to succeed even if initialization partially fails.
- The shipped slice covers tooling check, skill-pack sync, ledger build, catalog build, and persisted last-init state; CP-31 normalization was explicitly deferred because this repo has no callable normalization entrypoint.

### Current Ask

- Deliver CP-34 P-3 using the real binding save flows in the desktop app instead of a nonexistent runner-side binding lookup.

### Key Decisions

- `T-1` Auto-init is launched from desktop project create and desktop project save flows, because those are the places where binding data exists in the current product surface.
- `T-2` Auto-init is async and best-effort; the desktop app logs failures and does not block the primary save action.
- `T-3` The runner enforces the staleness gate for `trigger=bind`, skipping when `.flowpilot` already exists, `tooling.json` exists, and the skill-pack is already current.
- `T-4` The FlowPilot repo guard still applies, and the last init result is persisted for the desktop Engine Setup section.

### Constraints

- Reuse Task-104's init path from the desktop app instead of duplicating engine logic in the binding flows.

### Open Questions

- None blocking after the shipped decision to gate bind-trigger init on existing `.flowpilot` state plus skill-pack freshness.

### Source Refs

- `CP-34 §4.3`; `SD-17 §7.3`; `SS-14 AC-12`, AC-1, AC-9; `CP-31`.

## 1. Goal

Binding a project leaves it engine-ready with no manual runner step, without ever risking the project save or directory-binding action itself.

## 2. Parent Links

- coding plan: `CP-34` P-3
- tech design: `SD-17` §7.3
- system spec: `SS-14` AC-12, AC-1, AC-9
- specific upstream ids: `P-3`, `AC-12`

## 3. Trigger

Engine setup needed to happen when a project binding is created or updated, but the runner has no direct access to binding records, so orchestration had to be attached to the desktop settings save flows.

## 4. Exact Change

- `T-1` Add a shared desktop helper that calls the runner `engine/init` endpoint with `{ workingDirectory, trigger: "bind" }` and swallows failures after logging.
- `T-2` Invoke that helper after binding-bearing save flows in desktop `ProjectsSettings`: project create and project save.
- `T-3` Reuse the Task-104 runner init path so bind-trigger init performs tooling check, skill-pack sync, change-ledger build, and feature-catalog build.
- `T-4` Persist last-init outcome for Task-105 via `.flowpilot/engine-init.json`.
- `T-5` Explicitly defer CP-31 normalization from this slice because no normalization entrypoint exists in the current codebase.

## 5. Touched Areas

- files: `apps/desktop-flowpilot/src/components/settings/projectEngine.ts`, `apps/desktop-flowpilot/src/components/settings/ProjectsSettings.tsx`, plus the reused Task-104 runner files
- modules: desktop project save flows, `runner`, `changeledger`, `featurecatalog`, `skillpack`, `tooling`
- routes: none new
- tables: none

## 6. Acceptance Check

Detailed DoD checklist:

- [x] Desktop project create flow triggers best-effort bind-time engine init for each normalized binding.
- [x] Desktop project save flow triggers best-effort bind-time engine init for each normalized binding after bindings are persisted.
- [x] The same best-effort bind-time engine init helper is reused across both desktop save paths.
- [x] The helper passes `trigger: "bind"` and the explicit binding `localPath` to the runner.
- [x] Bind-time failures do not fail the project save or directory-binding action.
- [x] The runner-side bind gate skips already-initialized repos when `.flowpilot` state and skill-pack freshness indicate no work is needed.
- [x] Self-repo protection still prevents FlowPilot from initializing itself.
- [x] Bind-trigger init persists last-init details that the desktop Engine Setup section can display later.
- [x] Change-ledger and feature-catalog build run through the reused runner init path.
- [x] CP-31 normalization is called out as deferred rather than silently omitted.

## 7. Out of Scope

- The endpoints themselves (Task-104); the UI (Task-105); CP-31's normalization internals.

## 8. Completion Notes

- result: implemented through desktop settings binding save flows plus the shared runner init endpoint.
- follow-ups: wire CP-31 normalization later if a stable callable entrypoint is introduced.
- upstream docs updated: `CP-34` updated to reflect that bind-time orchestration lives in the desktop app and that CP-31 normalization is deferred in the current repo.
