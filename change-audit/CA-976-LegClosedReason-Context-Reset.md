# CA-976 — legClosedReason=context_reset for same-binding rotations

## Summary

Live Devin verification of rotate_leg showed the closed leg recorded
`legClosedReason: provider_switch` — a routing label for what was a context
reset (same provider+account+model, new session). The ledger now records
`context_reset`.

## What changed

- `chat_ssot.go` — new `LegClosedReasonContextReset = "context_reset"`.
- `chat_switch.go` — `switchChatLeg` closes the source leg with
  `context_reset` when invoked on the same-binding path
  (`allowSameProvider=true`, currently only `rotateChatLegForContext`).
- `healChatLegsLocked` — the closed-source heal window matches both
  `provider_switch` and `context_reset` so record-append healing still works
  after a reset.

## Tests

- New `TestTask443_ContextReset_CloseReasonIsContextReset` — red before the
  fix (`provider_switch`, want `context_reset`), green after.
- Full Task-443 + chat-switch suite green.

# ---8<--- flowpilot:change-ledger
feature_key: token-usage
source_doc_id: Task-443
change_type: bugfix
summary: same-binding leg resets close with legClosedReason=context_reset instead of provider_switch; heal window matches both reasons
# --->8---
