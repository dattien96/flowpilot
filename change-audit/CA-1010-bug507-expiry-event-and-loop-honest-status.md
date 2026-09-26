# CA-1010 — BUG-507: approval/question expiry emits a durable event; run stays honest while loop is open

## Context

Live run-22241 (vibe-ingest host, grok): an unattended
`session/request_permission` expired — the model's intended write never
ran and nothing at run level recorded the drop. The final turn then
ended without its verdict and the run still surfaced `status: completed`
while the mounted vibe-owner-debate loop stayed `running` — a dishonest
terminal that also swallowed the loop's `blocked` verdict.

## Changes

- `provider_event.go`: new event types `approval_expired` and
  `question_expired` (carry `ApprovalID`+`Details` / `QuestionID`+`Prompt`).
- `expireApproval` / `expireQuestion` (`interactive_service.go`): emit the
  typed event through `emitLocked` on expiry, so the dropped effect is
  persisted to the run log and broadcast — never a silent vanish.
- `emitLocked` `EventTurnCompleted` root path: when a no-code turn ends
  and the run's mounted loop is still open (status not in
  {"", done, stopped}), the run stays `running` instead of marking
  `completed`. Sealed/absent loops keep the original completion shape;
  `settleParentRunOnFlowDone` still publishes the honest terminal when
  the loop actually seals.

## Tests

`bug507_approval_expiry_test.go` (new, additive): approval expiry emits
the durable event with the card id; question expiry likewise; a no-code
turn on an open-loop run does not mark `completed`; a sealed loop still
completes normally.

## Verification

`go test -count=1 -run TestBug507 ./internal/runner/` — 4/4 green.
