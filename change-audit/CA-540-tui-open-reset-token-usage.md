# CA-540 — /open must not leak the previous chat's ctx/token usage

## Problem

Switching chats with `/open` kept the previously-opened chat's context/token
usage in the status line: chat A showed `ctx 23% remain · 46.1k/200k …`, then
`/open chat-B` still showed those figures. `m.lastTokens` is session RAM that
`/new` resets but `ChatOpenedMsg` never cleared, and `replayHistoryMessages`
dropped the runner's per-run `token_usage_updated` events on replay.

## Fix (TUI-only)

1. `ChatOpenedMsg` gains `TokenUsage *client.TokenUsageSnapshot` — the last
   `token_usage_updated` replayed for that run (nil when the run never emitted
   one).
2. `lastReplayTokenUsage(evs)` (`history.go`) scans the replayed events for the
   last non-nil `token_usage_updated` snapshot (the runner persists these for
   all providers).
3. `cmdOpenChat` seeds it from the replay.
4. `ChatOpenedMsg` handler (`app.go`) clears `lastTokens` on every open, then
   seeds from the opened run's own usage and updates `modelContextWin` when the
   snapshot carries one — so `/open` of a usage-less chat shows only the bare
   `ctx <window> window` fallback, never another chat's numbers.

## Provider impact

Provider-agnostic (Case 1). `lastReplayTokenUsage` takes no `providerKey` and
never branches on one; the runner emits `token_usage_updated` for all three
providers (claude_event_mapper.go:91,126; codex_event_mapper.go:55;
grok_adapter.go:553) — unchanged by this fix. Tests parameterize
Claude/Codex/Grok anyway.

## additive-tests-only compliance

Only a new test file was added
(`ca540_open_reset_token_usage_test.go`); no existing test file was modified.

## Verification

- `go build ./...` and `go vet ./internal/tui/...` clean; `gofmt` clean.
- New matrix tests green across claude/codex/grok: `/open` without usage clears
  chat A's stale figures; `/open` with a replayed usage seeds chat B's own
  numbers + window; `lastReplayTokenUsage` picks the last event and ignores
  nil snapshots.
- Existing `TestChatOpenedMsg_*` suites (transcript replacement, stepID,
  one-shot steps refresh, cursor seed) untouched and green.
- Full `go test ./internal/tui/app/ -count=1` green except the two pre-existing
  environmental `TestCmdFocusAgent_*` network failures (confirmed pre-change).

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CA-540
change_type: bugfix
summary: /open now resets per-run token usage instead of leaking the previous chat's ctx/token numbers — ChatOpenedMsg clears lastTokens on every open and seeds from the opened run's own last token_usage_updated replay event (lastReplayTokenUsage), so a usage-less chat shows only the model-window fallback.
# --->8---