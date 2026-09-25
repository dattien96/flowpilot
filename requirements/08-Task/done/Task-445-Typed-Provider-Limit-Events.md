# Task-445: Typed Provider-Limit Events

- Document ID: `Task-445`
- Title: `Normalize Claude/Codex/Grok/OpenCode/Devin limit failures once and remove Desktop string classification`
- Phase: `task`
- Status: `done`
- Owner: `dat.nguyen`
- Reviewers: ``
- Created: `2026-09-25`
- Last Updated: `2026-10-04`
- Parent Documents: `CP-87`, `CP-86`, `SS-22`
- Child Documents: ``
- Related Documents: `BUG-361`, `BUG-374`, `Task-434` (quota attention routing)
- Replaces: ``
- Tags: `quota`, `provider-events`, `provider-parity`, `error-taxonomy`

## AI Quick View

### Summary

- Go and Desktop currently maintain duplicate quota string lists. Known misses
  already caused retry/hang and lost billing details.
- Add one normalized limit taxonomy/event at provider ingress. Structured
  provider fields/stop reasons win; one centralized string fallback remains
  only for unstructured CLI failures and carries `confidence: heuristic`.
- Claude/Codex are fixture-contract tested (no live accounts); Grok/Devin can
  add live non-destructive evidence; OpenCode uses fake ACP plus optional free
  success path.

### Current Ask

- Introduce `ProviderLimit` + `EventProviderLimitReached`, map known shapes for
  five providers, make send retry depend on typed kind, and remove
  `store.ts:isUsageLimitMessage` after typed-event UI parity lands.

### Key Decisions

- `T-1` Kinds remain distinct: `quota_exhausted`, `rate_limited`,
  `credits_exhausted`, `billing_required`; do not lump transient 429 into an
  exhausted account.
- `T-2` Detection source/confidence are required audit fields:
  `structured_payload|stop_reason|stderr_fallback` and `exact|heuristic`.
- `T-3` Retry policy: Retry-After rate limit is safely retryable/bounded;
  exhausted/credits/billing are non-retryable and route to CP-87 preflight.

### Constraints

- Sanitized fixtures only; no secrets or real home paths.
- Gemini excluded. Provider matrix: Claude/Codex/Grok/OpenCode/Devin.
- Unknown shapes remain generic failures and emit unclassified audit evidence;
  never silently become quota or retry forever.

### Open Questions

- None blocking.

### Source Refs

- `CP-87 P-1`, `interactive_service.go:isProviderUsageLimitError`,
  `claudeUsageLimitMessage`, `opencodeIsQuotaStopReason`,
  `devinIsQuotaStopReason`, `store.ts:isUsageLimitMessage`.

## 1. Goal

One provider-limit contract from adapter to UI, with explicit evidence quality
and correct transient-vs-exhausted action.

## 2. Parent Links

- coding plan: `CP-87 P-1`
- tech design: `SD-17`
- system spec: `SS-22`
- specific upstream ids: `BUG-361`, `BUG-374`, AGENTS §5

## 3. Trigger

Duplicated string parsing drifts and turns new provider wording into generic
failure, wrong retry, or missing account-switch UX.

## 4. Exact Change

- `T-1` Add `ProviderLimitKind`, `ProviderLimit`, event enum/payload.
- `T-2` Provider mappers return typed limits from structured payloads/stop
  reasons; shared fallback classifier is runner-only.
- `T-3` `isRecoverableSendError` consumes typed error/kind; transient
  rate-limit honors bounded Retry-After.
- `T-4` Desktop consumes typed event and deletes the parallel classifier.
- `T-5` Fixture + fake process/ACP/app-server end-to-end tests for five
  providers with evidence labels.

## 5. Touched Areas

- files: provider event/mappers/adapters, `interactive_service.go`, Desktop
  client DTO + `store.ts`, sanitized provider fixture files/tests
- modules: `runner`, Desktop state
- routes: existing event stream only
- tables: none

## 6. Code Guide Signatures

```go
// internal/runner/provider_event.go
type ProviderLimitKind string
const (
    ProviderLimitQuotaExhausted ProviderLimitKind = "quota_exhausted"
    ProviderLimitRateLimited ProviderLimitKind = "rate_limited"
    ProviderLimitCreditsExhausted ProviderLimitKind = "credits_exhausted"
    ProviderLimitBillingRequired ProviderLimitKind = "billing_required"
)
type ProviderLimit struct {
    Kind ProviderLimitKind `json:"kind"`
    ProviderKey ProviderKey `json:"providerKey"`
    AccountID string `json:"accountId,omitempty"`
    RetryAfterSeconds int64 `json:"retryAfterSeconds,omitempty"`
    ResetAt string `json:"resetAt,omitempty"`
    RawCode string `json:"rawCode,omitempty"`
    SanitizedMessage string `json:"sanitizedMessage"`
    DetectionSource string `json:"detectionSource"`
    Confidence string `json:"confidence"`
}
```

```go
// internal/runner/provider_limit.go
func classifyProviderLimit(provider ProviderKey, payload any, err error) (*ProviderLimit, bool)
func providerLimitRecoverable(limit ProviderLimit) bool
```

## 7. Test Signatures

- `TestTask445_ClaudeFixtures`
- `TestTask445_CodexFixtures`
- `TestTask445_GrokFixtures`
- `TestTask445_OpenCodeFixtures`
- `TestTask445_DevinFixtures`
- `TestTask445_RateLimitDistinctFromQuota`
- `TestTask445_UnknownShapeAudited`
- `test("desktop consumes typed provider_limit_reached without parsing message")`

## 8. Acceptance Check

- A typed fixture failure reaches persisted event + focused/non-focused UX with
  no client string parsing; evidence records live vs fixture-contract.

## 9. Out of Scope

- Candidate selection/rotation (Task-447+).
- Intentionally exhausting live accounts.

## 10. Definition of Done

- [x] §6 signatures landed or deviation documented
- [x] §7 additive tests green; old tests not weakened
- [x] Five-provider fixture parity; live evidence accurately labeled
- [x] Desktop quota string classifier removed
- [x] CA ledger + feature keys `token-usage`, `runtime-intelligence`
- [x] GitNexus detect_changes reviewed before commit

## 11. Completion Notes

- result: `ProviderLimit`/`ProviderLimitKind`/`EventProviderLimitReached`
  landed; `classifyProviderLimit` + `providerLimitRecoverable` in new
  `provider_limit.go` are the single classification seam — structured payload
  fields (rpc code/http_status/retry_after/typed code) and stopReasons
  classify `exact`, free-text tokens classify `heuristic` under
  `structured_payload` or `stderr_fallback`. Dispatchers return `*rpcError`
  preserving JSON-RPC code/data (message byte-identical); adapters emit typed
  `provider_limit_reached` before `turn_failed` (opencode/devin in-adapter for
  non-retryable kinds; claude in `mapClaudeResult`; grok/codex/unknown via
  `finishTurn` on `providerLimitAwareError`); `finishTurn` backfills
  provider/account from the run. `isRecoverableSendError` prefers typed
  limits; only `rate_limited` with bounded Retry-After (≤30s) retries, via a
  ctx-aware test seam. Desktop `contract.ts`/`store.ts` consume the typed
  event (`surfaceProviderLimitForRun`); `isUsageLimitMessage` + its mirror
  test deleted. Deviations from §6: none to the signatures themselves —
  `DetectionSource`/`Confidence` are `string` fields; `classifyStopReasonLimit`
  and `providerLimitAwareError` added as helper seams.
- tests: `task445_provider_limit_test.go` —
  `TestTask445_ClassifierKindsAndSources`, `_RateLimitDistinctFromQuota`,
  `_UnknownShapeAudited`, `_ClaudeFixtures`, `_CodexFixtures`,
  `_GrokFixtures`, `_OpenCodeFixtures`, `_DevinFixtures` green;
  `go test ./internal/runner/` green incl. BUG-361/374/381 + send-retry
  regressions. Desktop `providerLimit.test.ts` 3/3: typed event opens switch
  surface (focused), drives inbox decision (non-focused), untyped
  `turn_failed` text no longer triggers.
- evidence labels: Claude/Codex = fixture-contract (no live accounts);
  Grok/OpenCode/Devin = fake ACP/app-server processes over the real
  dispatcher path; **no live accounts exhausted** (per §9).
- follow-ups: CP-87 Task-447+ consumes `ProviderLimit`+`accountId` for the
  routing gate; CA-977.
- upstream docs updated: CP-87 P-1 rollout step 1 satisfied; ledger dominant
  key `ai-providers` per CP-87 commit convention (task DoD keys
  `token-usage`/`runtime-intelligence` noted in CA-977 prose per SS-13).
