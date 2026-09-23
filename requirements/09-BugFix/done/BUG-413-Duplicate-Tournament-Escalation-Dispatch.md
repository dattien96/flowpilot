# BUG-413: Duplicate tournament escalation dispatch — second `continue` spawns another child and overwrites gate reason

## Metadata

- Document ID: `BUG-413`
- Title: `continue on already-tournament_escalation run re-fires escalation → run-31-tournament-2 spawned, gateReason rewritten, first child orphaned`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-23`
- Parent Documents: [CP-65-Test-Steps](../../07-Coding-Plan/done/CP-65-Test-Steps.md), [CP-65-Multi-Candidate-Tournament-Harness](../../07-Coding-Plan/done/CP-65-Multi-Candidate-Tournament-Harness.md)
- Feature Keys: `tournament`, `review-loop`, `flow-engine`

## AI Quick View

### Summary

- CP65-2 (run-31): after the review loop hit cap 3 and escalated (`tournament_escalated` → `run-31-tournament`, parent parked `tournament_escalation`/`blockReason:cap`), a second `POST /agent-loop/continue` fired a **second** `tournament_escalated` event and spawned `run-31-tournament-2`; the parent `gateReason` was rewritten to name child-2 and the first child was orphaned.
- The anti-rerun guard (`loop.Status == LoopStatusTournamentEscalation` in `maybeEscalateCapToTournament`) never fires because `mutateLoop` resets status to `blocked` before the escalation check runs.

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.

## Bug report

- **Symptom:** one capped run produces multiple tournament children — `flow-diag-run-31.ndjson` shows two `tournament_escalated` events (`run-31-tournament`, then `run-31-tournament-2`) on the same parent; `gateReason` ends pointing at the newest child; the first child is left unreferenced.
- **Expected:** once a loop is in `tournament_escalation`, further `continue` calls are idempotent (no-op or re-attach to the existing child) — at most one escalation child per capped run.
- **Actual:** each `continue` re-runs the escalation path and mints another child run.
- **Impact:** duplicate escalation children multiply (each non-resumable per BUG-412 — pure orphans), operator-visible run graph loses track of which child is authoritative, and the overwritten gate reason misdescribes the park.

## Reproduction

1. `FLOWPILOT_ENABLE_TOURNAMENT_ESCALATION=1`; drive run-31's review loop to cap → `tournament_escalated` → `run-31-tournament` dispatched; parent `tournament_escalation`.
2. `POST /agent-loop/continue` again → second `tournament_escalated` → `run-31-tournament-2`; gateReason rewritten.

## Root cause

- `maybeEscalateCapToTournament` guards re-entry with `loop.Status == LoopStatusTournamentEscalation`, but the `continue` path calls `mutateLoop` which resets status to `blocked` **before** the escalation check evaluates — so the guard always observes `blocked` and re-fires. (Ordering defect in the continue → mutateLoop → escalate sequence, `interactive_service.go` continue/mutateLoop path.)

## Evidence

- `~/fp-beds/lt-evidence/cp65/flow-diag-run-31.ndjson` — two `tournament_escalated` events (`run-31-tournament`, `run-31-tournament-2`) on one parent.
- `~/fp-beds/lt-evidence/cp65/l65-2-parent-continue.json`, `l65-2-final-graph.json` — gateReason naming child-2; parent `tournament_escalation`.
- `~/fp-beds/lt-evidence/cp65/RESULT.md` (BUG-LIVE-CP65-2, L-65-2 timeline).

## Severity

`medium` — operator action duplicates escalation children and corrupts the gate reason; no data loss but the escalation ledger is unreliable.

## Completion Notes (implemented 2026-09-23, CA-923)

- Root cause: `applyFlowControl`'s `continue` case mutated a parent already
  in `tournament_escalation` (Round++ → blocked → `maybeEscalateCapToTournament`
  re-fired) — every repeated Continue spawned another tournament child.
- Fix: `continue` on a `tournament_escalation` parent returns the parked
  state unchanged; `maybeEscalateCapToTournament` additionally refuses when
  a tournament child already exists (defense in depth).
- Tests: `TestBug413_RepeatedContinuesKeepSingleChild`,
  `TestBug413_SecondEscalateBlockedByExistingChild` (red by assertion).
- Live: three consecutive Continue calls on the parked parent returned the
  same `tournament_escalation` state; exactly one tournament child existed.
