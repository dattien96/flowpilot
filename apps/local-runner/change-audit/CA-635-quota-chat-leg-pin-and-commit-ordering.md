# CA-635 — Quota Chat-Leg Pin Validation + Commit Ordering (BUG-541 residual)

**Date**: 2026-09-28
**Author**: Devin
**Ticket**: BUG-541 (residual)

---

## Problem

Live `run-19946` / card `q-19948`: the operator typed an invented route
option `use_for_run|codex|fakeacct|gpt-5.4`. `ResumeQuotaGate` parsed the
option without validating the account; `commitQuotaRotation` emitted
`quota_route_committed` BEFORE the switch; `switchChatLeg` closed the
source grok leg while the destination could never provision (lazy
session provisioning on the first turn). Result: run `running` +
`leg_state=closed` + every turn `session_unavailable` + no re-gate — an
unrecoverable zombie. Chat-leg variant of BUG-534's close-before-spawn.

## Fix

### `internal/runner/quota_gate.go`

- `pinnedAccountResolvable` — strict pin check (same contract as
  `resolveAdapterAccount`): an operator-supplied route account must exist
  and be connected before any mutation. `""`/`"default"` keep legacy
  active-account resolution.
- `ResumeQuotaGate` validates the pin up front — invented accounts return
  an apply error so `AnswerQuestion` rolls the card back to pending.
- `commitQuotaRotation` chat branch emits `quota_route_committed` only
  after `switchChatLeg` returns success — matching the child-respawn
  path's spawn-first ordering.

### `internal/runner/chat_switch.go`

- Phase A: `ProviderAccountID` pins resolve before the durable intent is
  stamped — an unprovisionable binding never mints a leg.
- Phase B: a pinned switch whose seed turn fails provisioning-class
  (`session_unavailable`, `account_unavailable`, `account_not_signed_in`,
  `provider_unavailable`, `leg_closed` — `isProvisioningSeedError`)
  aborts: destination leg closes terminal `dispatch_failed`, source leg
  stays open, in-flight guard released. Content-level seed failures stay
  non-fatal per SD26-X-7 (TOCTOU cover for pins that die between Phase A
  and dispatch).

## Tests

`bug541_chat_leg_commit_test.go` — invented account rejected with card
kept pending and no committed record; valid listed route commits after
the destination leg exists; unresolvable pinned switch rejected
pre-mutation; provisioning-vs-content seed error classification pinned.
