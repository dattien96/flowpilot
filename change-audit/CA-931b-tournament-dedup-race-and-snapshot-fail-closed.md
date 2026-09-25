---
id: CA-931b
title: Tournament escalation dedup race + snapshot fail-closed (BUG-446, BUG-453)
type: BugFix
feature: tournament-harness
date: 2026-09-23
status: done
---

## Context

- BUG-446: tournament escalation dedup checked membership and inserted in two
  separate critical sections — concurrent triggers could both pass the check
  and spawn multiple tournament runs (live reproducer showed `-tournament-2/3/
  4/5/7` spawned together).
- BUG-453: arbiter snapshot/diff acquisition errors were tolerated and the
  merge-side `winnerPatch` lookup treated an absent map entry the same as a
  valid empty patch — a snapshot failure could silently drop the winner patch
  and the cleanup path removed the only merge source.

## Change

`internal/runner/tournament_escalation.go`:

- Dedup membership check and run-record insertion now happen inside the same
  critical section — the check is authoritative, so a second concurrent
  trigger observes the already-registered tournament child and does not spawn.

`internal/runner/tournament_behavior.go` (+ dispatch wiring):

- Arbiter fails closed on snapshot/diff errors — no arbitration proceeds on a
  partial snapshot.
- Winner-patch lookup is presence-aware: an absent entry is distinguished from
  an intentionally empty patch, so patch loss surfaces instead of merging an
  empty diff.

## Tests (added only)

- `internal/runner/bug446_453_tournament_test.go` — concurrent escalation
  spawns exactly one tournament child (red before: multiple `-tournament-N`
  children observed); snapshot-failure path leaves merge source intact;
  absent-vs-empty patch distinguished.
- `-race` run clean.

## Result

- Focused tournament tests green; `go test -race -count=1` on the escalation
  concurrency test clean.
- Deterministic arbitration invariant (AGENTS §4) preserved — no ungrounded
  ranking introduced.

# ---8<--- flowpilot:change-ledger
feature_key: tournament-harness
source_doc_id: BUG-446
change_type: bugfix
summary: Tournament escalation dedup atomic under one critical section; arbiter fails closed on snapshot errors; winner-patch presence-aware
# --->8---
