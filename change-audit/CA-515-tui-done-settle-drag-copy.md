---
id: CA-515
feature_key: cli-tui
title: Settle finished-flow TUI chrome; copy drag-selection on mouse release
date: 2026-08-15
status: COMPLETE
---

## Problem

Two operator-visible TUI defects after the CA-513/CA-514 live-agent work:

1. **Stale "streaming…" + [stop] on finished flows (run-189839).** A grok-flow
   finished its loop (`loopState=done`, all steps DONE, coder + reviewer agents
   completed) yet the statusline kept `[stop]` and flip-flopped to
   `streaming…`/`flow running…` forever. The runner deliberately keeps the hub
   run raw status `"running"` past loop `done` until the last SSE settles
   (`interactive_handlers.go` historyStatusForLiveRun comment), and the TUI's
   `turnIsActive()`/`shouldPollStepsRuntime()` treat a non-terminal raw handle +
   active orch stream as live — so nothing flipped the chrome to `done`.

2. **Copying a selection on macOS (Terminal.app).** The TUI binds copy to
   Ctrl+C (CA-480), but Terminal.app swallows Cmd+C for native copy and never
   delivers it to the TUI; bubbletea v1.3.10 has no Super/Cmd modifier, so the
   drag-select → Ctrl+C copy workflow was unreliable in the operator's terminal.

## Fix

**A — copy on drag release (Terminal.app workaround):**

- `autoCopySelectionOnDragEnd()`: on mouse release after a real drag (plain-left
  and Shift+drag paths in `mouse.go`), copy `selectionPlainText()` via
  `cmdCopyText(text, "selection")`. The `CopiedMsg` handler already toasts
  `"Copied selection."` (CA-511), so no new toast plumbing.
- The selection **stays armed** after auto-copy so Ctrl+C remains a working
  fallback — this preserves the CA-480 legacy test `TestPlainDrag_SelectsTranscriptWithoutShift`
  (release keeps the highlight). The next press clears it.
- Click-without-drag still clears leftover highlight and only pulses mouse mode
  (no accidental copies).

**B — settle flow chrome when the loop is done:**

- New `AppModel.flowLoopStatus` mirrors the orchestrator `LoopState.Status`
  from the live `agent_graph_updated` SSE (`handleEvent`) and `AgentGraphMsg`
  (in-memory only — nothing persisted, no replay-contract impact).
- `settleFlowIfDone()`: when flow chrome is active AND loop status is `done`
  AND no step is still active (`flowStepsActive == ""`) AND no agent is live
  (`flowHasActiveAgents() == false`) AND no turn/focus stream is open, flip the
  chrome to `ConnIdle` / `statusMsg="done"` / `runHandle.Status="completed"`.
  Called after every `StepsRuntimeMsg` refresh, `agent_graph_updated`, and agent
  hydrate — so once the last poll confirms all steps terminal, `[stop]`
  disappears and the statusline stops showing `streaming…`.
- `message_delta` no longer overwrites flow chrome with `"streaming…"`
  (guarded by `isFlowChrome()`, consistent with CA-511 quiet chat) — a delta
  must not mask a finished flow behind a fake live spinner.

## Provider impact

Agnostic. No provider adapter or SSE/stream path touched; `flowLoopStatus`
and the settle/copy logic branch on nothing provider-specific. Claude / Codex /
Grok flows share the same TUI chrome paths.

## Tests

`tui_drag_copy_test.go` — drag release copies + toasts, Shift+drag release
copies, click-without-drag does not auto-copy (mouseSel cleared).
`tui_flow_done_settle_test.go` — steps-DONE + loop-done + agents-completed
settles to ConnIdle/done (no [stop], no streaming…); running child blocks
settle; live turn stream blocks settle; loop still `running` blocks settle;
`AgentGraphMsg` with `LoopState.Status=done` settles.
Legacy suite untouched and green (only additive files; one pre-existing runner/
structure suite failure confirmed unrelated via stash before this change).

## Residual / out of scope

- Runner `runSnapshot` still reports raw `status:"running"` past loop `done`
  (out of scope per operator: TUI scope only). History list already maps loop
  done → completed via `historyStatusForLiveRun`.
- Shift+drag copy on terminals that do deliver Shift+drag is unchanged; plain
  drag auto-copy is the new default on all platforms.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: Copy drag-select on mouse release (Terminal.app Cmd+C workaround); settle finished-flow TUI chrome from loop done + steps done + no live agents; stop message_delta overwriting flow chrome with streaming…
# --->8---