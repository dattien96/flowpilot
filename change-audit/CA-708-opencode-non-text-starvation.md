# CA-708 — Opencode non-text starvation of wait-for-text (BUG-341)

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: BUG-341
change_type: bugfix
summary: only reset wait-for-text timer on actual text growth so usage_update/tool_call_update do not collapse 8s wait (424302/430742 blank)
# --->8---

## What changed

- `opencode_adapter.go` `drainOpencodeNotificationsBlocking`: store `before := lastText`, only `timer.Reset(150ms)` when `lastText != before` (i.e. `EventMessageDelta` grew the answer). Non-text frames (`usage_update` → `EventTokenUsageUpdated`, `tool_call_update` → `EventToolCompleted`, `agent_thought_chunk` → unmapped, `available_commands_update` → unmapped) no longer reset the timer. The 8s wait-for-text budget (CA-707) now actually runs; previously `usage_update` at 10ms reset it to 150ms and the 400ms-late `agent_message_chunk` was dropped, then `unregisterSession` discarded it.
- No change to `drainOpencodeNotifications` (non-blocking) or `emitTerminal` fallback; the outer `if lastText == ""` → 8s vs `else` → 150ms logic from CA-707 is kept.

## R1 evidence

- New additive tests (no legacy edits), all PASS:
  - `TestOpencodeUsageUpdateDoesNotStarveText` — `usage_update` at 10ms then text at 400ms, 8s wait → `Chao Nam` (≈550ms). **Red before fix** (old code reset to 150ms at 10ms → timeout at ~160ms, got `""`).
  - `TestOpencodeToolUpdateDoesNotStarveText` — `tool_call_update` then text, same.
  - `TestOpencodeThoughtChunkDoesNotStarveText` — unmapped `agent_thought_chunk` then text, same.
- Existing tests still green (no edits):
  - `TestOpencodeLateChunkIsCaptured` 0.23s, `TestOpencodeLateChunkAfter2sIsCaptured` 2.15s, `TestOpencodeNoWaitWhenTextAlreadyPresent` 0.15s — all PASS.
  - Full suites: `go test ./internal/tui/app -count=1` 7.3s PASS, `go test ./internal/runner -run TestOpencode -count=1` PASS (no legacy edit, `TestTurnStreamClosed_ClearsStrandedThinking` still PASS).
- Provider parity: opencode-only adapter path. The `wait-for-text` is in that adapter only; Claude/Grok do not use `session/prompt` result vs `session/update` split, so they are unaffected. TUI `turnLive` (CA-705) is provider-agnostic and already verified over `grok/codex/claude/opencode`. Desktop `HttpWsRunnerClient.sendTurn` has the same `providerTurnId` filter and `turn_completed` close as TUI (`client.go:1252` vs `HttpWsRunnerClient.ts:423`), so the runner-side empty-terminal fix covers Desktop without a Desktop code change (same SSE stream, same `turn_completed.FinalMessage`).

## Honest gaps

- TUI empty-terminal hydrate fallback (fetch `GET .../events?afterSeq` when `turn_completed.FinalMessage==""` and no assistant) is still TODO — the runner now ensures the terminal carries text, but a future adapter could still emit empty.
- Opencode `opencodeTextContent` requiring `content.type=="text"` (array vs object, `output_text`) is not yet live-observed; the new tests use the documented `{type:"text",text}` shape.

## Prior CA not undone

- `CA-707` 8s/150ms conditional wait, `CA-706` 600ms blocking drain, `CA-705` TUI `turnLive` + C2 busy guard, and `CA-696..CA-702` chat-history contracts remain. This tightens CA-707's contract from “wait-for-quiet” to “wait-for-text” (only text growth extends the timer).
