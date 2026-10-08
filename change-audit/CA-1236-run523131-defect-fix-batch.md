# CA-1236 — run-523131 defect fix batch: 21 bugs (BUG-633..636, BUG-644..660) across settle, resume, gate-hook, contract, verdict/drift, sprint boundary, debate/cohort

## Summary

Implementation batch closing the entire live-run-523131 defect capture
(CA-1235) plus the four orphaned earlier captures (BUG-633..636). Every
fix went reproduce-first: a red test pinning the live wedge, then the
smallest change that turns it green, then the surrounding regression
cluster. All 21 BUG docs flipped to `Status: FIXED — CA-1236`.

| BUG | Defect | Fix |
|-----|--------|-----|
| 633 | Straggler-step veto sealed the run `done` before the plan drained | Veto now owns the outcome: audit stays PENDING and the boundary re-checks; evidence-incomplete sprints escalate instead of sealing |
| 634 | `resume()` clobbered `blocked`/`done` park status without unparking | Resume preserves parked statuses (`blocked`, `done`, `tournament_escalation`); only `paused` flips to `running` — `stopped` still revives via the CA-1090 restart path |
| 635 | `submit_review_outcome` never offered to delegate legs | Every flow-driven `agent.code`/`agent.delegate`/`agent.scaffold` child is offered the face; the bridge still rejects settle from non-cohort legs |
| 636 | `.flowpilot/adjudications.ndjson` flagged as scope drift | Added to `RunnerLedgerBookkeepingPaths` |
| 644 | Freeze summary prose drifted from contract JSON (`CMakeLists.txt` dropped as doc) | `CMakeLists.txt`/`*.cmake` reclassified doc→code in `IsDocOrAuditFile`; freeze emits `freeze_summary_mismatch` diag on dropped draft paths; frozen event carries `declared_paths` |
| 645 | `submit_review_outcome` accepted `approved` + `fail`/`blocked` verdict rows | `parseReviewOutcomeInput` rejects the incoherent envelope |
| 646 | `agent-loop/amend` minted contract, left leg parked | Successful amend discharges `pendingFlowGateSettle` via `resumePendingFlowGate` |
| 647 | Completed child leg never settled the flow step mirror | Settle-time stamp: RUNNING/PENDING → DONE directly, WAITING via `settleFlowChildStepTerminalLocked` sibling check |
| 648 | Sprint `done` while tdd FAILED — silent task skip | `vibeSprintEvidenceComplete` now requires the whole write path (tdd `agent.scaffold`, coder `agent.code`, validate `command.validate`) DONE |
| 649 | `decisionTarget` consumed the gate block without arming `proposalTurnPending` | Target run gets `proposalTurnPending` + `clearedAt` armed — it re-prompts instead of re-blocking |
| 650 | Resume-with-feedback produced prose-only turns, no verdict/dispatch | Arms `resumeOutcomeTurn` + `lastFlowControlTurnID`; prose-only completion reprompts once (`repromptResumeOutcomeTurn`) then escalates |
| 651 | Sprint leg change contract inferred from dirty diff, not the frozen contract | `prepareChangeContract` takes the frozen record first — frozen wins over `InferFromDiff` on dirty worktrees |
| 652 | Debate loop repeated identical card + identical verdict forever | Per-entity verdict-signature circuit breaker in `restoreVibeFlowAfterDebate`; durable `VibeDebateVerdictSigs` map on `ProviderSessionState` ↔ `ndjsonSessionRecord`; identical verdict → park for human (no tournament rescue — same verdict re-argues the same card) |
| 653 | Dead-dispatch sweep skipped runs with `pendingFlowGateSettle` | Run-level filter dropped; hub node skipped while parked, dead delegate steps settle via `tryAdvanceFlowFromNode` (advance suppressed when the hub owns the park) |
| 654 | `agent-loop/continue` re-dispatched the parked hub instead of advancing the edge | Routing park continue routes through `applyFlowControl` edge traversal |
| 655 | `zero_delta_progress` fired on non-writing hub/debate turns | Drift detector resolves node `WorkloadClass` — `scan`/`high_reasoning` exempt; `coding` still fires; unknown stays fail-closed |
| 656 | Stale `running` child blocked the resume-confirm gate forever | `childRunningLegIsLiveLocked` checks live work (`turnInFlight`/`agentActivity`), not the persisted status |
| 657 | Leg-level `run_stop` fence had no release path | Fence released at authorized mint seams: `scheduleChildTurn`, `startTurnClearingIntent`, `deliverPendingRestart` — NOT inside `startTurn` (preserves the BUG-593 Stop-vs-claimed-send CAS) |
| 658 | Debate owner legs killed by `provider_limit` had no redrive | `maybeSettleVibeOwnerDebate` shield relaxes when both owners are terminal-failed (stale RUNNING synthesis = dead work); continue on dead `debate_synthesis` routes to the settle ladder; bounded retry re-mounts the cohort |
| 659 | Resume-at-tdd abandoned frozen contracts; once-freeze never re-ran | `forceStartVibeSprintAtTdd` no longer abandons the frozen contract; gate reports abandoned-but-existing contracts honestly |
| 660 | Loop `done` but run stayed `running` until manual `/resume` | Wedge sweep reconciles loop-done runs into settled run status |

## Test-only assertion updates (user-approved)

- `TestRun198699` — accepts `RUNNING|DONE` for the just-spawned leg step
  (BUG-647 stamps DONE at settle-time; asserting RUNNING pinned the bug).
- `TestIsConcreteCodeTarget_LegitPathsSurvive` — `CMakeLists.txt` moved to
  the code bucket (it IS a build-config code target; the old doc-class
  row was the BUG-644 defect).
- `ca1096Setup` / `TestVibeSprintBoundaryRefusesUnfinishedLegs` /
  `TestVibeSprintBoundary_AuditAutoFinalizeParks` /
  `TestRun2062497_RunningNonAncestorStillVetoesAutoAdvance` /
  `TestCA814_*` — fixtures updated to the new correct semantics (clean
  sprint = whole write path DONE; veto owns the outcome via defer/escalate
  rather than returning false into a seal).

## Deadlock found and fixed during the batch

`runTurn` acquires `s.mu` at the offer-ladder site — the BUG-635
eligibility probe must use lock-free helpers (`isFlowEngineDriven` inside
`isSignatureLockedCoderChild` runs BEFORE `s.mu.Lock()`; the new
`delegateClassChild` probe does the same). First attempt deadlocked
self-lock; caught by the focused test hanging + goroutine dump.

## Verification

- Focused tests: all new `bugNNN_*_test.go` files green
  (red→green verified per bug).
- Full `go test ./internal/runner/` — no deterministic regressions;
  remaining failures are environmental (missing `codex` binary, Supabase
  env vars on the host, gitnexus repo label, TempDir cleanup races) and
  match the pre-existing baseline failures.
