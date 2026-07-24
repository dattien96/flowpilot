# CA-413: blocked cap park clears stale Thinking (run-63960)

## Symptom

Live Review Loop `run-63960` (Codex hub): after 3 rounds with open issues,
engine correctly parked at cap (`loopState.status=blocked`,
`blockReason=cap`, synthesis `WAITING_USER_APPROVAL`), and dispatch recorded
`turn-69560` as `terminal_completed`. Desktop still showed perpetual
**Thinking...** / Running after the user saw the park (and typed `ok`).

## Cause

`applyOrchestrationEvent` only called `settleCompletedFlowTimeline` when
`loopState.status === "done"`. Blocked (awaiting-user) graphs left thinking
rows and running `submit_review_outcome` tool rows in the timeline.

Engine path (cap park, CA-403 preserve submitting turn) was correct — this is
primarily a **UI reconciliation** bug, not a provider deadlock.

## Fix

- `apps/desktop-flowpilot/src/state/store.ts` `applyOrchestrationEvent`: also
  settle timeline residue when `loopState.status === "blocked"` (same
  `settleCompletedFlowTimeline` helper as done).
- Status derivation (`deriveOrchestrationRunStatus`) **unchanged** so the
  existing BUG-231 suite contract remains: an actively-running child still
  wins over a blocked loop for derived run status.

Provider-agnostic shared UI path (no `providerKey` branch).

## Tests (additive only)

New file `store.flow-blocked-terminal.test.ts`:

- blocked cap × codex/claude/grok → status blocked + Thinking cleared
- blocked escalate shape
- running loop keeps Thinking (non-regression)
- done still settles (non-regression vs `store.flow-terminal.test.ts`)
- legacy contract: running child over blocked still derives running

Old suites: `store.flow-terminal.test.ts`, `store.post-stop-status.test.ts`,
BUG-231 tests in `store.test.ts` — run green, **not edited**.

## Prior CA claims preserved

- CA-403: cap park does not cancel submitting hub turn
- CA-375: gate settlement for chat hub turns
- BUG-231: blocked is actionable pause (composer unlock when status=blocked)

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-231
change_type: bugfix
summary: Clear stale Thinking when loopState is blocked at cap/escalate (run-63960) without treating the flow as completed
# --->8---
