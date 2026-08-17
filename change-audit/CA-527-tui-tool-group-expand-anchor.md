---
id: CA-527
feature_key: cli-tui
title: Expanding a tool-call group must push the transcript down, not up
date: 2026-08-17
status: COMPLETE
---

## Problem

The transcript viewport is bottom-anchored: `viewport.offset` is the number of
rows scrolled up from the bottom (`0` = follow live; `sliceViewport` computes
`end = len(lines) - offset`). When a tool-call group expanded (CA-525),
`toggleToolGroup` flipped the state and dropped the row cache but did **not**
touch `offset`. Inserting N `→ tool` rows after the summary kept the anchor glued
to the content below, so the expanded list pushed **up** off the top of the
window — the user had to scroll up to see the tool lines, and the following
message `b` stayed at its old screen row instead of shifting down:

```
before (screen):  a          after (bug):     a        (a pushed off-screen)
                  summary                     summary → tools  … off top
                  b                          b          (b glued)
```

## Fix (TUI-only)

`toggleToolGroup` now shifts the viewport offset by ±N on the same toggle, where
N is the number of tool lines in the run (from the group key: `strings.Count(key,
"\x1f") + 1`):

- **Expand:** `offset += N` — the window start is unchanged (`end` = total+N -
  (offset+N) = old `end`), so the summary row — and every row above it — stays
  pinned on screen while the N tool rows expand **downward** and `b` moves to a
  lower row.
- **Collapse:** `offset -= N`, floored at 0.

The adjustment is skipped while `m.selecting()` so it cannot fight the CA-526
viewport freeze during a drag; the click path clears `mouseSel` before dispatch,
so a normal click toggle still adjusts the offset. `View()`'s `clampViewport`
keeps `offset` within `[0, maxOff]` afterward. When the whole transcript fits in
the viewport, `sliceViewport` ignores `offset`, so short transcripts are
unaffected.

## Provider impact

Provider-agnostic (Case 1): the change is pure viewport geometry driven by the
tool-run key; the screen-anchor tests parameterize Claude / Codex / Grok.

## Tests

New additive file `app/tui_tool_group_expand_anchor_test.go`:

- Offset math: at the bottom anchor, expand raises `offset` by exactly N (3 tool
  rows) and collapse restores it.
- Screen anchor (scrollable transcript, bottom): expanding keeps the `tool calls`
  summary on the same screen row, moves the following `ZZZ_after` row **down**,
  and shows the `→ read_file` lines in the window — collapsing restores `b` to
  its original row and hides the lines × claude/codex/grok.
- Top anchor (`offset = maxOff`): expanding keeps the first content row in place
  and the tool lines appear just under the summary.
- Click path: after an expand click, the summary is still hit-testable at the same
  screen row, and a second click at that same row collapses the group × 3
  providers.

## Verification

- `go build ./...` clean; `go vet ./internal/tui/app/` clean.
- `gofmt` clean on all changed files.
- `go test ./internal/tui/... ./internal/cli/... -count=1` all `ok` (legacy
  untouched and green, incl. CA-525 `tui_tool_group_collapse_test.go` and
  CA-526 `tui_click_select_coords_test.go` — the freeze test still passes because
  the offset shift is skipped while `selecting()`).
- `go test -race ./internal/tui/app/ -count=1` clean.

## Out of scope / residual

- Desktop untouched.
- Edge-scroll while dragging past the viewport edge not added.
- Live-stream growth still anchors to the bottom (existing behavior, unchanged).