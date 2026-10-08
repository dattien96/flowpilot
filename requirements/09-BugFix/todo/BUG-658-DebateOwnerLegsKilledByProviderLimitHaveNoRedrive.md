# BUG-658 — Debate owner legs that fail on `provider_limit_reached` leave `debate_synthesis` RUNNING with no verdicts and no redispatch path: the synthesis hub re-evaluates dead owners forever and re-parks `blocked` on every continue

- **ID:** BUG-658
- **Severity:** High — a transient quota reset (~1h) becomes a permanent
  wedge: the cohort seats are marked FAILED (unrecoverable), the barrier
  never joins, and each continue only re-reads the same dead state.
- **Status:** FIXED — CA-1236 (2026-10-08): owner-fail retry ladder runs when both owners are terminal-failed despite a stale RUNNING synthesis step; continue on dead synthesis routes to the settle ladder

## Evidence chain (all live)

1. Owner legs `owner_1`/`owner_2` for a Task-114 debate both FAILED on
   provider_limit; `debate_synthesis` kept a RUNNING stamp with a live
   re-eval turn.
2. Hub turn submitted `blocked` outcome ("both owners dead, reset at
   02:01Z") → park; `agent-loop/continue` → new synthesis turn → same
   `blocked` → re-park. Observed the cycle repeat; no route re-spawned
   owners after the quota window passed.
3. Recovery required operator `flow-control` edge traversal to bypass
   the dead cohort entirely — the debate results were abandoned.

## Root cause (hypothesis)

Cohort member failure is terminal for the seat (correct), but nothing
offers "re-spawn cohort" after transient infra failure — the synthesis
node only knows how to re-evaluate existing seats, and the debate-mount
predicate only fires on fresh gates, not on "all members dead + park".

## Fix direction

- `F-1` When all cohort members fail on a TRANSIENT class error
  (provider_limit/quota/transport), arm a retry-cohort path: after a
  backoff, respawn owners once before declaring blocked.
- `F-2` Synthesis parks on dead cohorts should surface a `retry cohort /
  skip debate / escalate` decision card instead of re-evaluating.

## Regression coverage

- `TestBug658_AllOwnersQuotaFailRespawnsAfterReset` — transient-fail
  cohort → retry spawn scheduled, synthesis rejoins.
- `TestBug658_PermanentMemberFailureEscalates` — non-transient failure
  still surfaces blocked without infinite re-eval.
