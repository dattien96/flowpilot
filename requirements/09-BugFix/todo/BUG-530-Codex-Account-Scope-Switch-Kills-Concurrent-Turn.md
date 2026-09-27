# BUG-530 — Codex account scope switch kills a concurrent turn

## Severity

High — multi-account quota routing is incompatible with the process pool.

## Evidence

- `apps/local-runner/internal/runner/codex_appserver_process.go:83-101`
- Quota routing permits concurrent runs pinned to different connected accounts.

`Runner` stores one `codexAppServer` handle. `ensureCodexAppServer` reuses it only when `scopeKey` matches; requesting another account scope closes the existing dispatcher and kills its process before starting the replacement. It does not check whether the first scope has an in-flight turn.

## Impact

Two legitimate concurrent Codex runs on different account claims terminate each other's app-server. The first turn receives a teardown failure even though its account and claim remain valid; repeated traffic can thrash the singleton process between scopes.

## Missing regression

Start an in-flight turn on Codex account A, then admit a second run pinned to account B. Assert that B does not kill A and both complete under their own scope. Verification must use process/dispatcher lifecycle behavior, not only in-memory quota ranking.

## Scope

Capture only. No fix applied during the 2026-09-27 branch review.
