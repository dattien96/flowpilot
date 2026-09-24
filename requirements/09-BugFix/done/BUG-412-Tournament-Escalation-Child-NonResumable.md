# BUG-412: Tournament escalation children are non-resumable — `providerSessionId:""` → every turn 409 `session_unavailable`

## Metadata

- Document ID: `BUG-412`
- Title: `Tournament escalation child spawned without provider session → any turn rejected 409 session_unavailable — escalated tournament can never execute`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-23`
- Parent Documents: [CP-65-Test-Steps](../../07-Coding-Plan/done/CP-65-Test-Steps.md), [CP-65-Multi-Candidate-Tournament-Harness](../../07-Coding-Plan/done/CP-65-Multi-Candidate-Tournament-Harness.md)
- Feature Keys: `tournament`, `review-loop`, `flow-engine`

## AI Quick View

### Summary

- CP65-1 (run-31-tournament, run-31-tournament-2): with `FLOWPILOT_ENABLE_TOURNAMENT_ESCALATION=1`, driving a review loop to cap spawns an escalation child — but the child is created with `providerSessionId:""` and every `POST /turns` returns `409 {"code":"session_unavailable","message":"devin session has no real session id to resume"}`.
- Reproduced identically on both tournament children; real (non-escalation) children on the same runner have working sessions (run-9 `equable-snowshoe`, run-17 `ses_…`). The escalated tournament can never execute a single turn.

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.

## Bug report

- **Symptom:** `POST /client/workflow-runs/run-31-tournament/turns` → `409 session_unavailable`; `GET /client/workflow-runs/run-31-tournament` shows `providerSessionId:""`. Identical on the second escalation child `run-31-tournament-2`.
- **Expected:** the escalation spawn provisions a real provider session (like cohort/child spawns do) so the tournament run can take turns.
- **Actual:** escalation children are minted with no provider session id; the resume gate rejects every turn — the tournament escalation path is dead on arrival.
- **Impact:** cap-escalation (`tournament_escalated` event fires, parent parks `tournament_escalation`) produces a child that can never act — no arbiter verdict, no retry/ask traversal, no merge. Combined with BUG-413 the parent can spawn multiple dead children.

## Reproduction

1. `runner serve` with `FLOWPILOT_ENABLE_TOURNAMENT_ESCALATION=1`.
2. Drive a review loop to cap (or `agent-loop/continue` a `blockReason:cap` run) → `tournament_escalated` → child `run-31-tournament` dispatched.
3. `POST /client/workflow-runs/run-31-tournament/turns` → `409 session_unavailable`; `GET` the child → `providerSessionId:""`.

## Root cause

- Escalation spawn path never provisions a provider session: the child record is created with `providerSessionId:""`, then `ensureResumeReady` (`apps/local-runner/internal/runner/interactive_resume.go:2725+`, devin branch ~L2765) requires a real provider session id to resume → 409 `session_unavailable`. Contrast: normal child spawns receive sessions (`equable-snowshoe`, `ses_…`).

## Evidence

- `~/fp-beds/lt-evidence/cp65/l65-2-child-turn.json` — the 409 response body.
- `~/fp-beds/lt-evidence/cp65/flow-diag-run-31.ndjson` — `tournament_escalated` events for both children.
- `~/fp-beds/lt-evidence/cp65/l65-2-parent-run.json`, `l65-2-final-graph.json` — parent parked `tournament_escalation`.
- `~/fp-beds/lt-evidence/cp65/l65-1-agents.json` — real children carry sessions.
- `~/fp-beds/lt-evidence/cp65/RESULT.md` (BUG-LIVE-CP65-1).

## Severity

`medium` — the whole tournament-escalation feature is unreachable live; deterministic 409 on the first turn of every escalation child.

## Completion Notes (implemented 2026-09-23, CA-923b)

- Root cause: `escalateToTournament` hand-built the child `interactiveRun`
  and skipped everything `createRun` provisions — no `providerSessionID`,
  no `providerAccountID`, no `flowRef` — so `startTurn`/`ensureResumeReady`
  rejected every turn (`session_unavailable` / `provider_account_changed`)
  and the child could never be resumed or take its first turn.
- Fix: the child now inherits `providerKey` + resolves `providerAccountID`
  before the lock, mints a real `thread-*` provider session id, carries the
  tournament flow reference, and its first turn is kicked at spawn.
- Tests: `TestBug412_TournamentChildGetsProviderIdentity`,
  `TestBug412_TournamentChildAcceptsFirstTurn`,
  `TestBug412_TournamentChildCarriesFlowRef` (red by assertion before the fix).
- Live: `/tmp/fp-live-h` — cap escalation spawned `run-603-tournament`
  (devin, `thread-627`, `devin/swe-2-high`) which ran to `completed`.
