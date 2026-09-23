# BUG-407: `repair-resolution` replay → HTTP 502 `repair is not open` instead of idempotent result

## Metadata

- Document ID: `BUG-407`
- Title: `POST repair-resolution replay with same resolutionId returns 502 dispatch_operator_error — not idempotent by resolutionID (contract row RR)`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-21`
- Parent Documents: [CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation](../../07-Coding-Plan/done/CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md)
- Feature Keys: `dispatch-durability`, `operator-surface`

## AI Quick View

### Summary

- CP51-002: replaying `POST /client/workflow-runs/{runId}/repair-resolution` with the same `resolutionId` after a successful abandon returns `HTTP 502 {"code":"dispatch_operator_error","message":"repair is not open"}` instead of the recorded outcome — violating the CP-51 contract row `RR` ("idempotent by resolutionID, audited").
- Sibling endpoints (`resolve`, `retry-as-new`) replay cleanly via `s.resolutions[resolutionID]`; `BeginRepairResolution` has no resolution-id lookup and `ErrRepairNotOpen` falls through `dispatchOperatorErr` to 502 rather than a 409 conflict.

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.

## Bug report

- **Symptom:** first `repair-resolution` call for run-1302 with `resolutionId:"op-repair-1302-1"` → HTTP 200 `resolved_abandon` rev=3; replaying the identical request → HTTP 502 `dispatch_operator_error` / "repair is not open".
- **Expected:** replay returns the recorded outcome (idempotent by `resolutionId`), matching `resolve`/`retry-as-new` behavior.
- **Actual:** once the repair leaves `open`, any replay errors — and with the wrong status/code (502 transport-shaped error rather than a recorded outcome or a 409 conflict).
- **Impact:** operator tooling cannot safely retry repair resolutions — a network retry after a successful abandon surfaces as a server error, eroding trust in the operator API and breaking the CP-51 two-phase contract guarantee.

## Reproduction

1. Produce a `repair_required` record (e.g. `send_started` surviving stop+crash — see BUG-408).
2. `POST /client/workflow-runs/{runId}/repair-resolution {"resolutionId":"op-repair-…","action":"abandon", "expectedRepairRev":N}` → 200 `resolved_abandon`.
3. Replay the identical request → 502 `repair is not open`.

## Root cause

- `apps/local-runner/internal/runner/dispatch_store_memory.go` `BeginRepairResolution` (~L1243): `if r == nil || r.State != "open" { return 0, nil, ErrRepairNotOpen }` — no `s.resolutions[resolutionID]` lookup before the state check (the mechanism `resolve`/`retry-as-new` use for idempotent replay).
- `ErrRepairNotOpen` then falls through `dispatchOperatorErr` to HTTP 502 instead of mapping to a 409 conflict or returning the recorded resolution.

## Evidence

- `~/fp-beds/lt-evidence/cp51/l51-6b/repair-resolve.json` — HTTP 200 `resolved_abandon` rev=3.
- `~/fp-beds/lt-evidence/cp51/l51-6b/repair-replay.json` — HTTP 502 "repair is not open", same `resolutionId:"op-repair-1302-1"`.
- `~/fp-beds/lt-evidence/cp51/RESULT.md` (BUG-LIVE-CP51-002).

## Severity

`medium` — contract violation on an operator endpoint; replay produces a misleading 502, no data loss but no safe retry semantics.

## Completion Notes (implemented 2026-09-23, CA-922)

- Fix: `BeginRepairResolution` returns typed `*RepairResolutionReplay{Revision, Outcome}` when the repair is already `resolved` under the same `resolutionID` (durable on the record — survives restart); handler maps it to HTTP 200 with the recorded outcome. `ErrRepairNotOpen` now maps to 409 `dispatch_conflict`, not 502.
- Files: `internal/runner/dispatch_store_memory.go`, `internal/runner/dispatch_record.go`, `internal/runner/dispatch_operator.go`.
- Tests: `TestBug407_RepairResolutionReplayReturnsRecordedOutcome` (replay → recorded outcome; different resolutionID still conflicts), `TestBug407_RepairNotOpenMapsToConflict` (409).
