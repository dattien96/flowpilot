# Task-413: Shared Runner Lifecycle Contract Freeze (CP-81 P-0)

## Metadata

- Document ID: `Task-413`
- Title: `Shared Runner Lifecycle Contract Freeze — SS-24, SD-28, CP-81, Test Steps`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-22`
- Last Updated: `2026-09-22`
- Parent Documents: [CP-81](../../07-Coding-Plan/todo/CP-81-Shared-Runner-Lifecycle.md) `P-0`
- Child Documents: `Task-414`, `Task-415`, `Task-416`, `Task-417`, `Task-418`, `Task-419`, `Task-420`
- Related Documents: [SS-24](../../05-System-Specs/SS-24-Shared-Runner-Lifecycle.md), [SD-28](../../06-System-Tech-Design/SD-28-Shared-Runner-Lifecycle.md), [CP-81-Test-Steps](../../07-Coding-Plan/todo/CP-81-Test-Steps.md), `CA-445`, `CA-474`, `CA-901`, `CA-911`, `CA-913`
- Replaces: `None`
- Tags: `runner-lifecycle, docs, contract, planning`
- Feature Keys: `runner-lifecycle`

## AI Quick View

### Summary

- Freezes the shared runner lifecycle contract before code changes: lease registration/heartbeat/release, state machine, close UX, idle shutdown, force stop, restart reconnect, stale build replacement.
- Establishes the full task chain `Task-414..Task-420` and the manual Windows verification matrix.
- Registers the stable feature key `runner-lifecycle`.

### Current Ask

- Produce implementation-ready docs so no engineer/agent has to reopen product policy while editing runner, TUI, Desktop, and supervisor code.

### Key Decisions

- `T-1` The product contract is `SS-24`; the technical contract is `SD-28`; implementation sequencing is `CP-81`.
- `T-2` CP number is `CP-81` because `CP-80` already exists; task IDs start at `Task-413` because `Task-412` exists.
- `T-3` CA-474 is superseded explicitly: TUI/Desktop can be used concurrently, so ordinary client close cannot globally kill a shared runner.

### Constraints

- Documentation only; no runtime code changes in this task.
- Do not invent alternate endpoint names or UX labels; use the frozen contract.
- Preserve links to prior CA/BUG history so implementers know why the old assumptions changed.

### Open Questions

- None. Timer values, dialog labels, restart policy, stale replacement policy, and supervisor role are resolved in `SS-24`/`SD-28`.

### Source Refs

- Approved plan for shared-runner lease lifecycle and live incident analysis from 2026-09-22.
- Existing lifecycle code: `internal/cli/root.go`, `internal/tui/runnerboot/*`, `internal/tui/client/client.go`, `apps/desktop-flowpilot/electron/main.ts`, `scripts/supervisor.js`.

## 1. Goal

Create a complete, implementation-ready document chain for shared runner lifecycle before any symbol-level code edits begin.

## 2. Parent Links

- coding plan: `CP-81 P-0`
- system spec: `SS-24`
- tech design: `SD-28`

## 3. Trigger

The current system has competing lifecycle authorities: TUI close posts global shutdown, Desktop quit can post global shutdown, TUI-spawned runners are bound to a Windows Job Object, and supervisor independently kills/restarts process trees. The result is both accidental shared-runner kills and stale/orphaned runner leaks.

## 4. Exact Change

- `T-1` New `SS-24-Shared-Runner-Lifecycle.md` with acceptance criteria `AC-1..AC-16`, business rules `BR-1..BR-9`, edge cases `E-1..E-20`.
- `T-2` New `SD-28-Shared-Runner-Lifecycle.md` with decisions `D-1..D-9`, state machine, DTOs, endpoints, process ownership, restart handoff, and failure matrix.
- `T-3` New `CP-81-Shared-Runner-Lifecycle.md` with `P-0..P-7` implementation slices mapped to `Task-413..Task-420`.
- `T-4` New `CP-81-Test-Steps.md` covering Windows/live manual verification.
- `T-5` Append `runner-lifecycle` to `change-audit/FEATURE-KEYS.md`.

## 5. Touched Areas

- files:
  - `requirements/05-System-Specs/SS-24-Shared-Runner-Lifecycle.md`
  - `requirements/06-System-Tech-Design/SD-28-Shared-Runner-Lifecycle.md`
  - `requirements/07-Coding-Plan/todo/CP-81-Shared-Runner-Lifecycle.md`
  - `requirements/07-Coding-Plan/todo/CP-81-Test-Steps.md`
  - `requirements/08-Task/todo/Task-413..Task-420-*.md`
  - `change-audit/FEATURE-KEYS.md`
- modules: documentation only.
- routes: none. tables: none.

## 6. Acceptance Check

### 6.1 Definition of Done (DOD)

- [x] `SS-24` exists and traces all required UX states.
- [x] `SD-28` exists and freezes technical contracts.
- [x] `CP-81` exists and maps every P-slice to a child task.
- [x] `CP-81-Test-Steps` exists and covers Windows/live verification.
- [x] `Task-414..Task-420` exist with exact changes, touched files, signatures/tests, and DoD.
- [x] `runner-lifecycle` is registered in `FEATURE-KEYS.md`.

## 7. Out of Scope

- Runtime implementation.
- Changes to provider adapters, flow engine, scaffold behavior, or UI rendering.
- Committing code or claiming a PR merged.

## 8. Completion Notes

- result: contract documents created; implementation starts at `Task-414`.
- follow-ups: implement `Task-414` next.
- upstream docs updated: `SS-24`, `SD-28`, `CP-81`, `CP-81-Test-Steps`.
