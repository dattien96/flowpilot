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
- `maybeRecoverMissingVibeSSLock` — clear awaiting-lock fields + loop block; `startResolvedFlowFromNode(..., ingest_reader)`
- Wire after reconstruct (`interactive_resume`) and at start of `resumeVibeLock` before stamp/advance

## Tests

New `task327_missing_ss_auto_resume_test.go` only:

- reconstruct missing SS clears park
- reconstruct with SS keeps park
- Continue missing SS recovers (no seal / no cp_writer)
- empty cwd unknown
- Claude/Codex/Grok seeded runs (Case 1 agnostic)

Untouched and green: `TestVibeSession_ReconstructAwaitingLockAndIdempotent`, `TestCA793_*`, `TestBUG365_SSLockStampsApprovedOnDisk`.

## Providers

Case 1 agnostic: helpers take no `providerKey`; matrix seeds Claude/Codex/Grok runs.

## Will not undo

CA-793 demote exist-gate. CA-770 awaiting-lock restore when SS path retained / cwd empty. CA-820 / BUG-365 stamp approved when SS still on disk. CA-791 join.
