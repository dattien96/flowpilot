# CA-1045 — Spawned successor parks on re-blocked parent; leg claims close on failure/flow-done/sweep (BUG-538)

## Summary

Live run-1651: a quota-route successor was spawned, the parent loop
re-blocked before its async first `startTurn`, and the refusal
(`flow_awaiting_user`) was handled like a hard start failure — child
`failed`, step FAILED, `leg_state: active` left holding the
`candidate-candidate-a` worktree claim forever. Nothing re-drove it; a
restart normalized the run to `failed` but the durable claim stayed
`active` because the sweeps only scanned resident memory.

## Changes

- `internal/runner/interactive_service.go`
  - `handleSpawnedChildTurnFailure` (new): on `flow_awaiting_user` park the
    child — arm the durable resume intent (`pendingResumePrompt`,
    `pendingResumeStepID`, `pendingResumeGen++`), set
    `waiting_user_approval`, persist, keep the leg `active`. Other errors
    delegate to `handleChildStartTurnFailure`.
  - `handleChildStartTurnFailure`: close the failed leg with
    `LegClosedReasonDispatchFailed` and persist the failed snapshot.
  - `resumePendingLoopWork` + `resumeFlowWithFeedback`: flush armed child
    `pendingResumePrompt` intents via `flushDurableTurnIntents` on unblock
    (the `agent-loop/continue` entry included).
  - `parkFlowForAwaitingUser` (locked + unlocked variants): keep
    `pendingResume*` on children with `turnCount==0` — that intent is the
    pending first turn; wiping it re-stranded the successor.
- `internal/runner/flow_step_runtime.go` (`reconcileChildRunsOnFlowDone`):
  settled children now close `active` legs (`LegClosedReasonFlowDone`),
  drop armed intents, and persist — flow done cannot leave live claims.
- `internal/runner/tournament_dispatch.go` (`closeLegsBoundToWorktree`):
  after the in-memory sweep, scan `SessionIndexReader` for `active`-leg
  rows on the dir whose runs are not resident — dead-run residue like
  run-1651's — and close them `worktree_swept`.
- `internal/runner/chat_ssot.go`: `dispatch_failed`, `flow_done` reasons.

## Tests (RED→GREEN)

- `TestBug538_SpawnedChildBlockedDispatchParksDurableIntent` — compile-red
  (handler missing); post-fix: park fields armed, leg active, unblock
  dispatches exactly once.
- `TestBug538_ParkedSuccessorFlushedOnFlowResume` — continue-path flush.
- `TestBug538_ParkedSuccessorLegClosedOnFlowDone` — settle closes leg,
  clears intent.
- `TestBug538_SpawnedChildHardFailureClosesLeg` — hard failure closes
  `dispatch_failed`.
- `TestBug538_WorktreeSweepClosesDurableOnlyLegClaim` — verified red
  (`state="active"`) before the index sweep.

## Verification

- Focused batch `TestBug538|TestBug535`: PASS; `-race` clean; `go vet` +
  `go build ./...` clean.
- Live `/tmp/fp-live4` (fixed binary): run-1651's durable `active` claim
  closed `worktree_swept` via the real tournament spawn→sweep path; fresh
  legs claimed both worktrees; contested-dir park + ledger-blocked
  binding-skip re-verified live.
- Full `internal/runner` suite: only documented env baseline failures
  (missing provider binaries, MCP network, TempDir flake) — none in the
  touched seams.
