---
id: CA-525
feature_key: cli-tui
title: Collapse consecutive tool calls into a click-to-expand summary
date: 2026-08-17
status: COMPLETE
---

## Problem

Every `tool_started` event appends a `→ tool_name` line, so a provider turn that
fans out several sequential tool calls renders N stacked tool lines between two
normal chat responses — noisy, especially in flow runs where the model chains
many tools before answering (see the user's screenshot: six `→ …` lines between
"ly remediation. [copy]" and "Prior walkthrough").

## Fix (TUI-only, render-time)

**Rule:** a *run* is the maximal sequence of `Role=="tool"` messages. Any other
message (user / assistant / thinking / system) breaks it — i.e. grouping counts
tool calls between two normal chat responses. A run of **exactly one** tool
renders directly as before; a run of **2+** renders as a single collapsible
summary row.

- `buildChatRows` scans consecutive tool messages and, for a run of 2+, emits
  `toolGroupRows` instead of the individual lines.
- `toolGroupRows` renders `▸ N tool calls · click` (ascii `+`) when collapsed —
  the default — or `▾ N tool calls · click` (ascii `-`) plus the individual
  `→ tool` lines when expanded.
- State: `expandedToolGroups map[string]bool` on the model. The key is
  `toolGroupKey`, the run's tool names joined with `\x1f` — **content-derived**,
  so it survives thinking-placeholder reordering (`replaceThinkingAt` moves
  messages, which shifts indices).
- `chatRow` gains `ToolGroupKey`; only the summary row carries it, so a click on
  an individual `→ tool` line inside an expanded group does nothing.
- `mouse.go` — `hitToolGroupChrome` maps a click on the summary row to
  `tool-group:<key>`; `dispatchMouseClick` calls `toggleToolGroup` (flips the
  flag, clears `rowCache`). `chatRowsSig` hashes the expanded keys so the cache
  reflects toggles.
- Child-agent focus transcripts reuse `buildChatRows` (and
  `replayChildHistoryMessages` feeds the same `Role:"tool"` messages), so the
  same collapse applies there with no extra code.

## Provider impact

Provider-agnostic (Case 1): all touched logic reads message roles/format hints,
never `providerKey`. The render/click/replay tests parameterize Claude / Codex /
Grok.

## Tests

New additive file `app/tui_tool_group_collapse_test.go`:

- Single tool between two normal responses renders directly, no group wrapper.
- Six consecutive tools render as one collapsed `+ 6 tool calls · click`; the
  individual `→` lines are hidden until expanded × claude/codex/grok.
- Clicking the summary expands (`- 3 tool calls` + all `→` lines); a second
  click collapses again × 3 providers.
- A user message between tools splits the run (2-run collapses, trailing single
  tool renders directly).
- A thinking placeholder between tools breaks the run (no group forms).
- Two same-length runs with different tool names don't cross-toggle.
- Child replay: a 3-tool run from `replayChildHistoryMessages` collapses too × 3
  providers.

## Verification

- `go build ./...` clean; `go vet ./internal/tui/app/` clean.
- `gofmt` clean on all changed files.
- `go test ./internal/tui/... ./internal/cli/... -count=1` all `ok` (legacy
  untouched and green, incl. `bug324_thinking_placeholder_test.go` and
  `turn_stream_closed_thinking_test.go` which append tool rows but assert no
  rendering of them).
- `go test -race ./internal/tui/app/ ./internal/tui/client/ -count=1` clean.

## Out of scope / residual

- Desktop is untouched — `timelineGrouping.ts` still wraps even a single tool in
  a `tool-group` (TUI intentionally diverges: single tool shows directly per the
  request).
- No tool-call args/output display; the collapse only folds the `→ name` lines.
- Expanded runs are not persisted across sessions; state is in-memory UI state.
- Pre-existing unrelated gofmt differences in older test files untouched.