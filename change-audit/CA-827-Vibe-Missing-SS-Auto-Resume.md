# CA-827 — missing SS while ss_lock parked auto-resumes ingest_reader

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: Task-327
change_type: feature
summary: Clear ss_lock park and restart vibe-ingest from ingest_reader when SS artifacts are missing (reconstruct + Continue); closes CP-60 O-6 for R-SS-D
# --->8---

## Why

Live R-SS-D (run-225468): operator deleted `SS-*.md`, reopened same run, TUI still showed **SS Preview & Lock**. Durable `vibe_checkpoint_node` was already empty (CA-793 demote), but lock park / Continue still assumed a draft. Continue would stamp/advance toward `cp_writer` instead of regenerating SS.

CA-793 Replay already noted: demote on reconstruct **without** auto-`startResolvedFlowFromNode`. CP-60 O-6 residual.

## Change

- `vibeSSLockArtifactsPresent` — empty cwd = unknown/present (keeps CA-770)
- `restartVibeIngestForMissingSS` — clear lock **and resume-confirm**; start `ingest_reader`
- `maybeRecoverMissingVibeSSLock` — awaiting ss_lock **or** stacked resume-confirm + missing SS
- Reconstruct: recover **before** `maybeParkVibeResumeConfirm`
- `parkVibeLock(ss_lock)`: if SS missing → restart ingest (no empty Preview card)
- `SubmitGateDecision` Resume OK: restart ingest when SS missing (do not `tryAdvance` → empty lock)

## Tests

New `task327_missing_ss_auto_resume_test.go` only:

- reconstruct missing SS clears park
- reconstruct with SS keeps park
- Continue missing SS recovers (no seal / no cp_writer)
- Resume-confirm OK + missing SS restarts ingest (no empty lock)
- parkVibeLock + missing SS restarts ingest
- empty cwd unknown
- Claude/Codex/Grok seeded runs (Case 1 agnostic)

Untouched and green: `TestVibeSession_ReconstructAwaitingLockAndIdempotent`, `TestCA793_*`, `TestBUG365_SSLockStampsApprovedOnDisk`, `TestCA801_GateOKClearsConfirm`.

## Addendum (live residual)

After first land: reopen showed Resume confirmation; OK ran ingest but also `tryAdvance` → `parkVibeLock` with no SS (empty lock + ss_lock failure). Fixed above.

## Providers

Case 1 agnostic: helpers take no `providerKey`; matrix seeds Claude/Codex/Grok runs.

## Will not undo

CA-793 demote exist-gate. CA-770 awaiting-lock restore when SS path retained / cwd empty. CA-820 / BUG-365 stamp approved when SS still on disk. CA-791 join.
