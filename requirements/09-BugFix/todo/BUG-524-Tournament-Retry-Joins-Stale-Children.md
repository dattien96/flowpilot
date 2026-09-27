# BUG-524 — Tournament retry can join against stale children

## Severity

High — arbiter can evaluate an incomplete retry cohort.

## Evidence

- `apps/local-runner/internal/runner/tournament_dispatch.go:89-139`
- `apps/local-runner/internal/runner/tournament_dispatch.go:312-335`

`tournamentJoinSatisfied` accepts any terminal child whose parent and label match a predecessor. A retry spawns new children with the same candidate labels while prior terminal children remain in `s.runs`. A terminal child from round 1 can therefore satisfy the round-2 join while the round-2 child with the same label is still running.

## Impact

The arbiter may inspect a half-written worktree, choose the wrong winner, or merge an incomplete patch.

## Missing regression

Run a two-round tournament where round-1 children remain resident and one round-2 candidate is deliberately slow. Assert that the arbiter does not start until every child from the current cohort/attempt is terminal. Repeat after restart using durable step evidence.

## Scope

Capture only. No fix applied during the 2026-09-27 branch review.
