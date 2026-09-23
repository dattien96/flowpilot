# BUG-381: Provider quota/auth errors collapse to bare "Internal error" — real reason dropped with `error.data`, misleading 3× retries

## Metadata

- Document ID: `BUG-381`
- Title: `jsonRpcErrorMessage drops error.data (http_status/message); flattened errors defeat isProviderUsageLimitError → deterministic billing failures retried 3×`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-21`
- Parent Documents: [CP-46-Grok-Build-Controlled-Adapter-Over-ACP](../../07-Coding-Plan/done/CP-46-Grok-Build-Controlled-Adapter-Over-ACP.md), [CP-57-Opencode-Provider-Integration](../../07-Coding-Plan/done/CP-57-Opencode-Provider-Integration.md), [CP-70-Devin-Provider-Integration](../../07-Coding-Plan/done/CP-70-Devin-Provider-Integration.md)
- Feature Keys: `ai-providers, workflow-runtime`

## AI Quick View

### Summary

- ACP `session/prompt` JSON-RPC errors carry the informative payload in `error.data` (`{"http_status":402,"message":"API error (status 402 Payment Required): Grok Build usage balance exhausted"}`) while top-level `error.message` is just `"Internal error"`. `jsonRpcErrorMessage` (`apps/local-runner/internal/runner/sessions.go:266-279`) reads ONLY `error.message` → the real reason never reaches the turn error.
- Consequence chain: flattened `"Internal error"` → `isProviderUsageLimitError` (`interactive_service.go:8397-8439`) cannot match its quota token list → `isRecoverableSendError` (:8378) → `sendTurnWithRetry` (:8357) burns all 3 attempts with user-facing `[recovering: re-sending turn after a recoverable error (attempt N/3)]` deltas → `turn_failed "Internal error"`; `.flowpilot/chats/sessions.ndjson` persists `last_message:"Internal error"`.
- Second layer seen on opencode (CP-57-1): even where the message survives unflattened (`"Internal error: Upstream request failed: An active OpenCode Go subscription is required to use Go models."`), `isProviderUsageLimitError` has no `subscription is required`/`subscription required` token → same 3× ~3.5 min stall.
- Three live confirmations: CP-46 (grok 402, every turn), CP-57 (opencode-go entitlement), CP-70 R9 (grok 402 again).

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.
- Fix directions noted by testers: (a) unwrap `error.data.message`/`error.data.http_status` in `jsonRpcErrorMessage` (grok's `_x.ai/session/prompt_complete` `agentResult` also carries the full text); (b) add `"subscription is required"`/`"subscription required"` to `isProviderUsageLimitError`; (c) optional pre-flight: catalog marks `opencode-go/*` as Go-subscription-gated → reject at model-switch time.

## Bug report

- **Symptom**: Quota/auth failures surface to the user/run record as generic `turn_failed "Internal error"` after ~3 wasteful retries (~70 s each on opencode); the real `402 Payment Required`/entitlement reason is visible only in runner.log.
- **Expected**: `turn_failed` immediately with the provider's real message, `recoverable:false` — same fast-fail path as "rate limit"/"payment required" classifications.
- **Actual**: cp46 run-1 — turns turn-3/turn-8 each retried 3×, `last_message:"Internal error"`; cp57 run-1 turn-15 — `opencode-go/deepseek-v4-flash` on a subscription-less account retried 3× over ~3.5 min (05:45→05:48) before `turn_failed`; cp70 run-5524 — grok 402 → `turn_failed{"error":"Internal error"}` after 3 retries.
- **Impact**: high — billing/auth failures are indistinguishable from transient crashes; users get misleading "[recovering]" UX, runs waste minutes of retries on permanent errors, and any quota-based UX (remaining_7d, fast-fail) is bypassed.

## Reproduction

1. Grok (cp46 run-1 / cp70 run-5524): provider `grok`/`grok-4.5` on a quota-exhausted account (`remaining_7d_percent:0`, billing API HTTP 200). Send any chat turn → each `session/prompt` returns `{"error":{"code":-32603,"data":{"http_status":402,"message":"API error (status 402 Payment Required): Grok Build usage balance exhausted"},"message":"Internal error"}}` → 3 attempts → `turn_failed "Internal error"`.
2. Opencode (cp57 run-1 turn-15): chat run on `opencode/muse-spark-1.2-contributor-free`, then turn with `model:"opencode-go/deepseek-v4-flash"` on an account WITHOUT OpenCode Go subscription → `session/prompt` -32603 `"Internal error: Upstream request failed: An active OpenCode Go subscription is required to use Go models."` → attempts 2/3, 3/3 → `turn_failed` 05:48:45. Control turn-23 on `opencode/big-pickle` completed fine (memory intact).

## Root cause

- `apps/local-runner/internal/runner/sessions.go:266-279` — `jsonRpcErrorMessage` extracts only `error.message` (string); never unwraps `error.data.message` / `error.data.http_status` where ACP servers put the informative payload.
- `apps/local-runner/internal/runner/interactive_service.go:8397-8439` — `isProviderUsageLimitError` token list lacks `"subscription is required"`/`"subscription required"`; even an unflattened opencode-go entitlement message classifies recoverable → `isRecoverableSendError` (:8378) → `sendTurnWithRetry` (:8357) burns `maxTurnAttempts` (3).
- Existing regression coverage (`TestIsProviderUsageLimitErrorBaseRegressionPlusGrok402`) cannot see the 402 once the error is flattened upstream.

## Evidence

- `~/fp-beds/lt-evidence/cp46/BUG-LIVE-CP46-01-grok-402-masked-as-internal-error.md`, `runner.log` L281 (raw error frame with `data.http_status:402`), L282 (`[turn-failed] … error="Internal error"`), L255-256/L276-279 (`_x.ai` frames carrying the full text), `l46-1-run1-events.sse`, `l46-3-run1-resume-events.sse`, `l46-sessions.ndjson` (`last_message:"Internal error"`), `grok-probe-oneshot.txt` (CLI itself returns full 402 payload).
- `~/fp-beds/lt-evidence/cp57/BUG-LIVE-57-1.md`, `bug-live-57-1_log_excerpt.txt` (ACP -32603 at 05:46:19/05:47:28/05:48:45), `l57-3_run1_sse.ndjson` (recovering deltas 2/3, 3/3 → turn_failed).
- `~/fp-beds/lt-evidence/cp70/RESULT.md` (BUG-LIVE-CP70-2), `r9-sse.json`, `r9-grok-402.txt` (log lines 06:43:56).

## Severity

- high

## Completion Notes (implemented 2026-09-23, CA-917)

- Fix: `jsonRpcErrorMessage` (sessions.go) unwraps `error.data.message`, then
  `error.data.http_status`, before falling back to the generic top-level
  `error.message`. Quota/auth failures now surface "API error (status 402
  Payment Required): …" instead of "Internal error".
- Tests: `bug381_jsonrpc_error_data_test.go` (3 cases: data.message,
  top-level fallback, http_status synthesis).
- Shared ACP path — benefits grok (402 evidence), opencode, devin.
