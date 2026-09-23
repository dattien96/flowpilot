# CA-941 — BUG-379: mux fingerprint moved per-subscriber

## Summary

Change detection for the Task-429 mux plane was stored on
`interactiveRun.muxFingerprint` — shared across all SSE subscribers. The
first subscriber to drain a dirty run advanced the shared fingerprint, so
every other subscriber compared equal and skipped the upsert: second
clients received snapshots but never subsequent lane updates.

Fix: `runUpdateSub.fps[runID]` per-subscriber last-sent fingerprint,
seeded from the subscribe snapshot, deleted on remove. The per-run field
was removed. Lock order unchanged.

## Verified

- `TestRunUpdates_EverySubscriberReceivesUpserts` red→green; all mux +
  decision-payload tests green under `-race`.

## Files

- `internal/runner/decision_payload.go`,
  `internal/runner/interactive_service.go` (field removed),
  `internal/runner/decision_payload_test.go` (additive),
  `requirements/09-BugFix/todo/BUG-379-…md` (new)

# ---8<--- flowpilot:change-ledger
feature_key: event-plane
source_doc_id: BUG-379
change_type: bugfix
