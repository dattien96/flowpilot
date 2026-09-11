# CA-777 — ss_validator done parks ss_lock instead of escalate

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: CP-60
change_type: bugfix
summary: Hub submit_review_outcome(done) onto user.confirm ss_lock calls parkVibeLock; do not fail-closed escalate undispatchable successor
# --->8---

## Why

Live run-211980: ingest_reader/ss_converter worked; ss_validator approved then `Flow done successor "ss_lock" could not be dispatched`. `tryAdvanceFlowFromNode` already parks `user.confirm` vibe locks; `advanceHubDoneThroughEdge` (hub.inline tool) did not.

## Change

`advanceHubDoneThroughEdge`: if done-successor is `user.confirm` + vibe lock node → `parkVibeLock`, return awaiting_user.

## Tests

`ca777_hub_done_parks_ss_lock_test.go`

## Will not undo

CA-775/776. run-201295 undispatchable freeze escalate still for non-lock successors.
