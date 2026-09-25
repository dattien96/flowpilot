# BUG-475: realtime projection treats dispatch-attention store failure as an empty decision set

## Metadata

- Document ID: `BUG-475`
- Phase: `bugfix`
- Status: `done`
- Severity: `high`
- Evidence: `code-confirmed; store-failure E2E green (CA-973)`
- Feature Keys: `event-plane`, `agent-flow-engine`
- Parent Documents: `CP-84`, `CP-51`, `SS-17`, `SD-24`
- Related Documents: `BUG-380`, `B-51-5`
- Affected Area: `internal/runner/decision_payload.go`

## Summary

`dispatchAttentionByRun` silently converts `DispatchStore.ListAttention` errors
into an empty map. Snapshot/drain code then emits a normal authoritative
projection without uncertain/repair/cancel-required decisions. A client can
reconcile away operator attention even though the durable authority was merely
unreadable, not resolved.

## Evidence

```go
if list, err := store.ListAttention(context.Background()); err == nil {
    ...
}
return byRun
```

There is no error return, retry/resync frame, stale projection preservation, or
repair marker. `subscribeRunUpdates` and `drainRunUpdates` both consume this
empty map as authoritative.

## Expected vs Actual

- Expected: dispatch authority read failure fails closed; retain prior decision
  state or emit retryable resync/repair indication. Never claim “no attention.”
- Actual: failure is projected exactly like a successful empty result.

## Impact

- `uncertain`, `cancel_required`, `repair_required`, or `settle_pending` items
  can temporarily disappear from Desktop inbox.
- A terminal run whose only relevance is dispatch attention can be emitted as a
  remove, stranding the repair surface until a later successful poll/snapshot.
- This breaks CP-51's “no silent loss” operator contract at the CP-84 projection
  boundary while leaving durable records intact underneath.

## Required Fix Contract

1. Preserve the distinction between empty attention and failed read.
2. On failure, do not emit a decision-clearing authoritative frame.
3. Use a retryable resync/error path without leaking payload data.
4. Recovery API and per-run attention endpoint remain authoritative.

## Required Tests

- RED snapshot with failing `ListAttention` must not clear a prior repair item.
- RED dirty drain failure must not emit remove for a terminal repair-required run.
- Recovery after store restoration emits the current authoritative decision.
- HTTP SSE E2E across failure→reconnect→snapshot.

## Implementation Plan

### P-1 — RED projection tests

- Add a DispatchStore wrapper whose `ListAttention` can switch between success,
  failure and recovery while retaining a real repair record.
- Cover both `subscribeRunUpdates` snapshot and `drainRunUpdates` dirty update.
- Assert current behavior incorrectly emits empty decisions/remove.

### P-2 — Preserve authority failure

- Change `dispatchAttentionByRun` to return `(map, error)`.
- Snapshot handler: if attention cannot be read, do not send an authoritative
  snapshot that can clear dispatch decisions. Emit a retryable `resync` or
  terminate before snapshot completion so client staging cannot commit it.
- Dirty drain: on failure retain subscriber dirty state/fingerprint and request
  retry/resync; never update `sub.fps` from incomplete data.
- Log only run/store metadata, never decision reason/payload text.

### P-3 — Client recovery semantics

- Ensure Desktop treats retryable resync/stream close as reconnect-required and
  retains the previously committed attention set until the next complete
  snapshot.
- Confirm poll fallback reports typed failure rather than successful emptiness.

### P-4 — Cross-contract verification

- Run CP-51 uncertain/cancel-required/repair-resolution suites and CP-84 mux
  snapshot/reconnect suites together.
- Inject store failure while a terminal run has only `repair_required`; restore
  store and verify the same decision ID/revision returns.

## Definition of Done

- [ ] Store failure is distinguishable from a valid empty attention list.
- [ ] No failed read can emit a decision-clearing authoritative frame.
- [ ] Previous client attention survives until a complete healthy snapshot.
- [ ] Recovery emits stable decision IDs/revisions and no duplicate cards.
- [ ] Terminal repair-required lane is never removed during authority failure.
- [ ] B-51-5 resolution/replay semantics remain green.
- [ ] CP-84 HTTP SSE, reconnect and malformed-input tests pass.
- [ ] Live store failure/recovery evidence is recorded in the BUG and CA docs.
