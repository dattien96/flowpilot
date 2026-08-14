---
id: CA-479
feature_key: cli-tui
title: Unwrap orphan empty-lang prose fences after nested markdown
date: 2026-08-14
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-470-tui-unwrap-markdown-fence
will_not_undo: CA-470 markdown/doc unwrap; CA-466 fence [copy]; empty-lang real code stays boxed
```

## Issue

`run-95315`: model wrapped a sample in ` ```markdown ` with nested ` ```bash `. Same-length fences make goldmark close the outer block at the bash closer; the intended outer closer opens a new empty-lang fence that swallowed the trailing chat sentence into a labeled `code` box in the TUI.

## Change

In `markdown_render.go`, empty-lang fences that look like short multi-word chat prose (`looksLikePlainProseFence`) are unwrapped and re-rendered as markdown, same path as markdown-lang unwrap.

Conservative guards so prior contracts hold:

- Real code signals (`fmt.Println`, `./`, `=`, braces, keywords, …) stay boxed
- Single-token fences (`one` / `two`) stay boxed
- Multi-line dumps (>6 non-empty lines) stay boxed (truncation still applies)

## Tests

Additive only (`markdown_orphan_prose_fence_test.go`):

- run-95315 nested markdown/bash + trailing prose
- empty-lang sentence-only fence
- empty-lang code still boxed after heuristic

Full `./internal/tui/app` package green; no edits to pre-existing tests.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: Unwrap orphan empty-lang prose fences so trailing chat text is not boxed as code
# --->8---
