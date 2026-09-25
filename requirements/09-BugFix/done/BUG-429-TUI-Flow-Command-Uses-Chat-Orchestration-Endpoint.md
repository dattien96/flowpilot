# BUG-429: TUI `/flow` builds its catalog from the chat-orchestration endpoint — dev harnesses unarmable, list shows `(none)`

## Metadata

- Document ID: `BUG-429`
- Title: `/flow` queries builtin-orchestration-options (returns [] for every subMode) instead of flow-picker-options (lists 5) — list↔resolver inconsistent`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-23`
- Parent Documents: evidence `~/fp-beds/lt-evidence/ui/RESULT.md` (BUG-LIVE-UI-6)
- Feature Keys: `tui`, `flow-picker`, `builtin-flows`, `chat-orchestration`

## AI Quick View

### Summary

- `/flow` builds its catalog from `GET /client/chat/builtin-orchestration-options?subMode=bug` — the *chat-mode* orchestration picker — which returns `[]` for every subMode. The real user-startable flows live at `GET /client/flow-picker-options` (returns `task-harness, bug-harness, bug-plan-harness, cp-harness, context-coding-review-synthesis`).
- Consequences: (a) dev harnesses can **never** arm via `/flow` — `/flow task-harness` → `flow "task-harness" not found — configure in Desktop → Settings`; (b) `/flow` list shows `Built-in flows (bug mode): (none)` even in vibe mode where `vibe-ingest` *is* armable — the resolver (`flowCatalogForWorkingMode`) synthesizes vibe flows but the list renders raw `m.flowBuiltins`.
- Live inconsistency: `/flow vibe-ingest` → `Flow armed: vibe-ingest` works while the same `/flow` list shows `(none)`.

### Current Ask

- Fixed and verified in the bug-fix wave — see Completion Notes (implemented 2026-09-23).

## Bug report

### Symptom

`/flow` in the TUI cannot see or arm any dev-mode builtin flow, and its list output is `(none)` in both dev and vibe modes — despite flows being armable via the resolver.

### Expected

`/flow` lists and arms the user-startable builtin flows from `GET /client/flow-picker-options`; list and resolver share the same catalog.

### Actual

- `GET /client/chat/builtin-orchestration-options?subMode=bug` → `[]` (verified live; the endpoint is the chat-mode orchestration picker, not the flow picker).
- `/flow task-harness` → `flow "task-harness" not found — configure in Desktop → Settings`.
- `/flow` list → `Built-in flows (bug mode): (none)` in dev **and** vibe modes; `/flow vibe-ingest` still arms (resolver path differs).

### Impact

All dev harnesses are unarmable from the TUI (`task-harness`, `bug-harness`, `bug-plan-harness`, `cp-harness`, `context-coding-review-synthesis` invisible); the picker UI presents an empty catalog — users cannot start builtin flows without the API. UI-6 task-harness check BLOCKED by this bug.

## Reproduction

1. TUI connected to a runner: `/flow` → list shows `(none)`.
2. `/flow task-harness` → `flow "task-harness" not found`.
3. `curl http://127.0.0.1:<port>/client/chat/builtin-orchestration-options?subMode=bug` → `[]`; `curl .../client/flow-picker-options` → 5 flows.

## Root cause

- `apps/local-runner/internal/tui/app/app.go:~7029` — `cmdFetchFlows` calls `cl.ListBuiltinOrchestrationOptions(ctx, "bug")` (chat-orchestration endpoint) instead of the flow-picker endpoint; result populates `m.flowBuiltins`.
- `apps/local-runner/internal/tui/app/working_mode.go:18` (`flowCatalogForWorkingMode`, vibe synthesis ~:40-45) — the arming resolver synthesizes vibe flows from `workingmode.FlowPickerOptions`, but the `/flow` list renders raw `m.flowBuiltins` → list↔resolver inconsistency.

## Evidence

- `~/fp-beds/lt-evidence/ui/RESULT.md` — BUG-LIVE-UI-6: `api-flow-catalog.txt` (endpoint `[]` vs picker 5 flows), `ui6-arm-task-harness.txt` (`not found`), `ui6-flow-list.txt` / `ui6-vibe-flow-list.txt` (`(none)`), `ui6-vibe-ingest-arm.txt` (arm works via resolver).
- Verified on main worktree HEAD `435e336b`: `app.go` `cmdFetchFlows` → `ListBuiltinOrchestrationOptions(ctx, "bug")`; `interactive_handlers.go:30-31` registers both endpoints (`builtin-orchestration-options`, `flow-picker-options`).

## Severity

- `medium` — entire dev-flow arming surface dead in TUI; workaround via API or vibe-mode resolver quirk.

## Completion Notes (implemented 2026-09-23, CA-926b)

- Fix: new client method `ListFlowPickerOptions(ctx, workingMode)` →
  `GET /client/flow-picker-options?workingMode=…`; `cmdFetchFlows` now calls
  it so list and arm resolve from the same user-startable catalog. The
  section label reflects the actual working mode instead of the hardcoded
  "(bug mode)".
- Tests: `internal/tui/app/bug429_flow_catalog_test.go` (httptest-backed
  client + TUI list/arm), `internal/runner` in-process mux test
  (`TestBug429_FlowPickerEndpointServesDevHarnesses`).
- Live: `/flow` on the live bed listed all five dev harnesses and
  `/flow task-harness` armed successfully ("Flow armed: task-harness").
