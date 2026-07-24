# CA-403 - run-45103: cap continue must not self-cancel submitting hub turn

## Symptom

Live Review Loop `run-45103` (Codex): after 3 rounds, synthesis called
`continue` at the round cap. Loop correctly blocked with
`blockReason=cap` / `gateReason=cap 3 reached with 1 open issue(s)` and
synthesis settled `WAITING_USER_APPROVAL`, but the same hub turn was cancelled
via `parkFlowForAwaitingUser` → `context canceled` / `terminal_cancelled` /
"interrupted by user". Desktop looked hung on **Thinking...** despite an
awaiting-user state.

Claude/Grok testing often never hit this path (models escalate/done earlier);
the path is provider-agnostic.

## Cause

`applyFlowControl("continue")` at cap calls `parkFlowForAwaitingUser`, which
unconditionally cancelled `parent.turnCancel` while the synthesis turn that
just submitted `submit_review_outcome` was still in flight.

## Fix

- `parkFlowForAwaitingUser` accepts optional `preserveParentTurnID`.
- Cap path of `continue` preserves the current hub turn when it matches
  `lastFlowControlTurnID` (same-turn decision stamp).
- Children and auto-intents still freeze; escalate / hub_stall keep default
  cancel-all.

Provider-agnostic: no `providerKey` branch; shared `SubmitFlowControl` entry.

## Tests (additive only)

`run45103_cap_park_self_cancel_test.go`:
- submitting hub turn not cancelled at cap
- children still cancelled; default park still cancels parent
- mismatched preserve id does not skip cancel

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-231
change_type: bugfix
summary: Cap continue park preserves submitting hub turn so desktop is not stuck Thinking (run-45103)
# --->8---
