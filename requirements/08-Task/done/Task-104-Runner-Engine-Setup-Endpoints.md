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
- Parent Documents: [CP-34: Init / Setup Tool](../../07-Coding-Plan/todo/CP-34-Init-tool.md), [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- Child Documents: `None`
- Related Documents: [Task-101: Flow Skill Pack Install](./Task-101-Flow-Skill-Pack-Install.md), [Task-102: Tooling Check And Capability Profile](./Task-102-Tooling-Check-And-Capability-Profile.md), [Task-105: Engine Setup UI Tab](./Task-105-Engine-Setup-UI-Tab.md)
- Replaces: `None`
- Tags: `local-runner, http, engine-setup, skill-pack, tooling`

## AI Quick View

### Summary

- Expose engine setup over HTTP so the desktop app can inspect a bound repo and trigger initialization or re-sync on demand.
- The shipped contract uses `projectId` in the path plus an explicit `workingDirectory`, because the runner does not own Supabase directory bindings.
- `POST …/engine/init` performs tooling check, skill-pack sync, ledger build, catalog build, and persists `.flowpilot/engine-init.json`.

### Current Ask

- Deliver the runner transport for CP-34 P-1 and document the final contract used by the desktop settings surface.

### Key Decisions

- `T-1` Endpoints require an explicit `workingDirectory`; the runner does not have enough data to resolve project bindings from `projectId` alone.
- `T-2` `POST …/engine/init` runs `tooling.CheckAll`, `skillpack.Install`, `changeledger.Build`, and `featurecatalog.Build`, then returns fresh status plus the persisted init result.
- `T-3` Trigger-specific gating is handled inside the same init path: `trigger=bind` skips when `.flowpilot` already exists, `tooling.json` exists, and the bundled skill-pack is current.
- `T-4` The FlowPilot repo root and its subdirectories are rejected with `400 self_repo_forbidden`.

### Constraints

- Keep the transport inside the existing `runner` HTTP surface and reuse the already-built packages instead of duplicating install logic.

### Open Questions

- None blocking. (Auto-install of gitnexus/rtk is `CP-34 Q-1`, out of scope.)

### Source Refs

- `CP-34 §4.1`; `SD-17 §6.4`, §7.3, §9; `SS-14 AC-12`, AC-13, AC-1.

## 1. Goal

A bound project's engine state is readable and initializable over HTTP, scoped to the target repo, idempotent, and safe against self-targeting.

## 2. Parent Links

- coding plan: `CP-34` P-1
- tech design: `SD-17` §6.4, §7.3
- system spec: `SS-14` AC-12, AC-13, AC-1
- specific upstream ids: `P-1`, `AC-12`, `AC-13`

## 3. Trigger

The desktop Engine Setup section and bind-time auto-init both need a reusable runner endpoint for status, install, and non-fatal re-sync.

## 4. Exact Change

- `T-1` Register `GET /client/projects/{projectId}/engine/status?workingDirectory=<abs-path>` and return `EngineStatusResponse` with tooling state, capability profile, skill-pack status, and last init metadata.
- `T-2` Register `POST /client/projects/{projectId}/engine/init` with body `{ workingDirectory, trigger }`; execute tooling check, skill-pack sync, change ledger build, and feature catalog build in one path.
- `T-3` Persist `.flowpilot/engine-init.json` so the UI can show the last trigger, outcome, and step-level details.
- `T-4` Enforce self-repo rejection and bind-trigger skip gating inside the runner handler.
- `T-5` Cover the contract with runner tests for happy path, self-repo rejection, and bind-trigger skip.

## 5. Touched Areas

- files: `apps/local-runner/internal/runner/engine_setup.go`, `apps/local-runner/internal/runner/engine_setup_test.go`, `apps/local-runner/internal/runner/interactive_handlers.go`, `apps/local-runner/internal/skillpack/install.go`
- modules: `runner`, `skillpack`, `tooling`, `changeledger`, `featurecatalog`
- routes: `GET/POST /client/projects/{projectId}/engine/*`
- tables: none

## 6. Acceptance Check

Detailed DoD checklist:

- [x] `GET /client/projects/{projectId}/engine/status` is registered and returns real engine state for the provided `workingDirectory`.
- [x] `POST /client/projects/{projectId}/engine/init` accepts `{ workingDirectory, trigger }` and returns fresh status after the init flow completes.
- [x] The init flow runs tooling check, skill-pack install/re-sync, change-ledger build, and feature-catalog build from a single runner path.
- [x] The runner persists `.flowpilot/engine-init.json` with step-level results that the UI can read back later.
- [x] Missing tools degrade capability/status without turning the whole init flow into a 500.
- [x] The FlowPilot repo root and subpaths are rejected with `self_repo_forbidden`.
- [x] Bind-trigger calls skip when the repo is already initialized and the bundled skill-pack is current.
- [x] Runner tests cover happy path, self-repo rejection, and bind-trigger skip behavior.

## 7. Out of Scope

- The UI tab (Task-105); bind-time auto-init (Task-106); auto-installing external binaries (`CP-34 Q-1`).

## 8. Completion Notes

- result: implemented in the local runner; the final contract uses explicit `workingDirectory` instead of runner-side binding lookup.
- follow-ups: Task-105 consumes these endpoints and Task-106 uses the same init path for best-effort bind-time orchestration.
- upstream docs updated: `CP-34` aligned to the shipped endpoint contract and manual test flow.
