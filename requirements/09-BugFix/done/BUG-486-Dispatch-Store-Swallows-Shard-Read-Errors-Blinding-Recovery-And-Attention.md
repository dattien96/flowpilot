# BUG-486 — multiProjectDispatchStore swallows shard read errors → recovery coordinator and SSE attention see silent partial data

- Status: `done`
- Severity: **high** — fail-open on durable dispatch enumeration; same data-loss-adjacent family as BUG-485
- Found: 2026-09-25, BUG-484 live-drill prep (bed `/Users/tiendat/fp-beds/full`)
- CA: `change-audit/CA-985-bug486-dispatch-shard-errors.md`

## Symptom

`multiProjectDispatchStore` drops every shard-level error instead of
surfacing it:

| Site | Swallow | Consequence |
|------|---------|-------------|
| `ListRecoverable(ctx,"")` all-projects loop | `forProject` err → `continue`; `st.ListRecoverable` err → `continue` | `ScanAllRecoverable` gets partial set + nil error → BUG-484's retry coordinator sees a *successful* pass while recoverable records in the unreadable shard are silently skipped |
| `ListRecoverable(ctx,runID)` | `forRun` err → `nil, nil` | `ScanRun` treats an unreadable shard as "no records" |
| `ListAttention` | `forProject` err → `continue`; `ListAttention` err → `continue` | `dispatchAttentionByRun` never sees an error → BUG-475's resync-on-authority-failure branch is unreachable on the file-backed store — SSE snapshot claims empty attention while a shard is unreadable |
| `forRun` dir scan | `openProjectLocked` err → `continue` | run in an unreadable shard is indistinguishable from run-not-found |

The file layer DOES produce errors (`localDispatchStore.load` → `os.Open`/
`ReadBytes` failures, e.g. unreadable file, EIO); they die at the aggregator.

## Root cause

Aggregation loops were written "best effort" for multi-project fan-out —
fine for listing, wrong for durability authorities where absent ≠
unreadable. The recovery coordinator and the SSE attention path were built
to fail closed on store errors (BUG-484 retry, BUG-475 resync); the store
never gives them one.

## Fix contract

- `forRun`: shard-open errors during the scan are remembered; if the run is
  not found AND at least one shard could not be read → return that error
  (absence unprovable). `ErrNotFound` only when every shard read cleanly.
- `ListRecoverable("")` / `ListAttention`: accumulate first shard error;
  return partial results **with** the error so callers fail closed while
  still seeing what was readable.
- `ListRecoverable(runID)`: `ErrNotFound` → `nil, nil` (legit absence);
  other errors propagate.
- Memory store unchanged — it has no shard IO.

## RED tests (`bug486_dispatch_shard_errors_test.go`)

1. `ListRecoverable("")` with one unreadable shard → error returned
   (today: nil error + silently missing records).
2. `ScanAllRecoverable`/pass surfaces the failure (BUG-484 retry trigger).
3. `ListAttention` with unreadable shard → error → SSE/operator path sees
   authority failure (BUG-475 branch reachable).
4. `ListRecoverable(runID)` in unreadable shard → error, not empty.
5. Clean-not-found preserved: absent run on healthy shards → nil error.
6. `ListRecoverable`/`ListAttention` with all shards readable → unchanged
   results (no regression on the happy path).

## Live drill

chmod 000 one project shard's `dispatch.ndjson` → restart → expect
`[dispatch-recovery] pass N failed … retrying` + eventual
`pass still failing` or success after restore; `GET
/client/dispatch/attention` → 502 not empty list; restore perms → clean.

## DoD

- [ ] RED tests fail on main behavior
- [ ] errors propagate through all four aggregation sites
- [ ] `ErrNotFound` semantics preserved for true absence
- [ ] full suite green (baseline failures unchanged)
- [ ] live drill verified
- [ ] CA entry + commit
