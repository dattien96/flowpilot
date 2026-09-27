# BUG-529 — Tournament escalation child is not durable before async dispatch

## Severity

Important — kill/restart can lose the rescue child.

## Evidence

- `apps/local-runner/internal/runner/tournament_escalation.go:120-193`
- `apps/local-runner/AGENTS.md:27-43`

`escalateToTournament` constructs the child directly in `s.runs`, changes the parent loop, then launches `startTurn` in a goroutine. It does not persist the child session before returning. A process kill or a rejected first turn in this window leaves no durable child row. The rejection path only logs diagnostics.

## Impact

The parent can remain in `tournament_escalation` with no recoverable child and no provider work in flight. The in-memory dedup and happy-path tests cannot detect the loss after restart.

## Missing regression

Fault/kill after child mint but before first-turn admission, reconstruct the service, and assert either:

- the durable tournament child resumes exactly once, or
- the parent enters an explicit retryable/repair state and can recreate it safely.

No RAM-only rescue state should be reported as successfully dispatched.

## Scope

Capture only. No fix applied during the 2026-09-27 branch review.
