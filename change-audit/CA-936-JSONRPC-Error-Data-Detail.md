# CA-936 — BUG-374: keep JSON-RPC error.data detail in surfaced error text

## Summary

Live CP-84 validation surfaced a silent-failure path: Grok's ACP `session/prompt`
returns the quota detail inside `error.data` while `error.message` is the
generic `"Internal error"`. `jsonRpcErrorMessage` discarded `data`, so
`turn_failed` carried no quota tokens, `isProviderUsageLimitError` stayed
false, and neither the focused account-switch modal nor a lane quota decision
fired.

Fix is additive in the shared helper: append `error.data` detail
(`data.message`/`details`/`detail`/`reason` or plain string) to the surfaced
message when it adds new information. Message-only wire shapes are unchanged
byte-for-byte; all four adapter call sites (grok, codex, devin, opencode)
consume the helper identically.

## Verified

- 4 new tests red→green (`bug374_jsonrpc_error_data_dropped_test.go`),
  including the exact live frame
  `{"code":-32603,"message":"Internal error","data":{"http_status":402,"message":"API error (status 402 Payment Required): Grok Build usage balance exhausted"}}`.
- `isProviderUsageLimitError` now classifies the surfaced string as quota.
- Provider sweep (`Opencode|Grok|Codex|Gemini|Devin` + BUG-361): all green.
- Parity: Case 2 — helper takes no providerKey; all call sites identical;
  Claude's stream-json transport cannot reach this path.

## Files

- `internal/runner/sessions.go`,
  `internal/runner/bug374_jsonrpc_error_data_dropped_test.go` (new),
  `requirements/09-BugFix/todo/BUG-374-…md` (new)

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: BUG-374
change_type: bugfix
