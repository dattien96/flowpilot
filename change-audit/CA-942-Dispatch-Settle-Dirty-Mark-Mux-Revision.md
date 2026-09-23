# CA-942 — BUG-380: dispatch settle marks lane dirty; mux wire revision is a monotonic emission sequence

## Summary

Two stacked defects left settled `dispatch_attention` items stale in the
inbox: the three operator endpoints (resolve, retry-as-new,
repair-resolution) never marked the run dirty, and the wire `revision`
(`rs.seq`) did not advance on decision-only changes so the client's
`rev <=` dedupe dropped the upsert anyway.

Fix: `markRunRealtimeDirty` after each successful mutation; wire revision
now stamped from `s.runUpdateSeq` — a per-connection monotonic emission
sequence — at every emit point (snapshot lane, drain upsert, ss-lock
synthetic upsert).

## Verified

- `TestRunUpdates_DispatchResolveMarksRunDirty` red→green.
- `TestRunUpdates_UpsertRevisionAdvancesWithoutSeqChange` red→green.
- Mux + decision + dispatch tests green under `-race`.

## Files

- `internal/runner/dispatch_operator.go`,
  `internal/runner/decision_payload.go`,
  `internal/runner/interactive_service.go` (runUpdateSeq field),
  `internal/runner/decision_payload_test.go` (additive),
  `requirements/09-BugFix/todo/BUG-380-…md` (new)

# ---8<--- flowpilot:change-ledger
feature_key: event-plane
source_doc_id: BUG-380
change_type: bugfix
