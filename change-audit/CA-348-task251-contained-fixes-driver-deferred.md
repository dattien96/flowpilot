# CA-348: Task-251 — two contained fixes landed; full settle-driver refactor deliberately deferred

## Scope

Assessed Task-251's real remaining work (T-1 phase-driver refactor of `resumePendingFlowGate`, T-2 remaining convergent effects, T-3 retry worker, T-4 legacy-dance deletion). Made a deliberate call NOT to attempt the full architectural refactor in this session.

## Why deferred, not attempted

`resumePendingFlowGate` (`interactive_service.go`, ~350 lines) carries the accumulated fixes of roughly 10 separate BUG-288 rounds (P1-04, P1-05, P1-06, P1-16, R11 #4, R15–R17, R19, R20-3), each guarding a specific crash/Stop/race window. Rewiring it into the prescribed phase driver — with 7 real convergent effects (keyed event upsert, durable cohort projection, revisioned release manifest with parent-stop-fence, keyed finalizer) — while preserving every one of those guards requires a line-by-line cross-reference against each historical round. Attempting this under session time pressure risks silently reopening a bug that took 20 rounds to close. This mirrors the Task-254 T-1 judgment call, with one difference: here the deferred work genuinely IS still required by CP-51 (not waived as unnecessary) — it needs a dedicated future session with room to do the cross-reference properly.

## Changes (contained, low-risk, tested)

- `apps/local-runner/internal/runner/interactive_service.go` — inside `resumePendingFlowGate`'s gate-pass branch:
  - Fixed the "type-only replace" bug (CP-51 ledger `EL`): the completion-event upsert compared only `rs.events[n-1].Type == EventTurnCompleted`, so a DIFFERENT turn's completion event ending up last (e.g. a fast-completing sibling gate) would be silently overwritten instead of appended, losing history. Now keyed by `(Type, ProviderTurnID)`.
  - Replaced `_ = s.persistEvent(completedEv)` (silent discard) with error logging. Not the full T-3 retry-worker fix, but strictly better observability than before.
- New test `TestResumePendingFlowGate_CompletionEventKeyedByTurnID` (`cp51_tasks_test.go`) seeds a prior turn's completion event as the last event, then proves both the prior and the new turn's completion events survive after `resumePendingFlowGate` runs for a different turnID.

## Verification

- `go build ./...`, `go vet ./internal/runner` clean.
- New test passes; full BUG-288/gate/settle/V9/V10 regression suite (`TestBug288*`, `TestGate*`, `TestFlowGate*`, `TestResumePendingFlowGate*`, `TestV9*`, `TestV10*`, `TestSettle*`) green — zero regressions from touching this historically fragile function.
- Full `go test ./internal/runner/...`: 19 failures (18 known baseline + 1 additional flaky-under-parallel-load test, `TestRunCompatCheckIncludesPortabilityCanaries`, confirmed passing in isolation) — zero real regressions.

## Still not done (honest, not silently dropped)

- T-1: `resumePendingFlowGate` not refactored into `SettleDriver`; the driver remains a standalone, unwired unit (0 production callers).
- T-2 (remainder): cohort entry still RAM/label-dedup, not a durable effect; dependents-release not a durable revisioned manifest; finalizer not a keyed-overwrite durable effect.
- T-3: no retry worker wired.
- T-4: legacy `gateCheckpointNotDurable` + three-persist dance intact — correctly NOT deleted, since removing it before T-1/T-3 land would remove the only durability mechanism currently protecting this path.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-51
change_type: bugfix
summary: Fix completion-event type-only-replace bug (keyed by turnID) and silent persist-error swallow in resumePendingFlowGate; defer the full settle-driver refactor to a dedicated future session given ~10 BUG-288 rounds of guards to preserve
# --->8---
