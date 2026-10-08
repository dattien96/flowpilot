# BUG-636 — `.flowpilot/adjudications.ndjson` not in `RunnerLedgerBookkeepingPaths()` → the engine's own adjudication ledger flags as scope drift and parks the writer on its own bookkeeping

- **ID:** BUG-636
- **Severity:** Medium-High — fires on every sprint that ran adjudications
  (debates/verdicts); the file is appended mid-turn inside the exact diff
  window the frozen-scope gate observes, so it false-positives repeatedly.
- **Status:** OPEN — workaround: `agent-loop/continue` past the park
  (amend correctly refuses the path — `.flowpilot/**` is not a concrete
  code target); real fix is a one-line exemption.
- **Found:** run-306526 (`vibe-tasks` CP-04, Task-042 sprint),
  2026-10-06T03:11 — drift gate blocked on
  `.flowpilot/adjudications.ndjson` ("not written via this leg's tool
  calls") alongside the legit new-path entry. The adjudication ledger was
  written by the runner itself when adjudication adj-5/6/7 landed earlier
  in the same turn window.

## Symptom

```
flow gate block: flow scope drift: wrote outside the frozen contract's
declared paths: .flowpilot/adjudications.ndjson,
core/security-rasp/src/main/cpp/test/rasp_engine_spec_coverage_test.cpp;
not written via this leg's tool calls: .flowpilot/adjudications.ndjson
```

`RunnerLedgerBookkeepingPaths()` (changecontract/frozen_scope.go:77-87)
exempts `manifest.json`, `ledger/chat_summary.ndjson`,
`ledger/feature_history.ndjson`, `ledger/.cursor`,
`ledger-needs-update`, `catalog/features.ndjson`, and
`gate-metrics.ndjson` — but NOT `adjudications.ndjson` at the
`.flowpilot/` root, which the runner appends on every adjudication record.

## Defect

Same defect class as BUG-327, BUG-554, BUG-457: runner-internal
observability writes inside the drift-gate's own diff window. The
exemption list grew one file at a time as each new ledger surfaced live;
`adjudications.ndjson` (introduced by the adjudication/owner-debate
machinery) was never added.

Not amendable via `agent-loop/amend` — `IsAmendableDriftPath` correctly
rejects `.flowpilot/**` as "not a concrete code target" (CA-427: a writer
must not be able to amend itself into rewriting gate state). The path
lands in `unamendablePaths` and the gate re-fires whenever a turn
coincides with an adjudication append.

## Expected fix direction

- Add `path.Join(".flowpilot", "adjudications.ndjson")` to
  `RunnerLedgerBookkeepingPaths()` — exact path only, per CA-427 Finding 2
  (no `.flowpilot/**`-wide exemption; forged contract/settings siblings
  must still drift).
- Sweep for siblings written by the same machinery before closing:
  `workflow_drift_events.json`, `gate-metrics.ndjson` (already listed),
  `canonical-pending/*.ndjson`, `contracts/*.ndjson` — decide which are
  pure observability (exempt) vs gate inputs (still enforced).
- Regression test shape: append an adjudication record mid-turn, run the
  frozen-scope check, assert no drift violation for that path while a
  forged `.flowpilot/contracts/frozen_contracts.ndjson` edit still parks.
