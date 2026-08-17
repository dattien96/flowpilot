---
id: CA-534
feature_key: cli-tui
title: Run ID sidebar, flow thinking, text paste, prompt-history arrows
date: 2026-08-17
status: COMPLETE
---

## Problem

Four TUI polish items after the CA-533 thinking animation landed:

1. The right sidebar / session panel shows Runner / Path / Project / Session but
   not the current **Run id** (`run-xxxx`), so correlating the TUI with runner
   logs / Desktop is a manual hunt.
2. `run-102521`: after an answer finishes and `[GATE] Flow gate triggered`
   appears, the chat keeps running but the thinking animation is absent — flow
   chrome suppressed thinking placeholders entirely.
3. Copying a message from elsewhere on the PC then pasting it into the composer
   can fail or attach an image instead: `cmdClipboardPaste` preferred the image
   format, and rich-text apps put both text and an image on the clipboard.
4. Up/Down were transcript-scroll keys when no slash picker was open; the user
   wants bash-style prompt recall instead (scroll stays on PgUp/PgDown + wheel).

## Fix (TUI-only)

### 1. Run id in the sidebar

`sessionInfoPanel` gains `RunID`; `lines()` emits `Run: <shortID>`, `hasContent`
counts it, and `refreshSessionPanel()` seeds it from `m.runHandle.RunID` (empty
until a run exists). `renderRightSidebar` and the F2 overlay both render it for
free. `RunStartedMsg` and `ChatOpenedMsg` now refresh the panel so the id
appears as soon as a run is live.

### 2. Thinking in flow chrome + after a gate

`processInput` always adds the animated thinking placeholder (removed the
`!isFlowChrome()` gate — the F2/steps status line still works, and the chat row
now mirrors live work). `handleGateInput` adds a fresh thinking placeholder when
a valid gate decision is submitted, because the flow continues after the gate.
The old contract test `TestFlowMode_NoThinkingPlaceholderInChat` was removed —
its premise (progress lives only on F2) is obsolete and the user approved
dropping it. `isFlowChrome()` keeps its remaining status-line duties.

### 3. Text-first clipboard paste

`cmdClipboardPasteWithFallback` reorders: real clipboard **text** wins (so a
copied message pastes even when the clipboard also carries an image format);
an image-path is still attached; an image is attached only when there is no
text; bracketed-paste `fallbackText` covers the native-read failure case. Alt+V
/ ctrl+shift+v / `/image paste` remain the explicit image routes.

### 4. Up/Down prompt history

New `prompt_history.go`: `recordPromptHistory` (called from `processInput`)
appends sent non-slash prompts, collapses consecutive duplicates, caps at 100,
and resets browsing. `navigatePromptHistory` walks the ring: Up recalls older,
Down walks back to the stashed draft. `handleKey` Up/Down now navigate history
when no picker is open (scroll removed from those two keys).

## Provider impact

Provider-agnostic (Case 1): none of the touched paths branch on `providerKey`;
the new tests parameterize Claude / Codex / Grok.

## Tests

New additive file `app/tui_run_sidebar_gate_paste_history_test.go`:

- `TestFlowMode_ShowsThinkingPlaceholder` × claude/codex/grok — flow chat has an
  animated `Thinking … 0s` row.
- `TestGateDecision_ShowsThinkingPlaceholder` × claude/codex/grok — valid gate
  decision clears the gate, submits, shows thinking + `statusMsg`.
- `TestGateDecision_InvalidKeepsGateNoThinking` — bogus input leaves the gate
  pending and adds no thinking row.
- `TestSessionPanel_ShowsRunID` × claude/codex/grok — sidebar lines include
  `Run: run-102521`.
- `TestSessionPanel_RunIDUpdatesWithHandle` — no Run line before a run, appears
  after.
- `TestRightSidebar_IncludesRunID` — full-height right sidebar shows the run id.
- `TestClipboardPasteMsg_InsertsTextAtCursor` — text paste lands in the composer.
- `TestPromptHistory_UpDownRecall` — Up/Down ring + draft restore.
- `TestPromptHistory_ExcludesSlashAndDedupes` — slash skipped, dupes collapsed.
- `TestPromptHistory_NoHistoryKeepsInput` — no clobbering with empty history.
- `TestPromptHistory_NewSendResetsBrowse` — sending resets browse index.

One legacy test removed (user-approved): `TestFlowMode_NoThinkingPlaceholderInChat`.
All other legacy thinking / flow-chrome tests untouched and green.

## Verification

- `go build ./...` clean; `go vet ./internal/tui/...` clean.
- gofmt clean on the touched files (checked on LF-normalized copies — this
  checkout is CRLF repo-wide, untouched files are flagged identically).
- `go test ./internal/tui/... -count=1` green except the two **pre-existing**
  network-flaky `TestCmdFocusAgent_*` tests (same caveat as CA-533).
- Broader `go test ./internal/...` green except a pre-existing timing flake
  `TestRun75035_HubResumeStillLoadsOwnCodexSession` in `internal/runner`, which
  passes in isolation and is unrelated to these TUI-only changes.
- `-race` not runnable on this box (no gcc for cgo).

## Out of scope / residual

- Desktop `Timeline.tsx` untouched — this covers the TUI client only.
- Up/Down no longer scroll the transcript; scroll is PgUp/PgDown + mouse wheel
  (intentional per request).
- Prompt history is in-memory only (not persisted across sessions).
- GitNexus MCP tooling unavailable in this session; impact/scope verified via
  local code search + diff review (same caveat as CA-059).

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CA-534
change_type: feature
summary: Run ID in sidebar, thinking in flow chrome + after gate, text-first paste, Up/Down prompt history
# --->8---