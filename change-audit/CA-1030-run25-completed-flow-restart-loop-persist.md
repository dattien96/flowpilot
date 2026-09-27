# CA-1030 — run-25: completed flow run survives kill/restart (loop evidence no longer erasable)

Date: 2026-09-26 — restart-recovery bug found live on run-25 (CP-64 drill)

## Change

`apps/local-runner/internal/runner/interactive_service.go`:

- `persistProviderSession` (the single funnel for every `sessions.ndjson`
  write) now backfills `session.LoopState` from
  `s.agentOrchestrator.loopStateFor(session.RunID)` whenever the caller left
  it zero-valued. `sessionStateOf` never carries LoopState — the orchestrator
  owns it — so every persist site that forgot the explicit attach emitted a
  `loop=None` row capable of overwriting a terminal `loop=done` row under
  last-wins replay. Child runs own no loop (`loopStateFor` returns zero), so
  the backfill is a no-op for them; callers that attach a non-zero snapshot
  keep their value. `o.mu` only — safe under `s.mu` and from detached persist
  goroutines. The backfill is nil-guarded on `s.agentOrchestrator` because
  hand-constructed services in low-level persist tests (BUG-489) have none —
  a nil orchestrator means no loop record exists to backfill anyway.

`apps/local-runner/internal/runner/local_file_session_store.go`:

- `loopStatePtrIfSet` now treats a non-empty `Status` as set — previously a
  loop recorded with only `Status` populated serialized as absent, so even a
  correctly-attached Status-only loop could still land as `loop=None`.

## Why

Live evidence on run-25: the seal path wrote `status=completed, loop=done`
at 14:05:20.407; `applyFlowControl("done")` then appended the "Flow
completed." note through `appendPendingAgentContextLocked`, whose async
persist carried `sessionStateOf(parent)` with no LoopState — the goroutine
landed last, so `scanSessionsNDJSON` last-wins exposed
`completed + loop=None + pending_agent_context=[note]`. On restart
`resumedFlowRunIncomplete` classified the sealed run incomplete and
`normalizeResumedFlowRun` flipped it to `cancelled`; the stall watchdog then
persisted `cancelled + blocked/hub_stalled` at 14:10:02.

Fixing only `appendPendingAgentContextLocked` would leave every other
loop-less persist site (gate prep, flowStartOnly synthetic settle, cohort
notes, child snapshots, ~20 call sites) able to produce the same corruption.
The funnel guarantee — "no session row lands without the orchestrator's
current loop evidence" — matches the contract that the durable record is the
single source of truth and removes the whole bug class, including future
call sites.

The resume classifier is unchanged: `completed + loop="" + pending_ctx`
remains fail-closed "incomplete" for rows already corrupted on disk by the
old code (indistinguishable from a never-armed loop), but no new row can
produce that shape while a loop is mounted.

## Tests

- `TestRun25_AppendPendingContextPreservesTerminalLoopOnDisk` — real
  `localFileSessionStore`: persist `completed+loop=done`, then
  `appendPendingAgentContext` (the exact run-25 call); last-wins row must
  keep `loop=done`. Verified RED pre-fix (`LoopState{}` read back).
- `TestRun25_RestartKeepsFlowRunCompleted` — end-to-end restart: same
  durable rows, fresh service, `reconstructRun` on the loaded record;
  asserts `completed`, not `cancelled`. Verified RED pre-fix (flipped).

## Risk

Low: the backfill only adds LoopState to rows that previously wrote none;
explicitly-attached snapshots are untouched. Rows for runs with no mounted
loop are byte-identical to before. One behavioral nuance: a session row
written for a run id that shares a mounted loop gets that loop attached —
which is precisely the durable-truth semantics the store intends.
