# Task-102: Tooling Check And Capability Profile

## Metadata

- Document ID: `Task-102`
- Title: `Tooling Check And Capability Profile`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-23`
- Last Updated: `2026-06-23`
- Parent Documents: [CP-35: Context And Regression Engine Rollout](../../07-Coding-Plan/inprogress/CP-35-Context-And-Regression-Engine-Rollout.md), [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- Child Documents: `None`
- Related Documents: [CP-34: Init Tool](../../07-Coding-Plan/done/CP-34-Init-tool.md), [Task-098: GitNexus Structure Provider](./Task-098-GitNexus-Structure-Provider.md)
- Replaces: `None`
- Tags: `tooling, capability-profile, gitnexus, rtk, local-runner`

## AI Quick View

### Summary

- Detect and health-check the external tools the engine relies on (GitNexus, RTK, node, skill pack) and persist their status.
- Compute a capability profile the engine reads to pick behavior; degrade, never fail, when a tool is missing.
- Machine-specific data — stays local, never synced.

### Current Ask

- Implement `internal/tooling/check.go` + capability profile per CP-35 §4.7 (the install/setup UI stays in CP-34).

### Key Decisions

- `T-1` `CheckTool` via `exec.LookPath` / `--version`; persist `.flowpilot/tooling.json` (`ok|missing|stale`).
- `T-2` `ComputeCapabilityProfile` drives tier selection (e.g., GitNexus missing → file-level structure).

### Constraints

- Runner-side detection only; install actions + setup page are CP-34. `tooling.json` is machine-local, never synced.

### Open Questions

- None blocking.

### Source Refs

- `CP-35 §4.7` (P-7); `SD-17 D-8`, §3.6; `SS-14 AC-13`.

## 1. Goal

A tooling registry + capability profile so the engine knows what is installed and runs at the available tier.

## 2. Parent Links

- coding plan: `CP-35` P-7
- tech design: `SD-17` `D-8`, §3.6
- system spec: `SS-14` AC-13
- specific upstream ids: `P-7`, `D-8`, `AC-13`

## 3. Trigger

The engine depends on external tools that may not be present on a user's machine; it must detect and degrade rather than fail.

## 4. Exact Change

- `T-1` `check.go` — `CheckTool` for `gitnexus`, `rtk`, `node`, `skill_pack`.
- `T-2` persist `.flowpilot/tooling.json` (machine-local).
- `T-3` `ComputeCapabilityProfile` → `{has_gitnexus, has_specs, has_tests, tiers, languages}`; expose to the engine + setup page.
- `T-4` unit tests: status detection + tier selection.

## 5. Touched Areas

- files: `apps/local-runner/internal/tooling/*`
- modules: `tooling`
- routes: status endpoint for the CP-34 setup page
- tables: none (local-only)

## 6. Acceptance Check

- CP-35 P-7 DoD: with GitNexus removed, `tooling.json` shows `missing` and the engine selects file-level structure.
- `go test ./internal/tooling/...` passes.

## 7. Out of Scope

- Installing tools + the desktop setup page (CP-34); GitNexus query logic (Task-098).

## 8. Completion Notes

- result: planned
- follow-ups: CP-34 setup page consumes this registry
- upstream docs updated: none
