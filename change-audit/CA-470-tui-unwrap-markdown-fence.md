---
id: CA-470
feature_key: cli-tui
title: Unwrap markdown fences; box real code only
date: 2026-08-13
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-469-tui-status-bar-rows
will_not_undo: CA-466 fence [copy]; CA-464 goldmark walk; CA-463 no assistant markdown-label box
```

## Change

Grok (and any provider) often wraps a cheatsheet in ` ```markdown ` with a nested ` ```go ` plus a prose/table summary. Goldmark treated the outer fence as one code box titled `markdown` (raw inner ticks) and leftover empty-lang/indented prose as a `code` box.

`walkCode` now:

- Re-parses `markdown`/`md`/`gfm` fences as GFM (no outer labeled box; nested `go` gets its own hug box + `[copy]`).
- Unwraps empty-lang or indented blocks that look like a markdown document (headings, GFM tables, `**bold**`, nested fences).
- Skips empty fences so a leftover closer does not draw a blank `code` box.
- Real empty-lang code (`fmt.Println`) stays boxed as `code`.

Provider-agnostic: `markdown_render.go` has no `providerKey` branch. TUI chrome only.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: Unwrap markdown fences and skip boxing prose/tables so only real code gets a labeled box
# --->8---
