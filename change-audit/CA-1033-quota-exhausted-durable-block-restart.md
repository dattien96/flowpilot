# CA-1033 — quota_exhausted persists a durable account block (survives restart), lifts on fresh headroom

Date: 2026-09-26 — review finding: a live-observed `quota_exhausted` entered
the quota gate but never reached the durable routing ledger, so after a
runner restart the just-exhausted account silently re-entered rotation and
could be re-pinned — telemetry is exactly the evidence most likely to be
absent/stale at boot.

## Change

`apps/local-runner/internal/runner/interactive_service.go`:

- `finishTurn`'s provider-limit branch now calls `noteAccountBlockedLocked`
  for `quota_exhausted` alongside `billing_required`/`credits_exhausted`.
  All three hard kinds behave identically for durability; `rate_limited`
  stays transient (Task-445 retry policy owns it).

`apps/local-runner/internal/runner/quota_preflight.go`:

- New `quotaBlockLiftedOnHeadroom` helper: a recorded `quota_exhausted`
  block is contradicted by fresh exact telemetry reporting healthy/low
  headroom — the provider window refilled, so the stale row must not veto
  a recovered account. `billing_required`/`credits_exhausted` never lift on
  telemetry; stale/unknown telemetry keeps the block (fail closed on the
  last hard observation). `ObservedLimit` doc updated: the gate-time
  rejection is now complemented by durable persistence on the emit path.

`apps/local-runner/internal/runner/quota_candidates.go`:

- `CrossProviderCandidates` applies the lift rule when evaluating the
  durable block ledger.

`apps/local-runner/internal/runner/quota_preflight.go` (`SameProviderCandidates`):

- Same lift rule — a recovered account must re-enter rotation on fresh
  headroom even while its block row still stands.

`apps/local-runner/internal/runner/quota_claim.go`:

- `claimAccountForLeg` honors the same lift so a manual `use_once`/`use_for_run`
  pick of a recovered account is not vetoed by a stale row.

`apps/local-runner/internal/runner/quota_gate.go`:

- `pinnedAccountHardVeto` restructured: billing/credits blocks veto on the
  ledger alone (a live probe cannot pay a bill); a `quota_exhausted` block
  defers to telemetry — fresh healthy headroom lifts it, absent/stale
  telemetry or an exhausted reading keeps the veto.

## Why

The review claim was only half-right: `accountBlockReason` was already read
by candidate selection and the pinned-account veto (BUG-511), but the emit
path never wrote `quota_exhausted` into the ledger. Persisting it
unconditionally would have been wrong too — quota windows self-heal, so a
permanent block would poison a recovered account. The fix makes the block
durable **and** evidence-bounded: it survives restart and stands while the
last hard observation is unrefuted, and lifts only when fresh exact
telemetry proves recovery.

## Tests

`quota_exhausted_restart_block_test.go` (new, red-first):

- `TestQuotaExhaustedBlockPersistsAndSurvivesRestart` — real entry path:
  `startTurn` → provider adapter returns a `usage limit` error →
  `finishTurn` classifies `quota_exhausted` → durable ledger row written;
  a fresh `InteractiveService` sharing only the ledger file (restart
  simulation) re-pins the account via `startTurn` → veto fires →
  `quota_route_*` — the turn never dispatches onto the blocked account.
- `TestQuotaExhaustedBlockLiftsOnFreshHealthyTelemetry` — recorded
  `quota_exhausted` block + fresh exact healthy telemetry → `pinnedAccountHardVeto`
  returns ""; stale/unknown telemetry → veto stands.

Regression: `go test -count=1 -run 'TestQuota|TestTask44[7-9]|TestTask45|TestBug51[1-7]|TestBUG51'`
— green (6.5s), covering BUG-511 blocked-pin admission, Task-447 same-provider
claims, Task-448 cross-provider enumeration, Task-449 gate outcomes, and
Task-450 route cards.

## Provider parity

Provider-agnostic: the change touches the shared `finishTurn` emit path and
the quota ledger/veto — no adapter, event stream, or provider-specific
branch. `ProviderLimitQuotaExhausted` is classified identically for all
providers in `provider_limit.go`.
