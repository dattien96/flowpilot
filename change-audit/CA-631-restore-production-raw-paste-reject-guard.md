# CA-631: Restore production Windows raw-paste reject guard removed by CA-630

## What

CA-630 (commit a521c32) deleted the `Run()` assignment that arms
`rejectWindowsRawPaste` on Windows, so production Windows Terminal never
rejected raw Ctrl+V floods anymore. Live log pid 16512 (08:28:16) shows the
regression end to end: 8 rapid runes `" vao for"` (3-8ms gaps, the same Ctrl+V
flood shape CA-612 guards) were **inserted and collapsed** into
`burst collapse active=false inputLen=16` (`[Pasted 8 chars]`) instead of
`burst collapse (windows reject) inputLen=1`, then **0 KeyMsg for ~7 minutes**
(08:28:17 → 08:35:47 `View slow dur=357.81ms ... side=true`). Compare pid 996
(same machine, same flood, pre-CA-630 binary): `(windows reject)` + input
survived. The suite stayed green because every paste test sets
`rejectWindowsRawPaste = true` directly on the model — the production path was
never exercised.

## Why

- CA-630's intent was to make paste logic platform-agnostic and testable, but
  it removed the only production arming site while keeping
  `model.go:292`'s contract comment "Set only in Run()".
- Without the guard, a WT Ctrl+V flood (runes at 3-8ms) passes the 25ms burst
  chain, gets inserted char-by-char, and collapses to a paste token. The flood
  also fills the conhost 64-slot input queue while View stays ~357ms with the
  F2 sidebar (ConnectedMsg forces `sessionPanel.Collapsed = false`), so keys
  stop arriving — the exact hang the user reported persists after CA-630.
- CA-630's NUL filter and `ClipboardPasteMsg` → `resetPasteBurst()` are correct
  and kept; they simply never ran in this session (no `alt+v` logged).

## Fix

- `apps/local-runner/internal/tui/app/app.go`:
  - New `applyProductionInputGuards(m *AppModel, goos string)` — arms
    `rejectWindowsRawPaste` when `goos == "windows"`, logs
    `input guards: rejectWindowsRawPaste=%v goos=%s`. The `goos` parameter
    keeps the contract testable on any host.
  - `Run()` calls `applyProductionInputGuards(m, runtime.GOOS)` after `New()`,
    restoring the deleted CA-612/CA-611 behavior for production Windows.
- No change to burst thresholds, IME logic, mouse filter, clipboard timeout,
  or the CA-630 NUL filter.

Will not undo: CA-610 mouse filter, CA-611 IME exempt (ngưỡng 8 +
`isSingleRepeatedRuneChain` + Backspace reset), CA-612 rejectArmed across gaps
+ settle wipe, CA-615 clipboard timeout, CA-621 View slow throttle, CA-624
token delete, CA-630 NUL filter + ClipboardPasteMsg reset.

## Tests (additive, no old edit)

- New `apps/local-runner/internal/tui/app/ca631_production_input_guard_test.go`:
  - `TestCA631_ProductionInputGuards_WindowsArms_OthersNoop` — linux/darwin
    no-op, windows arms (provider-agnostic, claude representative).
  - `TestCA631_Replay16512_FloodRejectedAndTypingLive_ClaudeCodexGrok` —
    exact pid-16512 flood with guard ON → reject armed, input wiped, settle
    resets, then `abc` typing live (matrix claude/codex/grok).
  - `TestCA631_Replay16512_WithoutGuardCollapsesToken_ClaudeCodexGrok` —
    same flood with guard OFF → `[Pasted 8 chars]` token (locks the CA-630
    regression; matrix claude/codex/grok).
- `go vet ./internal/tui/app` clean; `go test ./internal/tui/app -count=1`
  full suite green 13.2s (old tests untouched).

## Provider parity

Provider-agnostic: the guard is keyed on `goos`, never `providerKey`; the
matrix test covers claude/codex/grok so a future provider branch trips it.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CA-631
change_type: bugfix
summary: restore production Windows raw-paste reject guard (rejectWindowsRawPaste armed via applyProductionInputGuards in Run) so Ctrl+V floods are rejected instead of collapsing to a paste token and stalling input (log pid 16512, CA-630 regression)
# --->8---