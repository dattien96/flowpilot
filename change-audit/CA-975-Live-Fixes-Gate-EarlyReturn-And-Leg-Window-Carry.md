# CA-975 — Live-test fixes: budget check behind gate early-return + same-leg window carry-forward

## Summary

Two bugs surfaced only under the real provider (Devin SWE-2) during the
live-verification phase, after the Task-440–444 suites were already green.
Both were fixed TDD-style: a failing test against the real code path first,
then the minimal production change.

## Bug 1 — `maxUsageTokens` silently skipped for artifact-less children

`checkUsageBudgetPostTurn` sat at the bottom of
`runChildArtifactOutputGateAtEpoch` / `runFlowGateAtEpoch`, after early-return
guards. A profiled child with no artifact bindings / no coding contract took
the no-op gate exit and never ran the post-turn budget check — the live Devin
flow burned 12,384 real tokens on a `maxUsageTokens: 2000` test profile with
no card.

Fix: hoisted `s.checkUsageBudgetPostTurn(rs, turnID)` to the top of both gate
functions (after the nil-guards, before every early return); removed the
redundant later call. The check is now a post-turn invariant of the gate
dispatch layer, not a gate-rule side effect.

Regression test: `TestTask442_GateEarlyReturnStillEnforcesBudget`
(`task442_usage_budget_test.go`) — drives the gate path itself; red before
the fix, green after.

## Bug 2 — provider omits context window on the terminal usage event

Devin reports `size` (window 262,000) on a mid-turn `usage_update` but drops
it on the turn-terminal usage event — so the final, authoritative event lost
`ModelContextWindow` and could skip pressure evaluation / UI display.

Fix: `interactiveRun.legContextWindows map[string]int64` keyed by
`ProviderSessionID`. In `emitLocked` the window resolution order is now:
provider self-report → static catalog → same-leg remembered window. A new leg
starts with an empty map — never inherits the previous leg's window.

Regression test: `TestTask440_LegWindowCarryForward`
(`task440_window_enrichment_test.go`).

## Live verification (this machine, Devin SWE-2 High)

- `flowpilot runner serve` @ 127.0.0.1:4317, workspace `C:\tmp\fp-live-ws3`.
- Real flow turn: `usage_budget_exceeded` card fired post-turn with
  `extend`/`stop` (rotate correctly absent — routing seam not wired);
  answering `stop` via `POST …/questions` was accepted and the run escalated.
- Terminal `token_usage_updated` now carries `modelContextWindow: 262000`
  (carry-forward) where the first live run had it missing on evt-8.
- Live devin report: window 262,000; a trivial turn ≈ 11–12.4k tokens.

# ---8<--- flowpilot:change-ledger
feature_key: token-usage
source_doc_id: Task-440;Task-442
change_type: bugfix
summary: hoist post-turn usage-budget check above gate early-returns (live Devin run exposed skip); remember last self-reported context window per leg so terminal usage events keep ModelContextWindow
# --->8---
