# CA-977 — Task-445: typed provider-limit events

## Summary

Provider quota/rate-limit/credits/billing failures now classify once at the
adapter boundary into a typed `ProviderLimit` and ride a persisted
`provider_limit_reached` event, replacing three duplicated string-token lists
(runner `isProviderUsageLimitError`, `opencodeIsQuotaStopReason` /
`devinIsQuotaStopReason` bodies, desktop `isUsageLimitMessage`).

## What changed

- `provider_event.go` — `EventProviderLimitReached` (`provider_limit_reached`),
  `ProviderLimitKind` (quota_exhausted | rate_limited | credits_exhausted |
  billing_required), `ProviderLimit` payload (kind/provider/account/retryAfter/
  resetAt/rawCode/sanitizedMessage/detectionSource/confidence), and the
  `ProviderEvent.providerLimit` field.
- `provider_limit.go` (new) — `classifyProviderLimit(provider, payload, err)`:
  structured payload fields (http_status 402/429, typed code/type, retry_after)
  and stopReasons classify `exact`; free-text token matches classify
  `heuristic` under `structured_payload` (provider envelope) or
  `stderr_fallback` (plain CLI error text). `providerLimitRecoverable` —
  only `rate_limited` with a bounded Retry-After (≤30s) is retryable; every
  other kind is terminal. `providerLimitError` carries the typed limit on
  adapter-returned errors; `providerLimitAwareError` wraps+audits
  unclassified failures. `newRPCError` preserves JSON-RPC code/data that
  dispatchers previously flattened (message text byte-identical).
- Dispatchers (`grok_process.go`, `opencode_process.go`, `devin_process.go`,
  `codex_appserver.go`) return `*rpcError` instead of a flattened string.
- Adapters — opencode/devin `SendTurn` RPC-error paths classify; non-retryable
  limits emit `provider_limit_reached` + existing `turn_failed` copy, retryable
  rate-limits return the typed error into `sendTurnWithRetry`. Their
  `emitTerminal` stopReason paths emit the typed event via
  `classifyStopReasonLimit`. Grok/codex wrap errors via
  `providerLimitAwareError`; `finishTurn` emits `provider_limit_reached` before
  `turn_failed` for any limit-shaped terminal error (typed or fallback).
- `claude_event_mapper.go` — `mapClaudeResult` emits the typed event before
  `turn_failed` when the result payload classifies (limit turn_failed now
  `Recoverable:false` — a surfaced limit is terminal by construction).
- `sessions.go` — legacy claude print-path limit errors wrapped as typed
  `providerLimitError` via `claudeLimitError` (stderr → fallback, parsed
  result → structured_payload).
- `interactive_service.go` — `isProviderUsageLimitError` is now a thin wrapper
  over `classifyProviderLimit` (name preserved for callers/tests);
  `isRecoverableSendError` prefers the typed error and honors
  `providerLimitRecoverable`; `sendTurnWithRetry` waits the bounded
  Retry-After (`providerLimitRetryDelayFn`, ctx-aware) before re-sending.
- Desktop — `contract.ts` gains `ProviderLimitDTO` + `provider_limit_reached`
  variant; `store.ts` surfaces the quota account-switch modal/inbox decision
  from the typed event via exported `surfaceProviderLimitForRun`;
  `isUsageLimitMessage` and its mirror test deleted (spec: remove the
  parallel classifier).

## Evidence

- New tests: `task445_provider_limit_test.go` — classifier kind/source/
  confidence matrix, rate-limit-vs-quota distinction, bounded Retry-After
  retry honoring, unclassified-shape audit, plus fixture/fake-ACP e2e for
  Claude (result frames), Codex (fake app-server RPC error), Grok (fake ACP
  402 with `data.http_status`), OpenCode (stopReason), Devin (stopReason).
- Desktop: `providerLimit.test.ts` — typed event opens the switch surface with
  deliberately unparseable wording; untyped `turn_failed` no longer triggers;
  non-focused run lands the inbox quota decision.
- Parity: Claude/Codex = fixture-contract (no live accounts on this machine);
  Grok/OpenCode/Devin = fake ACP/app-server processes over the production
  dispatcher path. No live accounts were intentionally exhausted.
- Regression: `TestBug361*`, `TestBug374*`, `TestBug381*`, `TestSendTurn*`,
  provider adapter suites green — token coverage unchanged on the legacy
  `isProviderUsageLimitError` surface.

## Follow-ups

- CP-87 Task-447+ consumes `ProviderLimit` + `accountId` for quota routing;
  the event is the durable admission signal.

Note: Task-445 §10 names feature keys `token-usage` + `runtime-intelligence`;
the ledger block carries the dominant key `ai-providers` (CP-87 convention, as
with CP-86's two-key DoD resolving to dominant `token-usage`). The typed event
feeds both the token-usage observability surface and runtime-intelligence
routing.

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: Task-445
change_type: feature
summary: typed provider_limit_reached events classify quota/rate-limit/credits/billing once at the adapter boundary (5 providers); retry honors bounded Retry-After; desktop quota surface consumes typed event, string classifier deleted
# --->8---
