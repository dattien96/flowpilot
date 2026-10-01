# Task-457: Vibe-Tasks Entry Points & Live Test

- Document ID: `Task-457`
- Title: `Open vibe-tasks in the mode gate, TUI/Desktop pickers, and cover with unit + LIVE=1 tests`
- Phase: `task`
- Status: `draft`
- Owner: `dat.nguyen`
- Reviewers: ``
- Created: `2026-09-30`
- Last Updated: `2026-09-30`
- Parent Documents: `CP-90` (work item `P-3`, `P-4`)
- Child Documents: ``
- Related Documents: `Task-456`, `CA-1070`, `cp89_live_test.go`
- Replaces: ``
- Tags: `vibe`, `flow`, `picker`, `tests`, `live-test`

## AI Quick View

### Summary

- `vibe-tasks` becomes the third user-startable vibe flow everywhere the
  family gate and pickers enumerate entries: `workingmode` SSOT, TUI
  `/flow` CP arming, desktop flow-tab select.
- `vibe_tasks_live_test.go` adds deterministic in-process coverage plus a
  `LIVE=1` real-binary path in the `cp89_live_test.go` style.

### Current Ask

- A user in vibe mode can pick `vibe-tasks` and point at a CP; dev mode never
  lists it; `vibe-sprint`/`vibe-owner-debate` stay system-only.

### Key Decisions

- `T-1` `vibe-tasks` is explicit-picker only — `DetectVibeEntry` keeps
  mapping a bare CP path to `vibe-cp-ingest` (existing quick-start unchanged).
- `T-2` TUI CP arming generalizes `isVibeCpIngestFlow` →
  `isVibeCpSourcedFlow` (`vibe-cp-ingest` | `vibe-tasks`) — same `@path`
  prefetch + `RejectNonCP` contract.
- `T-3` Desktop mirrors the gate in `workingMode.ts`; the CA-1070 picker
  expectation updates additively (vibe list now 3 entries).

### Constraints

- Provider-less machines skip the LIVE=1 block with a named reason — fixture
  tests carry the always-on coverage, live is additive evidence.

### Source Refs

- `internal/workingmode/workingmode.go`
- `internal/tui/app/{app.go,working_mode.go}`
- `apps/desktop-flowpilot/src/state/workingMode.ts`
- `internal/runner/cp89_live_test.go` (live harness)

## 1. Goal

`vibe-tasks` selectable exactly where `vibe-cp-ingest` is, with the same
CP-source contract — and prove the chain end-to-end.

## 2. Parent Links

- CP-90 P-3/P-4.

## 3. Trigger

- User in vibe mode chooses `vibe-tasks` for a CP with existing tasks.

## 4. Exact Change

- `internal/workingmode/workingmode.go`: `vibeUserFlowIDs` += `vibe-tasks`;
  `FlowPickerOptions(vibe)` returns 3 ids.
- `internal/tui/app/working_mode.go` + `app.go`: `isVibeCpSourcedFlow` at
  picker-expand, suggestion accept, and `/flow` arming sites; arming message
  generic to the flow id.
- `apps/desktop-flowpilot/src/state/workingMode.ts`: picker list +
  `VIBE_USER_FLOW_IDS` += `vibe-tasks`.
- `internal/runner/vibe_tasks_live_test.go` (new): always-on unit tests +
  `LIVE=1` block.
- `apps/desktop-flowpilot/src/state/ca1070_vibe_flow_surface.test.ts`:
  extend vibe picker expectation.

## 5. Touched Areas

- `internal/workingmode`, `internal/tui/app`, desktop `state`, runner tests.

## 6. Acceptance Check

- [ ] `FlowPickerOptions("vibe")` = `[vibe-ingest vibe-cp-ingest vibe-tasks]`.
- [ ] `FlowAllowedForWorkingMode(vibe, vibe-tasks, user)` passes; dev user
  and any system start are rejected; `vibe-sprint` still system-only.
- [ ] TUI `/flow vibe-tasks @requirements/07-Coding-Plan/**/CP-*.md` arms
  with SourceDocID; `/flow` list shows it in vibe mode only.
- [ ] Desktop vibe picker lists `vibe-tasks`; dev picker does not.
- [ ] `vibe_tasks_live_test.go` unit tests pass; LIVE=1 path runs or skips
  with a named-missing-binary reason.

## 7. Out of Scope

- Flow YAML + runner seams — Task-456.

## 8. Completion Notes

