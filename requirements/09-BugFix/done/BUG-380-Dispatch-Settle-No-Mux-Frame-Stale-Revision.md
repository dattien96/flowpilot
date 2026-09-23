# BUG-380: Dispatch settle emits no mux frame; wire revision can fail to advance → stale inbox items

## Metadata

- Document ID: `BUG-380`
- Title: `dispatch operator mutations don't dirty the lane, and mux wire revision (=rs.seq) doesn't advance on decision-only changes — settled attention lingers`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-09-23`
- Last Updated: `2026-09-23`
- Feature Keys: `event-plane`
- Parent Documents: CP-84, Task-429 (mux), CP-51 dispatch operator endpoints
- Child Documents: `none`
- Related Documents: BUG-379 (per-subscriber fingerprint)
- Replaces: `none`
- Tags: `mux, dispatch, settle, stale-attention, revision`

## AI Quick View

### Summary

Two stacked defects made settled `dispatch_attention` items linger in the
desktop inbox:

1. `handleDispatchResolve`, `handleDispatchRetryAsNew`, and
   `handleDispatchRepairResolution` mutated the durable store but never
   called `markRunRealtimeDirty` — subscribers got no frame at all until an
   unrelated event dirtied the run. (Earlier live verification used a fresh
   snapshot, which reconciles — masking the missing mark.)
2. Wire `revision` was `rs.seq` (event sequence). A dispatch-only change
   leaves `rs.seq` untouched, so even a marked upsert carries an unchanged
   revision — the desktop dedupe (`rev <= last applied`) drops it.

### Constraints

- safe-fix-contract: additive regression tests; no existing test touched.
- Lock order unchanged: `markRunRealtimeDirty` takes `s.mu` itself; handlers
  hold no locks at call sites.

## 4. Expected vs Actual

- expected: operator settle → lane upsert/remove on every open mux stream,
  and the client applies it.
- actual: no frame emitted; if emitted, revision equal to last-seen →
  client-side drop.

## 5. Root Cause

- Missing `markRunRealtimeDirty` at the three operator mutation endpoints.
- `Revision: rs.seq` conflated transcript sequence with mux ordering — but
  lane content also depends on the durable dispatch store, which advances
  outside `rs.seq`.

## 6. Fix

- All three handlers now `markRunRealtimeDirty(runID)` after a successful
  mutation.
- New service counter `s.runUpdateSeq` stamps `Projection.Revision` at every
  emit point (subscribe snapshot, drain upsert, ss-lock synthetic upsert) —
  the wire revision is now a monotonic mux-emission sequence, strictly
  increasing for every frame a connection sends.

## 7. Regression

- `TestRunUpdates_DispatchResolveMarksRunDirty` — red before, green after.
- `TestRunUpdates_UpsertRevisionAdvancesWithoutSeqChange` — red before
  (upsert rev == snapshot rev), green after.
- `go test -race -run 'TestRunUpdates|TestDecisionPayload|TestDispatch'` —
  all green.
