---
id: CA-932
title: Dispatch durability — repair resolution ordering, receipt disk-before-RAM, seq rollback (BUG-447, BUG-448, BUG-449)
type: BugFix
feature: agent-flow-engine
date: 2026-09-23
status: done
---

## Context

Three durability defects in the in-memory dispatch store, all violating
disk-before-RAM or commit atomicity:

- BUG-447: `CommitRepairResolution` persisted `resolved` on the repair record
  before terminalizing live dispatch records — a mid-path failure left the
  repair marked resolved with live records stranded and unreachable on retry.
- BUG-448: `CommitReceiptAndClearIntent` mutated RAM (state →
  `provider_accepted`, intent cleared) before the durable commit succeeded —
  a failed commit left RAM ahead of disk, and a retry could mask the lost
  receipt via the stale in-memory state.
- BUG-449: a failed terminal commit burned the dispatch audit sequence —
  `s.seq` advanced in RAM while the durable write failed, leaving a gap in the
  NDJSON log.

## Change

`internal/runner/dispatch_store_memory.go`:

- `CommitRepairResolution` terminalizes eligible live records first, persists
  each transition, and only then writes the repair `resolved` record — a
  failure anywhere leaves the repair open and every record still reachable.
- `CommitReceiptAndClearIntent` builds and durably commits the
  `provider_accepted` record (with fsync via `afterCommit`) before mutating
  RAM or clearing the intent; retry sees the durable revision, never a stale
  in-memory one.
- `commitTerminal` (and the shared commit path) rolls back `s.seq` and audit
  state when `commitLine` fails — failed commits no longer consume sequence
  numbers.

## Tests (added only)

- `internal/runner/bug447_449_durability_test.go` — fault-injection tests:
  abandon cannot resolve while live records remain; receipt commit failure
  leaves RAM/disk consistent and retry persists the receipt (verified across
  restart); failed terminal commit leaves the log's sequence contiguous.
  All honestly red before the change (fixture needed a disarmable failure
  injection — nested-closure wrapping was fixed so the injected error can be
  cleared between attempts).

## Result

- `go test -count=1 -run 'Bug447|Bug448|Bug449'` — green.
- AGENTS §2 invariants restored on these paths: durable record is the source
  of truth, non-terminal records stay reachable, delivery stays three-outcome.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-447
change_type: bugfix
summary: Dispatch store — repair resolution terminalizes records first; receipt commit is disk-before-RAM; failed terminal commit rolls back audit sequence
# --->8---
