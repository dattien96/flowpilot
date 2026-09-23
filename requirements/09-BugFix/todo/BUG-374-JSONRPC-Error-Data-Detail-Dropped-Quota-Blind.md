# BUG-374: JSON-RPC error.data detail dropped — quota errors surface as bare "Internal error"

## Metadata

- Document ID: `BUG-374`
- Title: `JSON-RPC error.data detail dropped — Grok 402 quota surfaces as "Internal error", quota classifier blind`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-09-23`
- Last Updated: `2026-09-23`
- Feature Keys: `ai-providers`
- Parent Documents: CP-84 (surfaced during live multi-lane validation)
- Child Documents: `none`
- Related Documents: BUG-361 (OpenCode quota shapes), `isProviderUsageLimitError`
- Replaces: `none`
- Tags: `grok, quota, json-rpc, turn-failed, account-switch`

## AI Quick View

### Summary

- Live `run-1770057` (2026-09-23, Grok Build): account out of balance → ACP `session/prompt` replies
  `{"error":{"code":-32603,"message":"Internal error","data":{"http_status":402,"message":"API error (status 402 Payment Required): Grok Build usage balance exhausted"}}}`.
- `jsonRpcErrorMessage` kept only `error.message` → surfaced `turn_failed` error was literally `"Internal error"`. Every quota token lived in the dropped `error.data` object.
- Consequence: `isProviderUsageLimitError` returned false → run failed as a generic error; no `pendingAccountSwitch` modal on the focused run, no quota `DecisionPayload` on the lane. Silent provider failure — same failure silhouette as the CP-81 "app randomly kicks me out" class.
- Fix: `jsonRpcErrorMessage` now appends `error.data` detail (`data.message`/`details`/`detail`/`reason` string, or plain string data) when it adds information. `"Internal error: API error (status 402 Payment Required): …"` → classifier matches `payment required` → account-switch flow fires.

### Constraints

- safe-fix-contract: additive helper change only; existing message-only shapes byte-identical; no old-test edits.
- cross-provider-parity Case 2: shared helper, per-adapter call sites — all four call sites (`grok_process.go`, `codex_appserver.go`, `devin_process.go`, `opencode_process.go`) verified to wire identically (`fmt.Errorf("%s", jsonRpcErrorMessage(...))`); helper takes no providerKey. Claude uses stream-json (no JSON-RPC transport) — structurally cannot reach this path. Gemini exercises the helper in tests only.

## 3. Environment and Reproduction

- environment: macOS, runner `:4317`, Grok account with exhausted Build balance.
- reproduction:
  1. `POST /client/workflow-runs` (providerKey `grok`), then `POST …/turns`.
  2. Observe `[turn-failed] … provider=grok error="Internal error"` while the raw ACP frame carries the 402 detail in `error.data`.
- frequency: every Grok RPC error whose detail lives in `data` (quota is the observed one).

## 4. Expected vs Actual

- expected: turn_failed carries the provider's real error text; quota tokens reach `isProviderUsageLimitError`; account-switch decision/modal fires.
- actual: `turn_failed error="Internal error"`; generic failure; no quota routing.

## 5. Root Cause

`sessions.go jsonRpcErrorMessage` extracted `error.message` and ignored `error.data` entirely (JSON-RPC spec: `data` is "additional information about the error").

## 6. Fix

- `internal/runner/sessions.go`: `jsonRpcErrorMessage` appends `": " + detail` when `jsonRpcErrorDataDetail(errObj["data"])` yields a non-empty string not already contained in `message`.
- New `jsonRpcErrorDataDetail`: handles `data` as string or object (`message`/`details`/`detail`/`reason` keys).

## 7. Verification

- `internal/runner/bug374_jsonrpc_error_data_dropped_test.go` (4 tests, red→green):
  - live wire shape → surfaced text contains `Payment Required` and classifies quota via `isProviderUsageLimitError`;
  - string `data` appended;
  - duplicate `data.message` not double-printed;
  - healthy `data` (http_status only) stays non-quota.
- `TestJsonRpcErrorMessageHandlesGeminiACPShapes` unchanged and green.
- Provider sweep (`-run 'TestBug361|Opencode|Grok|Codex|Gemini|Devin'`): 157s, all pass.

## 8. Provider Parity Evidence

- Helper is provider-agnostic (no `providerKey` branch — grep `jsonRpcErrorMessage` call sites).
- Case 2 verification: all four adapters call it in the identical waiter-error branch; no adapter post-processes the returned string.
- Claude: stream-json transport, no JSON-RPC error path — unaffected by construction.
