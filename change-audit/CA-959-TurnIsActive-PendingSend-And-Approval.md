# CA-959 — turnIsActive regression: done-loop pending send + approval-pending [stop]

## Summary

Two pre-existing test failures in `internal/tui/app` traced to a real
regression in `AppModel.turnIsActive` introduced by `8d3162d4` (BUG-371,
"hide composer stop when loop done"):

1. **Done-loop pending send.** BUG-341 (`8840f1f9`) split `turnSendPending`
   out of `pendingPrompt` for the send→stream gap, but the BUG-371 done-guard
   (`flowLoopStatus==done && !hasLiveWorkingChild() → false`) ran before the
   pending-prompt check and did not know about `turnSendPending`. A post-done
   follow-up in the send gap (CA-544 contract) was treated as not-live —
   `[stop]` disappeared mid-send and `TestPostDoneFollowUp_StepsPollInSendGapDoesNotSettle`
   failed for all three providers.

2. **Approval-pending.** The same commit added `m.approval != nil → false` to
   the park-guard, conflating "turn awaiting approval" with "parked". The
   underlying turn is still live — the operator must be able to cancel
   instead of answering. `TestApprovalBarAndStopAreClickable` failed.

## Fix

In `turnIsActive`:

- Done-guard now exempts `m.pendingPrompt != "" || m.turnSendPending` —
  a pending send is live work even across a done loop boundary.
- `m.approval != nil` removed from the question/gate park-guard (question
  and gate remain parked-states; approval keeps [stop] armed).
- `turnSendPending` folded into the existing `pendingPrompt` early-true.

All parked/blocked semantics are unchanged: `hasUnresolvedAttention`,
`flowLoopBlocked`, question/gate, and BUG-371's done-no-child guard still
disarm `[stop]` — verified by the full BUG-371/bug328/run136749/CA-622
suite staying green.

## Repro

Both failures were deterministic pre-existing reds:

- `TestPostDoneFollowUp_StepsPollInSendGapDoesNotSettle` (claude/codex/grok)
- `TestApprovalBarAndStopAreClickable`

## Files

- `apps/local-runner/internal/tui/app/app.go` — `turnIsActive` guard order
  and approval carve-out (comments updated; no signature change)

## Verified

- `go test ./internal/tui/...` — all packages pass, including every
  [stop]-arm/disarm invariant (BUG-371, BUG-231, BUG-327/328, run136749,
  run97624, CA-622, attention banner).

## Provider parity

Provider-agnostic TUI model logic; the CA-544 test exercises all three
providers via subtests — all green.
