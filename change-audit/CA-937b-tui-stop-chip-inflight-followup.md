---
id: CA-937b
title: TUI — composer [stop] stays armed for post-done follow-up turns; stale approval-bar test aligned to CA-826 (BUG-454)
type: BugFix
feature: cli-tui
date: 2026-09-23
status: done
---

## Context

- `TestPostDoneFollowUp_StepsPollInSendGapDoesNotSettle` (CA-544, red on
  baseline): after a flow reports `done`, sending a follow-up chat turn sets
  `turnSendPending`/`ConnRunning`, but `turnIsActive` returned false because
  the BUG-371 guard (`done && !hasLiveWorkingChild`) short-circuited before
  the in-flight-turn checks — so composer `[stop]` was disarmed while a real
  turn was still being sent. A steps poll landing in the send gap could
  settle the finished loop and leave the in-flight turn unstoppable.
- `TestApprovalBarAndStopAreClickable` (red on baseline): written before
  BUG-371/CA-826 (`8d3162d4`), which intentionally hides composer `[stop]`
  while a question/approval/gate card is mounted ("ask_user is not a
  stoppable turn"). The old assertion expected `[stop]` clickable next to
  Approve/Deny — stale against the shipped contract.

## Change

- `tui/app/app.go` (`turnIsActive`): the BUG-371 done-loop guard now exempts
  an in-flight turn — `!m.turnSendPending && m.turnStream == nil` must both
  hold for the early `false`. A done loop with a live child still arms
  `[stop]`; a done loop with a follow-up turn in flight now arms it too.
- `chat_ux_actions_test.go`: `TestApprovalBarAndStopAreClickable` keeps its
  approve/deny click-target assertions and now positively asserts `[stop]`
  is *not* clickable while an approval is mounted — matching CA-826 instead
  of contradicting it.

## Tests

- `TestPostDoneFollowUp_StepsPollInSendGapDoesNotSettle` (claude/codex/grok)
  — red→green.
- `TestApprovalBarAndStopAreClickable`, `TestBUG371_*`,
  `TestFlowDone_NotSettledWhileChildRunning` — green.

## Result

- [stop] is armed exactly when live work exists: in-flight send, open
  stream, or live child — and stays hidden for settled done loops and
  parked approval/question states.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: BUG-454
change_type: bugfix
summary: turnIsActive exempts in-flight post-done follow-up turns (turnSendPending/open stream) from the done-loop stop-chip guard; stale approval-bar test asserts the CA-826 contract
# --->8---
