# BUG-397: Frozen contract vanished between freeze and reproduce-lock — implement blocked

## Metadata

- Document ID: `BUG-397`
- Title: `run-5307 frozen contract present in worktree at freeze, absent at lock attempt → 'coder has no frozen contract after tdd'`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-21`
- Parent Documents: [CP-64-Test-Steps](../../07-Coding-Plan/done/CP-64-Test-Steps.md)
- Feature Keys: `change-contract`, `reproduce-first-gate`

## AI Quick View

### Summary

- `preflight_contract_freeze` reported DONE and contract v1 `12038e86…` (`run_id: run-5307`, `coder_step_id: implement`) was written to `.flowpilot/contracts/frozen_contracts.ndjson` — visible in the working tree at 05:18:28 (reproducer's own `git diff` output).
- At 05:22:00 the reproduce gate accepted and tried to lock the test file: `no active frozen contract for step "implement" in run "run-5307" to lock read-only paths on` (runner.log:10245). Post-run `frozen_contracts-final.ndjson` contains **no run-5307 row at all** and `frozen_contract_events-final.ndjson` has no run-5307 entry.
- The run then blocked `block=requirement` / `coder has no frozen contract after tdd`; implement stayed PENDING.

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.

## Bug report

- **Symptom:** A persisted frozen-contract record disappears between freeze and the reproduce-lock lookup, ~3.5 min later, with no delete/supersede event.
- **Expected:** Once freeze persists a contract row, the lock step and coder dispatch read it; loss of the record is impossible or at minimum loudly fails the freeze step.
- **Actual:** Silent record loss — lock skipped with only a log line, then a hard `coder has no frozen contract after tdd` block; `reproduce_test` still marked DONE despite its postcondition (locked contract) not holding.
- **Impact:** Run unrecoverable mid-flow (implement never dispatches). Incidentally prevented the BUG-389 gamed-RED from dispatching a coder in this run — accidental save, not design.

## Reproduction

1. Run `bug-harness` (opencode) through freeze → reproduce.
2. Intermittent: during the reprompt-heavy reproduce window, the frozen record for the run is lost (occurred once across 5 bug-harness runs this session).
3. Lock attempt fails `no active frozen contract`; flow blocks at implement dispatch.
- runIds: `run-5307` (reproducer `run-5364`); freeze observed 05:18:28, lock failure 05:22:00.

## Root cause

- Suspected mechanism (not fixed; capture-only): a whole-file rewrite of `frozen_contracts.ndjson` from a stale in-memory view dropped the run-5307 line between 05:18:28 (present in worktree) and 05:22:00 (absent at lock lookup). No other run was active in that window, so the rewrite likely came from a reprompt-cycle store operation in run-5307 itself.
- Store write path to audit: `changecontract` frozen-store append/rewrite used by `preflight_contract_freeze`, `LockReproduceTestPaths`, and amend (`internal/changecontract/` frozen store; `runner/flow_validate_audit_dispatch.go` freeze chain) — the exact writer that clobbered the row is unidentified; no `frozen_contract_events` supersede/delete entry was emitted.

## Evidence

- `~/fp-beds/lt-evidence/cp64/RESULT.md` (Bugs table, BUG-LIVE-CP64-04; L-64-2/L-64-3 notes)
- `~/fp-beds/lt-evidence/cp64/BUG-LIVE-CP64-04-frozen-contract-lost.md` — full field report (rated high there)
- `~/fp-beds/lt-evidence/cp64/runner.log` — ~8718 (freeze visible in worktree git diff), :10245 (lock failure), block `coder has no frozen contract after tdd`
- `~/fp-beds/lt-evidence/cp64/frozen_contracts-final.ndjson` — zero run-5307 rows; `frozen_contract_events-final.ndjson` — no run-5307 entry
- `~/fp-beds/lt-evidence/cp64/run5307/` — per-run artifacts

## Severity

- `medium` (assigned; field report rated high) — intermittent silent contract loss; hard-blocks coder dispatch when it hits.

## Completion Notes (implemented 2026-09-23, CA-920b)

- New `contract_state_guard.go`: when a frozen contract is active, destructive git commands targeting `.flowpilot/**` (checkout/restore/reset/clean/rm/stash/switch incl. bare-token forms) are denied at the approval bridge — before YOLO auto-approval. No contract → no guard; safe git unaffected.
- Live root cause confirmed in cp64 run-5307: agent replayed `git checkout -- .flowpilot/contracts/frozen_contracts.ndjson` ×25.
- Unit: `TestBug397_GitRewindOfFlowPilotStateDenied`, `TestBug397_SafeGitCommandsUnaffected`, `TestBug397_NoActiveContract_NoGuard` — green.
