---
id: CA-529
feature_key: cli-tui
title: Keep step [open]/[back] chip stable while viewing a child agent
date: 2026-08-17
status: COMPLETE
---

## Problem

While a sub-agent is open (F2 step shows `[back]`), the right sidebar chip
**automatically flickers** between `[back]` and `[open]` without any user input —
the transcript stays stable, only the chip flips.

Chip rendering (`session_panel.go`):

```go
if m.viewingChild() && child.RunID == m.focusRunID { … "[back]" } else { … "[open]" }
```

`childRunForStep` returned the **first** run in `m.agentRuns` matching the step
keys (AgentRef/NodeID/StepType). Live `agent_graph_updated` SSE and the
`GET …/agents` poll (~1.6 s) both **replace** `agentRuns`. The runner can report
two runs for the same agent (a live run + a historical/restored run with the
same `AgentName`) and their order is not stable across polls. When a different
run moved to the front, `childRunForStep` returned a RunID ≠ `focusRunID`, so
the chip flipped to `[open]` for one poll and back to `[back]` the next —
`focusRunID` and the transcript were untouched, which is why only the sidebar
blinked.

## Fix (TUI-only)

1. **`childRunForStep` prefers the focused run** (`agents_focus.go`): when
   `m.viewingChild()`, the run whose RunID matches `focusRunID` and also matches
   the step keys wins; otherwise the previous first-match behavior is preserved
   (CA-513/524 contract for the unfocused case).
2. **Safe chip comparison** (`session_panel.go`): `EqualFold` + `TrimSpace` on
   both RunIDs instead of a raw `==`, so casing/whitespace drift cannot flip the
   chip.

No click debounce was added — the transcript never flickered, so the loop was a
data-order oscillation, not a click loop.

## Provider impact

Provider-agnostic (Case 1): the change only orders the agent-run mapping against
`focusRunID`; no `providerKey` is read. The hydrate-reorder chip test
parameterizes Claude / Codex / Grok.

## Tests

New additive file `app/tui_open_back_chip_stable_test.go`:

- `TestChildRunForStep_PrefersFocusedRun` — focused `run-b` wins over same-named
  `run-a`; without focus the first match still wins (regression guard).
- `TestOpenBackChip_StableAcrossHydrateReorder` — list hydrate puts `run-a`
  first, focused `run-b` keeps `[back]`, never `[open]` × claude/codex/grok.
- `TestOpenBackChip_GraphReorderKeepsBack` — `agent_graph_updated` reorder keeps
  `[back]` for the focused child.
- `TestOpenBackChip_OpenWhenNotFocused` — unfocused mapped step still shows
  `[open]` (CA-513/524 kept).

## Verification

- `go build ./...` clean; `go vet ./internal/tui/app/` clean.
- `gofmt` clean on all changed files.
- `go test ./internal/tui/... ./internal/cli/... -count=1` all `ok` (legacy
  untouched and green, incl. CA-524 `tui_f2_right_sidebar_test.go` and
  `tui_step_agent_open_test.go`).
- `go test ./internal/tui/app -race -count=1` clean.

## Out of scope / residual

- `/agents` API not changed; runner-side ordering not addressed.
- No click debounce (not needed once the chip no longer oscillates).
- 8-step cap in the sidebar unchanged.