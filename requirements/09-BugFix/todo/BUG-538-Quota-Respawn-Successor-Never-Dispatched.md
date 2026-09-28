# BUG-538 — Quota-route successor leg spawned but never dispatched; parent wedges

Status: **FIXED** (CA-1045; regression-tested + live-verified)
Severity: Important — wedges a tournament parent run and strands an `active` leg claim forever.

## Symptom (live, `/tmp/fp-live4`, run-551)

1. Tournament leg `run-945` (candidate-a, grok) vetoed on `credits_exhausted`
   → quota card `q-1441`, step `candidate-a` → `WAITING_USER_APPROVAL`.
2. Operator answered `use_for_run|devin|…` while parent `run-551` was `running`.
3. BUG-534 spawn-first ordering worked: successor `run-1651` (devin, same
   cohort `attempt-0`, same worktree `candidate-candidate-a`) created at
   14:43:39.188; `run-945` leg closed `provider_switch` at .189;
   `quota_route_committed` emitted.
4. At the same instant the step `candidate-a` → **FAILED** — it consumed the
   OLD leg's failure outcome.
5. `run-1651` received its handoff (`bus-1653`, queued=false) but **no turn was
   ever dispatched**: `status: idle`, `agent_status: spawned`, no run dir, no
   turn-params line, `leg_state: active`.
6. Parent flow ran attempt-1 (`problem_scout` DONE 14:51:10, rest SKIPPED,
   `loop_state.status: done`) — yet `run-551.status` stays `running` because a
   child leg is still `active` (BUG-521-withheld completion is firing correctly
   on a stranded leg). Nobody ever dispatches or closes `run-1651`.
7. Post-restart residue: `run-1651` normalized to `failed` but kept
   `leg_state: active` — a durable claim on `candidate-candidate-a` that no
   in-memory sweep could ever reach (the run is never reloaded).

## Root cause (confirmed)

`spawnChildRun` checks the parent loop is runnable, then dispatches the child's
first turn on an async goroutine (`go s.startTurn`). Between the guard and the
dispatch, the parent loop re-blocked (the quota veto's escalate landed in the
same window). `startTurn` returned `flow_awaiting_user` —
"parent flow is waiting for your decision; resolve the form before a child
turn" — which `handleChildStartTurnFailure` treated like any start failure:
child → `failed`, step → FAILED, cohort result appended. The leg stayed
`leg_state=active` and the worktree claim durable → stranded successor, exactly
the "spawned but never dispatched" wedge.

## Fix (CA-1045)

`internal/runner/interactive_service.go`:

- New `handleSpawnedChildTurnFailure` on the spawned-child async dispatch path:
  `flow_awaiting_user` (a retryable refusal — the parent re-blocked mid-race)
  now **parks** the child instead of failing it: durable intent armed
  (`pendingResumePrompt`/`pendingResumeStepID`/`pendingResumeGen`++), honest
  `waiting_user_approval` status, session persisted immediately, leg stays
  `active`. Non-retryable failures flow to `handleChildStartTurnFailure`.
- `handleChildStartTurnFailure` now closes the failed leg
  (`LegClosedReasonDispatchFailed`) and persists the failed snapshot — a
  hard-failed child can no longer strand an `active` claim.
- `resumePendingLoopWork` and `resumeFlowWithFeedback` scan children for armed
  `pendingResumePrompt` and flush via `flushDurableTurnIntents` (the durable
  claim/generation/idempotency path) on every unblock entry — including
  `agent-loop/continue`.
- `parkFlowForAwaitingUser` (both variants) preserves a **never-dispatched**
  child's `pendingResume*` (turnCount==0 — that intent IS the first turn, not
  a stale continuation), so a re-park cannot re-strand it.
- `reconcileChildRunsOnFlowDone` now closes settled children's legs
  (`LegClosedReasonFlowDone`), clears their resume intents, and persists.

`internal/runner/tournament_dispatch.go`:

- `closeLegsBoundToWorktree` additionally sweeps the durable session index:
  an `active`-leg row on the swept dir belonging to a run not resident in
  memory is dead residue (the run-1651 case — failed run, never reloaded) and
  is closed with `worktree_swept`. Live in-memory claimants are untouched.

`internal/runner/chat_ssot.go`: new leg-closure reasons
`dispatch_failed`, `flow_done`.

`flow_awaiting_user` stays classified retryable by `isPermanentStartTurnError`
— a re-flush on a still-blocked parent keeps the intent, not a burn.

## Tests (all RED→GREEN)

`bug538_spawned_successor_stranded_test.go`:

- `TestBug538_SpawnedChildBlockedDispatchParksDurableIntent`: forced
  `flow_awaiting_user` on a spawned child's first turn → child not failed,
  leg stays active, durable intent persisted, `pendingResumeGen != 0`; unblock
  → real turn dispatched exactly once (`turnCount==1`).
- `TestBug538_ParkedSuccessorFlushedOnFlowResume`: `resumeFlowWithFeedback`
  (the `agent-loop/continue` path) flushes the parked child's intent.
- `TestBug538_ParkedSuccessorLegClosedOnFlowDone`: flow-done settle → child
  completed, leg closed `flow_done`, intent cleared.
- `TestBug538_SpawnedChildHardFailureClosesLeg`: non-retryable failure →
  child failed, leg closed `dispatch_failed`, no active claim.

`review_followup_tournament_test.go`:

- `TestBug538_WorktreeSweepClosesDurableOnlyLegClaim`: a `failed`+`active`-leg
  session row with NO resident run (run-1651 residue) is closed
  `worktree_swept` by the sweep; spawn proceeds on the reclaimed dir.

## Live verification (`/tmp/fp-live4`, fixed binary)

- Pre-existing residue `run-1651` (`failed`, `leg: active` on
  `candidate-candidate-a`): a fresh tournament's candidate spawn swept the
  dir; the durable row now reads `leg: closed, reason: worktree_swept` —
  closed through the real spawn→sweep entry.
- `run-3240` tournament parked honestly on the contested dir while a live
  leg (run-1663's `run-2830`) claimed it — BUG-535 live-claimant guard.
- Binding with grok ledger-blocked produced devin×2 candidates (BUG-536
  re-verified live).

## Residual (cosmetic)

`run-945` keeps `status: waiting_user_approval` + `pending_resume_*` fields
after its leg closed `provider_switch` — the run-level status wasn't synced to
the leg terminalization.

## R14 live leg (2026-09-28, `/tmp/fp-live5`, binary w/ CA-636)

- `run-26036` tournament: candidate-a `run-27175` quota-parked; answering
  `q-27188` while the parent loop was `blocked(escalate)` returned typed
  `quota_route_apply_failed` and the card stayed **pending** — the
  parked-successor path fails closed instead of dispatching into a blocked
  loop.
- After unblocking, the same card resolved and successor `run-28790`
  spawned (devin, `candidate-a`, worktree preserved). A subsequent
  candidate-b escalate park-cancelled the successor's in-flight turn —
  durable `pending_resume_*` residue is the pinned contract (BUG-543).
