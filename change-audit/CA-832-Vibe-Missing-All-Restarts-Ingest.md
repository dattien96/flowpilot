# CA-832 — delete SS+CP+Task on vibe-sprint restarts ingest (not Resume→tdd)

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: BUG-373
change_type: bugfix
summary: R-TK-D3 — when SS is gone, restart ingest_reader even if active graph is vibe-sprint; do not park Resume from tdd
# --->8---

## Why

Live R-TK-D3 (run-225468): operator deleted SS+CP+Task, reopen parked **Resume from tdd?**. `restartVibeIngestForMissingSS` required ingest/ss_lock nodes in the active graph; after sprint the nodes are tdd/coder. CP/Task recover need SS or CP present — both gone — so fallthrough parked resume.

## Change

- Broaden `vibeIngestHasSSLockTopology` to vibe-sprint / cp-ingest / checkpoint / plan history
- Reconstruct switch calls `restartVibeIngestForMissingSS` directly
- Clear sprint cursor + retarget vibe-ingest + release stop fence

## Tests

`bug373_missing_all_restarts_ingest_test.go`. Task-327 / CA-770 / CA-801 green.

## Providers

Case 1 agnostic.
