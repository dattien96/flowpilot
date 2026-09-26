# CA-1013 — BUG-512: production context-reset headroom callback wired

## Context

Deep review B-2: `contextResetHeadroomOK == nil` was treated as
"sufficient headroom" and only tests assigned it — the CP-87 P-3b branch
(reseed on the same binding only when it fits the node budget, else
route to the quota gate) could never fire in production.

## Changes

- `newInteractiveService` (`interactive_service.go`) wires a default:
  under `s.mu` it reads `nodeUsageTokensLocked(rs)` + a reseed estimate
  (`len(lastPrompt)/4`), unlocks, then compares against
  `nodeUsageBudgetFor(rs)` — uncapped nodes pass; capped nodes must fit
  strictly under budget.
- `bug512_headroom_seam_wired_test.go`: seam non-nil; uncapped → true;
  under-budget → true; over-budget → false.

## Verification

`go test -count=1 -run TestBug512 ./internal/runner/` — green.
