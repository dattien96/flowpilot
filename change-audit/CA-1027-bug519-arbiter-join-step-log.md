# CA-1027 — BUG-519: arbiter join falls back to durable step log after restart

Date: 2026-09-26 — BUG-519 found live on run-49109 post-restart resume

## Change

`apps/local-runner/internal/runner/tournament_dispatch.go`:

1. `tournamentJoinSatisfied` — memory scan collects pending predecessors
   under `s.mu`, then releases it and consults
   `tournamentTerminalNodesFromLog`. A predecessor whose last durable
   step stamp is terminal (DONE/FAILED/CANCELED/SKIPPED) is removed
   from pending; only truly-unsettled siblings keep the join blocked.
2. `tournamentTerminalNodesFromLog` — replays
   `StepTransitionLogStore.LoadStepTransitions` for the parent run
   (last-wins per node), returns the terminal-stamped ids. Store absent
   or log unreadable/empty → nil → memory-scan-only semantics
   (fail-closed; no fail-open on missing evidence). I/O outside the lock.

## Why

The house contract: durable state is the source of truth across
kill/restart. The join gate violated it by trusting only in-memory
`s.runs`, which deliberately drops finished cohort members on resume
(reconstructPendingChildSessions restores only live/pending). Any
tournament interrupted at the arbiter became unrecoverable — operator
could continue forever and the join would never satisfy.

`runTournamentArbiterNode`'s downstream patch-snapshot path reads
worktree layouts/patches persisted on the parent run's state — those
survive restart independently of s.runs, so satisfying the join is
sufficient to make the arbiter recoverable.

## Tests

- `TestBug519_JoinSatisfiedViaStepLogAfterRestart` — children absent
  from memory; sidecar DONE stamps satisfy the join.
- `TestBug519_JoinStillBlocksOnNonTerminalStepLog` — RUNNING last-stamp
  still blocks (no fail-open).
- `TestBug519_JoinCountsFailedStepLogStamp` — FAILED satisfies (the
  live run-1890 "failed member counts" contract preserved at the
  durable layer too).

## Risk

Low. Single caller (`runTournamentArbiterNode`); fallback fires only
when the memory scan already found pending predecessors. GitNexus
impact: runTournamentArbiterNode + TestBug426 direct callers, LOW.
