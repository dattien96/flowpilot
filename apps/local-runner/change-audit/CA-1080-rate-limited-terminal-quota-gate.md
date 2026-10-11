# CA-1080 — terminal rate_limited enters quota gate (BUG-661)

## Scope
Provider-limit terminal handling in `finishTurn`, the adapter-emitted
terminal seam in `turnBridge.Emit`, `resolveQuotaGate` demand mapping,
and `quotaBlockLiftedOnHeadroom` telemetry lift. Bounded in-turn retry
(Task-445) untouched.

## Defect
A `rate_limited` limit that survived (or was ineligible for) the bounded
in-turn retry reached terminal `failed` instead of the quota gate —
live run-644825 reproduced twice (10-min reset on Task-124; the same
class killed the run at 05:59). Two delivery paths existed, and the old
code covered only one:

- **Path A (typed error):** the send path returns a `providerLimitError`;
  `finishTurn(err != nil)` switched on hard kinds
  (quota/credits/billing) only — `rate_limited` fell through to plain
  terminal failure.
- **Path B (adapter-emitted):** Devin/OpenCode emit
  `EventProviderLimitReached` + `EventTurnFailed` and return `nil`, so
  `finishTurn`'s error branch never ran at all — no gate, no account
  block, run terminal `failed`.

## Fix
- `provider_limit.go`: new `providerLimitGateEligible(kind)` — the shared
  second-tier predicate (quota_exhausted, credits_exhausted,
  billing_required, rate_limited). `providerLimitRecoverable` unchanged:
  bounded Retry-After still retries in-turn first.
- `interactive_service.go`: `interactiveRun` records
  `lastProviderLimit`/`lastProviderLimitTurnID` in `emitLocked` on
  `EventProviderLimitReached`. `turnBridge.Emit`, on a subsequent
  `EventTurnFailed` for the SAME turn with a gate-eligible remembered
  limit, durably blocks the account and invokes `enterQuotaGate` after
  terminal settlement — covering Path B without double-firing Path A.
  The remembered state clears on consume in the seam, and Path A's gate
  branch also consumes it — a future hybrid adapter that both emits the
  bridge failure AND returns a typed error cannot double-fire
  (review round).
- `finishTurn` error branch now keys on `providerLimitGateEligible`, so
  terminal `rate_limited` joins the quota gate (Path A).
- `quota_gate.go`: `resolveQuotaGate` maps every gate-eligible kind —
  including `rate_limited` — into `demand.ObservedLimit`.
- `quota_preflight.go`: `quotaBlockLiftedOnHeadroom` lifts a
  `rate_limited` block on fresh exact healthy/low-headroom telemetry,
  matching `quota_exhausted` semantics; `billing_required`/
  `credits_exhausted` still require operator action; stale/unknown
  telemetry fails closed.

## Provider parity
All adapters normalize into `ProviderLimit` via the shared
`classifyProviderLimit`; the gate decision keys on the normalized kind,
not the provider.

- Codex / Grok: typed-error path via `providerLimitAwareError`
  (codex_adapter.go:203,273; grok_adapter.go:398) → Path A.
- Claude: `claudeLimitError` + `claude_event_mapper` emission → both
  paths.
- Devin / OpenCode: `EventProviderLimitReached` + `EventTurnFailed` +
  `nil` return → Path B (the bridge seam).

## Tests
- `bug661_long_rate_limit_gate_test.go` (new): RED → GREEN.
  Adapter-emitted rate-limit enters the gate; finishTurn rate-limit
  enters the gate; exhausted bounded retry enters the gate; the
  rate-limited account is durably blocked. Invariants (bounded retry
  still retries in-turn; non-limit failures unchanged) green.
- Existing Task-445/447/449, quota-gate, and provider-limit suites —
  untouched and green.
- Full package suite: failures are environmental (missing `claude`/
  `codex` binaries, live MCP/Supabase fetches, TempDir cleanup races)
  or pre-existing (`TestReconstructResume*` fail identically on clean
  baseline — verified via stash).
