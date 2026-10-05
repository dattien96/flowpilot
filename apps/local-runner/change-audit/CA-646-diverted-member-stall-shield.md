# CA-646 — Debate-Diverted Cohort Member Shielded From Stall Sweep (BUG-1195)

**Date**: 2026-10-05
**Author**: Devin
**Ticket**: BUG-1195

---

## Problem

Live `run-225691`: `owner_1` leg `run-258141` finished a turn while the
owner debate overlay was already mounted. Its post-turn gate diverted into
`startVibeOwnerDebate`, the mount was suppressed (`debate already active`),
and `stashVibeFlowForDebate` recorded the child in
`vibeParkedGatedRunIDs` — the durable "owes a post-debate reprompt" list.
Its `pendingFlowGateSettle` was consumed by the divert
(`settleChildStatusAfterGateBlockLocked` left it `Running` because the
loop was not blocked), so the member sat silent by design: no provider
events, no armed intent, no user-visible card, seat held open.

`checkAndBlockStalledMembers` counted exactly that shape as a silent
member and parked the hub `member_stalled` ~10 min later — while the
debate it was waiting on was still live. The Continue respawn then lost
the cohort binding (BUG-1194) and the wedge looped. A diverted member is
queued remediation — the same contract as the BUG-539 armed-intent
shield — not silence: its re-drive is owned by
`restoreVibeFlowAfterDebate`, and if the debate itself wedges, the
debate's own members surface through their own sweep.

## Fix

Both stall paths now skip members recorded in the parent's
`vibeParkedGatedRunIDs`:

- `maybeScheduleStallCheck` clones the list under `s.mu` and excludes
  diverted members from the earliest-deadline computation, so a held
  member can no longer shorten the timer for genuinely quiet siblings
  either.
- `checkAndBlockStalledMembers` clones the same list and `continue`s past
  diverted members before the gate/intent checks.

The list's lifecycle already bounds the shield: entries are added at the
divert (`stashVibeFlowForDebate`), cleared at
`restoreVibeFlowAfterDebate` and `dropVibeDebateStaleClaimLocked`, and
persisted/restored with the run session — so a member stops being
shielded the moment its divert is resolved or its claim is dropped.

## Files

- `internal/runner/cohort_stall.go` — `slices` import; diverted-member
  skip in `maybeScheduleStallCheck` + `checkAndBlockStalledMembers`.
- `internal/runner/bug1195_diverted_member_stall_shield_test.go` —
  red→green tests.

## Test evidence

- `TestBug1195_DivertedMemberDoesNotStall` — red before the fix
  (`member_stalled` fired on the diverted member), green after: no park.
- `TestBug1195_NonDivertedSilentMemberStillStalls` — a silent member not
  in the gated list still trips `member_stalled`: the shield does not
  swallow real stalls.

Provider parity: provider-agnostic — sweep bookkeeping only.
