# BUG-381: Mux lane projection lacks providerKey — inbox renders wrong provider for decision-free lanes

## Metadata

- Document ID: `BUG-381`
- Title: `RunRealtimeProjection carries no providerKey; laneHistoryRow defaults to "claude" for mux-only lanes`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-09-23`
- Last Updated: `2026-09-23`
- Feature Keys: `event-plane`
- Parent Documents: CP-84, Task-429
- Child Documents: `none`
- Related Documents: BUG-379, BUG-380 (same review pass)
- Replaces: `none`
- Tags: `mux, inbox, provider-label`

## AI Quick View

### Summary

- `RunRealtimeProjection` never carried `providerKey`; `laneHistoryRow`
  (attentionQueue.ts) fell back to `decisions[0].providerKey ?? "claude"`.
- A mux-surfaced lane with no pending decision — e.g. a codex/grok run
  waiting on nothing — rendered in the inbox/board labeled "claude".
- Fix: runner stamps `ProviderKey` on the projection; desktop prefers it.

### Constraints

- Additive field — old runners omit it, desktop falls back (unchanged).

## 5. Root Cause

Field simply absent from the projection contract; the fallback guessed from
decisions, which are absent exactly when the lane has nothing pending.

## 6. Fix

- `RunRealtimeProjection.ProviderKey` (`omitempty`) populated from
  `rs.providerKey`.
- `laneHistoryRow` prefers `p.providerKey`, then decision provider, then
  "claude".

## 7. Regression

- `runUpdates.test.ts` 7/7 green; runner mux tests green.
