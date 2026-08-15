---
id: CA-524
feature_key: cli-tui
title: F2 right sidebar (OpenCode-style) + todo-list step styling
date: 2026-08-15
status: COMPLETE
---

## Problem

F2 renders the session/steps info as a top-right **overlay** that floats over the
transcript and reserves its rows by shrinking the chat viewport. On a wide
terminal this reads unlike OpenCode's full-height right sidebar, and the flow
steps render as a terse `+1.DONE name` list rather than a clear todo list.

## Fix (TUI-only)

**A. Right sidebar layout (OpenCode-style)**

- `useRightSidebar()` — a full-height right column engages when the terminal is
  `>=100` cols, the F2 panel is expanded, and the panel has content. Narrow
  terminals keep the legacy top-right overlay; a wide-but-collapsed panel shows
  the `[info]` chip overlay. F2 still toggles expand/collapse in all cases.
- `renderRightSidebar(h)` — returns the sidebar column: a `session` header +
  info lines, a `steps` header + todo-list steps, and a `[collapse]` chip, padded
  to full terminal height.
- `View()` builds the chat column at `contentWidth` (`width - sideW - 1`, sideW
  clamped 30..42, OpenCode default 42) then joins it with the sidebar via
  `joinRightSidebar`. `m.fullWidth` preserves the real terminal width even while
  `m.width` is temporarily narrowed for rendering, so sidebar geometry stays
  stable.
- `mouse.go` — `tuiChrome` gains `sideActive/sideW/sideX/sideLines` and zeroes
  `panelH` when the sidebar is active (no top-overlay rows). `hitSidebarChrome`
  maps clicks to step `[open]`/`[back]`, `[collapse]`, and the `session`/`steps`
  headers (offsetting x by `sideX` since sidebar lines sit at absolute x>=sideX).
  `dispatchMouseClick` handles `sidebar-collapse`. `tryPlaceInputCursor` refuses
  to place the caret in the sidebar region.

**B. Todo-list step styling**

`flowStepsPanelLines` now renders OpenCode-style glyphs instead of `>1.RUNNING`:

- DONE `[✓]` (ascii `[+]`) · RUNNING/WAITING_USER_APPROVAL `[•] <STATUS>` ·
  FAILED `[x]` · other `[ ]`.
- Kept the legacy tokens the old suite asserts: `Steps N:`, `RUNNING`,
  `Now: <name>`, and the `[open]`/`[back]` step chips.

## Provider impact

Provider-agnostic (Case 1): all touched logic reads layout/step state, never
`providerKey`. The sidebar engagement and step-open matrix tests parameterize
Claude / Codex / Grok.

## Tests

New additive file `app/tui_f2_right_sidebar_test.go`:

- Wide expanded → `useRightSidebar`, `sideActive`, `panelH==0`, view has
  `[collapse]` × claude/codex/grok.
- Narrow (80) expanded → overlay retained (`panelH>0`, no sidebar).
- Wide collapsed → `[info]` chip, no sidebar `[collapse]`.
- Click `[open]` on a child step in the sidebar focuses that child × 3 providers.
- Click `[collapse]` folds the panel; click `session` header toggles.
- Step restyle keeps `Steps N:` / `RUNNING` / `Now:` tokens and adds `[+]`/`[•]`.
- FAILED step uses `[x]` and keeps its rejection note.

## Verification

- `go build ./...` clean; `go vet ./internal/tui/... ./internal/cli/...` clean.
- `gofmt` clean on all changed files.
- `go test ./internal/tui/... ./internal/cli/... -count=1` all `ok` (legacy
  untouched and green).
- `go test ./internal/tui/app -race` clean.

## Out of scope / residual

- No git-diff tab and no `todowrite` tool — sidebar is session + steps only.
- `/continue`/`/stop` and the blocked/attention chips unchanged.
- The `contentWidth` narrowing applies to the whole chat column; the status and
  composer now span only the chat column when the sidebar is active (status/composer
  no longer stretch under the sidebar).
- Pre-existing unrelated suites (`internal/runner` flaky, `internal/structure`,
  `internal/changecontract` platform path-separator on darwin) unchanged.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: feature
summary: F2 now renders a full-height right sidebar on wide terminals (>=100 cols, OpenCode-style) instead of the top-right overlay, and flow steps are restyled as a todo list ([✓]/[•]/[x]/[ ]) while keeping the legacy Steps N:/RUNNING/Now: tokens and step [open]/[back] chips; narrow/collapsed terminals keep the existing overlay behavior
# --->8---
