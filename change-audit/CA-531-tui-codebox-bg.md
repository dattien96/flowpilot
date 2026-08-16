---
id: CA-531
feature_key: cli-tui
title: Lift code-block background into a solid card (opencode-style)
date: 2026-08-17
status: COMPLETE
---

## Problem

Fenced code blocks in the TUI render with a **bright, empty-looking outline**: the
body fill was `#252526` (nearly identical to the terminal/`--bg-3` `#1e1e1e`) while
the `┌─┐│└─┘` border glyphs and header/footer strokes were plain default (bright
terminal text). The result reads as a white box around near-black content instead
of a lifted gray card. Compare to opencode, which paints each code block as a
solid gray panel against the dark app background.

## Fix (TUI-only)

1. **`colorCodeBg` → `#2e2e2e`** (`app.go:36`) — a clearly lifted gray vs the dark
   terminal, still distinct from `colorBg3`.
2. **Full solid fill** (`markdown_render.go`): new `strokeTopSolid` /
   `strokeBottomSolid` replace `strokeTopChip` / `strokeBottom` inside
   `renderCodeFenceBox`, and `strokeCodeFill` dims the `│` glyphs. The entire
   block — header, body, footer, border glyphs — now carries `colorCodeBg`, with
   border glyphs at `colorTextDim` (`styleMdCodeBar`, `markdown.go`).
3. **Not changed:** `styleMdCodeBox` stays bound to `colorCodeBg`; You-box / chat
   strokes / `colorBg3` untouched; no full-viewport repaint (terminal bg already
   reads as the opencode `--bg` canvas).

## Provider impact

Provider-agnostic (Case 1): pure rendering change, no `providerKey` read. The
render-across-model test runs × claude/codex/grok.

## Tests

New additive file `app/tui_codebox_bg_test.go`:

- `TestCodeBoxSolidFill_AllRowsOnCodeBg` — every body row carries the `#2e2e2e`
  truecolor bg (forced `termenv.TrueColor`; distinct md-cache keys so profile
  cannot serve a stripped cache hit).
- `TestCodeBoxSolidFill_HeaderAndFooterOnCodeBg` — header and footer strokes also
  carry the bg (not just the body).
- `TestCodeBoxSolidFill_BorderGlyphsDimmed` — `styleMdCodeBar` bg equals the box
  bg and fg is dimmer than body text; `colorCodeBg != colorBg3`.
- `TestCodeBoxSolidFill_RendersAcrossProviders` — labeled `go` box with body
  renders through `AppModel` × claude/codex/grok, no raw ` ``` ` leak.

## Verification

- `go build ./...` clean; `go vet ./internal/tui/app/` clean; `gofmt` clean.
- `go test ./internal/tui/... ./internal/cli/... -count=1` all `ok` — legacy
  `TestRenderMarkdown_CodeBlockHasLabeledBox` (bg == `colorCodeBg` != `colorBg3`)
  untouched and green.
- `go test ./internal/tui/app -race -count=1` clean.

## Out of scope / residual

- No full-viewport background paint.
- You-box and other chat chrome unchanged.
- No syntax highlighting (only block-level styling).