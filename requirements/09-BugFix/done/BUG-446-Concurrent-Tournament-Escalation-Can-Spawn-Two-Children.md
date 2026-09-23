---
id: BUG-446
title: Concurrent tournament escalation can spawn duplicate rescue children
status: done
version: 1
created: 2026-09-23
updated: 2026-09-23
owner: FlowPilot
linked: [BUG-413, CA-923]
---

## AI Quick View
- **What**: Concurrent cap/stall triggers can both pass tournament dedup and create distinct child runs.
- **Why**: The existing-child check and child insertion use separate critical sections.
- **Key constraint**: Dedup must be atomic and durable across restart without blocking provider I/O under the run mutex.

## 1. Metadata
- Document ID: `BUG-446`
- Phase: `bugfix`
- Status: `done`
- Feature Keys: `agent-flow-engine`, `tournament-harness`
- Parent Documents: [BUG-413](../done/BUG-413-Duplicate-Tournament-Escalation-Dispatch.md), [CA-923](../../../change-audit/CA-923-tournament-escalation-dispatch-card-and-join-fixes.md)

## 2. Symptom and Impact
BUG-413 claims repeated rescue triggers cannot dispatch duplicate tournament children. `maybeEscalateCapToTournament` first reads loop state, scans `s.runs` under `s.mu` for a child with label `tournament_escalation`, **releases** the mutex, then calls `escalateToTournament` (`internal/runner/tournament_escalation.go:203-234`). The latter calls `shouldEscalateToTournament` and acquires the mutex again before choosing a free child ID (`:99-151`), but does **not** recheck child existence or loop state inside that lock; its ID allocator deliberately picks `-2` when the first ID exists.

Two concurrent callers can both finish the scan before either inserts; caller A creates `run-X-tournament`, caller B creates `run-X-tournament-2`; both dispatch first turns and mutate the parent loop. Each child may run a competing rescue, with duplicate cost and conflicting decisions/patches. Severity: **high**. Existing BUG-413 tests cover sequential Continue/repeat, not concurrent triggers.

## 3. Reproduction / Evidence
Code-path interleaving: A/B read `Status=blocked` → A/B independently scan zero children and unlock → A inserts `-tournament` → B inserts `-tournament-2` → each schedules `startTurn`. This is a deterministic race schedule derived from the separated locks; **no new red concurrency test or live reproduction was run during review**.

## 4. Acceptance and Verification
Add an assertion-based concurrent E2E test using a synchronization barrier, plus restart/retry and different rescue-trigger combinations; enforce a single durable ownership claim/child ID and no duplicated first turn. Keep existing sequential tests unchanged, verify provider parity and write CA only after fix.

## 5. Resolution (2026-09-23, CA-931)

- Dedup membership check + child-run insertion now occur inside the same
  critical section in `tournament_escalation.go` — the check is authoritative;
  a second concurrent trigger observes the registered child and does not
  spawn.
- Test: `bug446_453_tournament_test.go` concurrent test — red before fix
  (spawned `-tournament-2/3/4/5/7`); green after; `-race` clean.
