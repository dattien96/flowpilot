# BUG-508 — Completed workflow runs return `run_not_found` after a runner restart though durable rows exist

## Status
FIXED + LIVE-VERIFIED — 2026-09-26, fixed build, runner on :19400.

- Fix (CA-1005): `runSnapshot` falls back to a durable single-run snapshot
  (`durableRunSnapshot` via `SessionHistoryReader.GetProviderSession`)
  when the in-memory `s.runs` cache has no entry — the same store-as-SSOT
  pattern the history endpoint already uses. `s.runs` is a write-through
  cache that is empty on a fresh process; the boot reconciler does not
  rehydrate every terminal row, so the endpoint must read through.
- Unit: `bug508_snapshot_rehydrate_test.go` (green).
- Live re-verify (fixed build, post-restart): `GET
  /client/workflow-runs/run-6010` and `…/run-16950` both return 200 with
  providerSessionId/providerKey/workingDirectory populated — the same
  calls returned `run_not_found` on the pre-fix binary while the durable
  session rows existed on disk.

## Live-found during
Full CP live-test rerun, 2026-09-26, build 8c95a5bb — post-SIGKILL
durability drill (B-51).

## Observed

- After `kill -9` + `runner serve` restart, `GET /client/workflow-runs/
  run-16950` returned `{"error":{"code":"run_not_found"}}` while
  `chats/sessions.ndjson` still held the run's persisted rows (status
  `running`/`completed`). 4 of 8 probed runs showed the same asymmetry:
  durable rows present, snapshot endpoint 404.
- The history endpoint (project history) still listed the runs — proving
  the durable store was intact; only the in-memory snapshot path was
  empty.

## Root cause

`runSnapshot` (interactive_handlers.go) read only `s.runs[runID]` — the
live in-memory map. Runs that terminated before restart (or whose rows
the reconciler skipped) are absent from the map even though the workflow
store still holds their session record.

## Fix applied

Same pattern as BUG-060's history SSOT: on cache miss, project a
read-only view from the durable session row instead of 404. The projected
view carries only what the durable record proves (ids, provider key,
status, working directory) — no live gate/approval state exists to fake.

## Related
- BUG-502: durable rows themselves lost across restart (different defect —
  write-side). This bug was read-side only.
