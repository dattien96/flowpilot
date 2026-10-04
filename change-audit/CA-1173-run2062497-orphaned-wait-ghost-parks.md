# CA-1173 — run-2062497: orphaned waiting states and cardless question/approval parks

## What changed

`apps/local-runner/internal/runner/interactive_service.go`:

- `clearPendingQuestion` and `clearPendingApproval` now perform the
  BUG-289 status flip (`waiting_question`/`waiting_permission` → `running`)
  that `expireQuestion`/`expireApproval` already had, emit the resolved
  event, and persist a session snapshot. Live: cancelling an `ask_user`
  turn cleared the card but left the parent parked `waiting_question` with
  no live card for ~87s until the wedge sweep healed it.
- `EventUserQuestionRequired` / `EventPermissionRequired` handlers gate the
  park on a live pending record: a ghost event carrying no `QuestionID` /
  approval id (or one already resolved) no longer stamps a waiting status.
  All emit sites create+persist the record before emitting, so the gate
  admits every real park; replay does not pass through `emitLocked`, so
  resume is unaffected.

## Invariant

A waiting status always has a live card behind it — a cleared card clears
the wait, and an event without a record cannot mint a cardless park.

## Tests

`bug289_test.go` —
`TestRun2062497_ClearPendingQuestionClearsWaitingStatus`,
`TestRun2062497_ClearPendingApprovalClearsWaitingStatus`,
`TestRun2062497_QuestionRequiredWithoutRecordDoesNotPark`,
`TestRun2062497_PermissionRequiredWithoutRecordDoesNotPark`.
