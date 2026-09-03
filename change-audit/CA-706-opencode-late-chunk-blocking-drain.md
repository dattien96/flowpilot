# CA-706 — Opencode late chunk blocking drain (BUG-341 follow-up)

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: BUG-341
change_type: bugfix
summary: capture late agent_message_chunk that arrives just after session/prompt result via blocking drain so first turn is not blank (421135)
# --->8---

## What changed

- `opencode_adapter.go` `RunPrompt` post-result path: after the non-blocking `drainOpencodeNotifications`, call `drainOpencodeNotificationsBlocking(ctx, ..., 600ms)` which waits for late `agent_message_chunk` (tool burst + 3 deltas). It resets a 150ms quiet timer on each chunk so a burst finishes, but returns quickly when quiet. The late chunk's `EventMessageDelta` is emitted via the bridge and its text becomes `lastText` → `emitTerminal` → `turn_completed.FinalMessage` is no longer empty.
- New additive test `opencode_late_chunk_test.go` `TestOpencodeLateChunkIsCaptured` (80ms late chunk, table not needed — single provider path, but the TUI `bug341_turnlive_blank_test.go` already covers provider-agnostic TUI side over `grok/codex/claude/opencode`).

## R1 evidence

- New test `TestOpencodeLateChunkIsCaptured` 0.23s PASS — non-blocking drain misses the 80ms late text, blocking drain captures it and the bridge sees the delta.
- Old suites: `go test ./internal/runner -run TestOpencode -count=1` PASS (all 20+ opencode tests), `go test ./internal/tui/app -count=1` 7.05s PASS (no edits).
- Provider parity: opencode-only path; the TUI `turnLive` fix in `CA-705` is provider-agnostic and already verified over the 4 providers. No other adapter changed.

## Honest gaps

- None for opencode; the TUI `turnLive` + C2 guard remain as in `CA-705`.

## Prior CA not undone

- `CA-705` TUI `turnLive` + C2 guard and `CA-696..CA-702` chat-history contracts remain. This is a complementary runner-side fix for the same blank (421135) — the TUI now waits for the terminal via orch, the runner now ensures the terminal carries the text.
