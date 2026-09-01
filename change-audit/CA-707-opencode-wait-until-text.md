# CA-707 — Opencode wait-until-text (BUG-341 follow-up for 424302)

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: BUG-341
change_type: bugfix
summary: extend opencode post-result drain to wait until text arrives (up to 8s) so tool-heavy first turns are not blank (424302)
# --->8---

## What changed

- `opencode_adapter.go` `RunPrompt` post-result path: after `session/prompt` returns, drain already-queued notifs, then:
  - if `lastText != ""` (already have deltas) → `drainBlocking(ctx, 150ms)` just for burst tail (no 600ms tax).
  - if `lastText == ""` and `result["text"]` has text → use it (no wait).
  - if `lastText == ""` and no result text → `drainBlocking(ctx, 8s)` which waits for the first `agent_message_chunk` + 150ms quiet. The previous 600ms timeout was too short for a 2–5s generation after a 3-tool burst (600ms → blank until next prompt's replay). Now 424302's late chunk is captured and `emitTerminal` carries `FinalMessage` so `turn_completed` renders immediately. Added log when waited >200ms.
- `drainOpencodeNotificationsBlocking` now takes `ctx` and uses a `time.Timer` (150ms quiet reset) — already added in CA-706, timeout is now caller-supplied (150ms vs 8s).

## R1 evidence

- New additive tests (no old edits), all PASS:
  - `TestOpencodeLateChunkAfter2sIsCaptured` — 2s late chunk with 8s wait is captured (≈2.15s), proves 600ms was insufficient.
  - `TestOpencodeNoWaitWhenTextAlreadyPresent` — when text already present, wait is ~150ms, not 8s (0.15s), proves no tax on normal turns.
  - Existing `TestOpencodeLateChunkIsCaptured` (80ms) still PASS (0.23s).
- Old suites untouched and green:
  - `go test ./internal/runner -run TestOpencode -count=1` PASS (7s, no legacy edit).
  - `go test ./internal/tui/app -count=1` 7.05s PASS (CA-705 `turnLive` still green, `TestTurnStreamClosed_ClearsStrandedThinking` still PASS).
- Provider parity: opencode-only adapter path; TUI `turnLive` (CA-705) is provider-agnostic and already verified over `grok/codex/claude/opencode`. No other adapter changed, so agnostic proof holds.

## Honest gaps

- TUI empty-terminal hydrate fallback (fetch `GET /events?afterSeq` on blank `turn_completed`) is still TODO per plan — the runner-side wait makes it unnecessary for 424302, but it would be a safety net for any future adapter that still emits empty terminal.

## Prior CA not undone

- `CA-706` 600ms blocking drain and `CA-705` TUI `turnLive` + C2 guard remain; this extends CA-706's timeout to be conditional (150ms vs 8s) and adds result-text fast-path.
- `CA-696..CA-702` chat-history contracts untouched.
