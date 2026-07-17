# CA-356 — Hang residuals H-A / H-B / H-C (post run-1618)

## Context

After CA-355 (EventTurnFailed non-cohort + flowStartOnly settle), review found three
sibling hang seams:

| ID | Seam | Symptom |
|----|------|---------|
| **H-A** | `spawnChildRun` async `startTurn` fail (pre-adapter) | Child never emits EventTurnFailed; hub waits forever |
| **H-B** | All flow entry `spawnChildRun` fail (no child run) | Hub active, zero agents |
| **H-C** | F-0 treats `pendingFlowGateSettle` alone as busy | Watchdog never `hub_stalled` on stale settle |

## Changes

### Shared helpers (`interactive_service.go`)

- `clearStaleHubPendingGateSettleLocked` — single clear path (Continue / fail / flowStartOnly)
- `notifyHubOfFlowChildFailureLocked` — step FAILED + clear settle + reinvoke (CA-355 + H-A)
- `handleChildStartTurnFailure` — cohort F-2 + non-cohort H-A
- `notifyHubOfFlowEntrySpawnFailure` — H-B hub reinvoke when no entry spawned

### `flow_executor.go` (H-B)

- Track entry spawn success/fail; on all-fail mark steps FAILED and call
  `notifyHubOfFlowEntrySpawnFailure`.

### `hub_stall.go` (H-C)

- Remove bare `pendingFlowGateSettle` from F-0 busy set.
- Live gate still busy via `postTurnGateCancel != nil`.

## Tests

- `TestNonCohortEntryFailSettlesStepAndClearsHubGateSettle` (CA-355)
- `TestNonCohortPreflightStartTurnFailReinvokesHub` (H-A)
- `TestFlowStartAllEntrySpawnsFailedReinvokesHub` (H-B)
- `TestHubStallFiresDespiteStalePendingGateSettle` (H-C)
- `TestHubStallStillBusyDuringLivePostTurnGate` (H-C no regression)
- `TestFlowStartOnlyClearsSyntheticPendingGateSettle`
- `TestBug289_F0_HubStallBlocksWhenIdleTooLong` still green

```bash
cd apps/local-runner
go test ./internal/runner/ -count=1 -timeout 8m \
  -run 'TestNonCohort|TestFlowStart|TestHubStall|TestBug289|TestResumeFlowWithFeedbackClearsStale|TestCohort|TestFinishTurn|TestTryAdvance'
```

## Live retest

1. Rebuild serve.
2. Review Loop + no Claude → coder failed, hub reinvokes (not hang).
3. Force spawn fail (broken agent) → hub note + reinvoke, not empty active.
4. Inject stale settle, wait 2m → `hub_stalled` card (if no live gate).

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-51
change_type: bugfix
summary: Close hang residuals H-A startTurn fail, H-B entry spawn fail, H-C F-0 stale settle blind spot
# --->8---
