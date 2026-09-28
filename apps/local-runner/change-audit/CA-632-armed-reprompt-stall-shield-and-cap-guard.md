# CA-632 — Armed Reprompt Stall Shield & Cap Guard (BUG-539)

**Date**: 2026-09-28
**Author**: Devin
**Ticket**: BUG-539

---

## Problem

Live run-2830/run-1663: a flow member with an armed `pendingGateReprompt*`
intent was counted silent by `checkAndBlockStalledMembers` — the intent
could not dispatch because the loop was blocked. The resulting park wiped
the reprompt (deliberate BUG-354 freeze) leaving `pendingGateCodePaths`;
the orphan-cure then minted a generic Resume turn which reset
`repromptAttempts` — `maxFlowGateReprompts` unreachable, unbounded gate
loop. Skip/retry also left armed intents + active leg claims behind.

## Fix

- `internal/runner/cohort_stall.go` — member-stall detector treats armed
  `pendingGateReprompt*`/`pendingResume*` as queued work (mirrors the
  hub_stall.go shield); `skip` clears reprompt/resume/pending-turn intents
  and closes the active leg with `LegClosedReasonMemberSkipped`; `retry`
  clears the same stale intents before dispatching.
- `internal/runner/interactive_service.go` — orphan-cure routes
  armed-reprompt children through `resumeIDs`/`flushDurableTurnIntents`
  (reprompt channel, counter preserved) instead of the Resume mint;
  `resumeIDs` now also selects armed-intent children regardless of parked
  status; `startTurn` resets `repromptAttempts` only when no
  `pendingGateCodePaths` debt remains, so even a cure-minted remediation
  turn keeps the cap reachable.
- `internal/runner/chat_ssot.go` — `LegClosedReasonMemberSkipped`.

The park wipe and blocked-loop stale-clear are unchanged — BUG-354's
"parked flow holds no live auto-intents" contract stands;
`pendingGateCodePaths` is the surviving re-check seed. BUG-520/BUG-327/
run-63960 tests pass unchanged.

## Tests

`bug539_member_stall_reprompt_strand_test.go` (5 tests) — all green.
