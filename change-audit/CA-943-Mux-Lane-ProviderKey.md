# CA-943 — BUG-381: providerKey added to mux lane projection

## Summary

`RunRealtimeProjection` had no `providerKey`, so mux-only lanes (no polled
history row yet) rendered in the inbox/board with a hardcoded "claude"
fallback — wrong provider label for codex/grok lanes carrying no pending
decision.

Fix: runner stamps `providerKey` (omitempty) on the projection; desktop
`laneHistoryRow` prefers it over the decision-derived guess. Old runners
without the field degrade to the previous fallback — additive, no wire
break.

## Verified

- `runUpdates.test.ts` 7/7 green; runner `TestRunUpdates|TestDecisionPayload`
  suite green.

## Files

- `internal/runner/decision_payload.go`,
  `apps/desktop-flowpilot/src/types/contract.ts`,
  `apps/desktop-flowpilot/src/state/attentionQueue.ts`,
  `requirements/09-BugFix/todo/BUG-381-…md` (new)

# ---8<--- flowpilot:change-ledger
feature_key: event-plane
source_doc_id: BUG-381
change_type: bugfix
