---
id: CA-533
feature_key: cli-tui
title: Animated opencode/Grok-style thinking placeholder
date: 2026-08-17
status: COMPLETE
---

## Problem

The TUI chat "Thinking" row was a static italic `thinking…` placeholder
(`styleThinking`) under the prompt. Opencode / Grok show a live spinner with a
moving highlight and elapsed seconds, which reads far better while waiting on a
long reasoning turn.

## Fix (TUI-only, render-time)

The thinking placeholder message keeps its stored content (`"thinking…"`) so
message-state logic and legacy tests are untouched; **only rendering is
animated**. A dedicated `thinkingFrame` drives spinner glyph, label shimmer and
elapsed time, so the row-cache signature only needs the frame to invalidate.

Visual (chat row + status line):

```
⠋ Thinking  1.4s        ← braille spinner, accent shimmer window sweeps label, elapsed
| Thinking  0s           ← ascii fallback (legacy console)
```

1. **`thinking_anim.go` (new)** — pure render helpers: `renderThinkingLine`
   (chat row with spinner + 3-char accent shimmer window sweeping left→right
   across the label, dim-italic outside the window), `thinkingLabelText` (plain
   status-line twin), `thinkingSpinner`, `formatThinkingElapsed` (`0s` →
   `1.4s` → `12s` → `1m 04s`). Braille spinner `⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏`, ascii `|/-\`.
2. **`model.go`** — `thinkingFrame`, `thinkingTickerActive`, `thinkingTickMsg`.
3. **`app.go`** — `cmdThinkingTick` (90ms) + `thinkingTickMsg` case (advances
   frame, self-cancels when the row is gone). The always-on cursor tick starts
   the thinking ticker once a thinking placeholder is live (any creation site:
   prompt send, high-reasoning stream, replay) and re-arms only per thinking
   phase. `buildChatRows` special-cases `FormatHint=="thinking"` to emit the
   animated row (no copy chip, left-aligned — same placement as before).
   `chatRowsSig` hashes `thinkingFrame` while a thinking row is live so the row
   cache never freezes the first frame. `addMessage` resets `thinkingFrame=0`
   on a fresh placeholder so each turn's elapsed restarts.
4. **`status_bar.go`** — `statusReadyLabel` returns the animated plain label
   while a thinking row is live; `statusMsg` itself stays `"thinking…"`.

## Provider impact

Provider-agnostic (Case 1): all touched logic reads message `FormatHint` +
`thinkingFrame`, never `providerKey`. The render/tick/cache tests parameterize
Claude / Codex / Grok.

## Tests

New additive file `app/thinking_anim_test.go`:

- `TestFormatThinkingElapsed` — 0s / 1.4s / 9.9s / 10s / 12s / 1m 04s etc.
- `TestThinkingLabelText_SpinnerAdvancesAndWraps` × claude/codex/grok.
- `TestThinkingLabelText_AsciiSpinner` — `|/-\` + wrap.
- `TestThinkingLabelText_ElapsedRisesWithFrame` — frame→seconds coupling.
- `TestThinkingShimmer_WindowSweeps` — accent segment count: 4 (spinner + 3
  label chars) inside the window, 1 when the window is past the label; visible
  label/elapsed unchanged while the highlight moves (forced TrueColor).
- `TestThinkingRow_RendersAnimatedInView` × claude/codex/grok — View shows the
  spinner + "Thinking" + "0s".
- `TestThinkingRow_OnlyAnimatedForThinkingHint` — a normal assistant message is
  never animated (near-miss guard).
- `TestThinkingTick_AdvancesAndSelfStops` — tick advances the frame, reschedules
  while live, self-stops after `replaceThinkingAt`.
- `TestThinkingTick_ResetsOnFreshPlaceholder` — follow-up turn restarts elapsed.
- `TestChatRowsSig_ThinkingFrameInvalidates` × claude/codex/grok — cache
  re-renders when the frame advances.
- `TestChatRowsSig_StableWithoutThinkingRow` — frame does not churn the cache
  when no thinking row is live (guards the negative case).
- `TestStatusReadyLabel_ShowsAnimatedThinking` — status line animates only with
  a live row; static `"thinking…"` otherwise.

## Verification

- `go build ./...` clean; `go vet ./internal/tui/app/` clean.
- gofmt clean (checked on LF-normalized copies — this checkout is CRLF repo-wide,
  untouched files are flagged identically).
- `go test ./internal/tui/... ./internal/cli/... -count=1` green except two
  **pre-existing** network-flaky tests in `tui_child_open_resume_fallback_test.go`
  (`TestCmdFocusAgent_ResumeRetriedOnceOnDialError`,
  `TestCmdFocusAgent_TotalFailureNoStuckChrome` — `wsarecv: connection forcibly
  closed`). Confirmed identical failure on the clean baseline (changes stashed).
- Related legacy thinking tests untouched and green:
  `bug324_thinking_placeholder_test.go`, `turn_stream_closed_thinking_test.go`,
  `tui_flow_chrome_quiet_test.go`, `tui_tool_group_collapse_test.go`,
  `chat_ux_actions_test.go`, `bugfix_regression_test.go`.
- `-race` not runnable on this box (no gcc for cgo).

## Out of scope / residual

- Desktop `Timeline.tsx` untouched — its `Thinking...` row stays as-is; this
  covers the TUI client only (per request).
- GitNexus MCP tooling unavailable in this session; impact/scope verified via
  local code search + diff review (same caveat as CA-059).
- Spinner cadence fixed at 90ms; not configurable.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CA-533
change_type: feature
summary: Animated opencode/Grok-style thinking placeholder in the TUI chat row + status line
# --->8---