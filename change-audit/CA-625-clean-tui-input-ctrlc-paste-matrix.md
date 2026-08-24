# CA-625: Clean TUI input context, protect Ctrl+C, support Command-V and Alt-V paste matrix

## What

Cleaned and consolidated TUI chat input logic across Windows and macOS. Added prompt-clearing protection on `Ctrl+C` (avoids accidental process quit when typing a draft prompt), standardized Windows Ctrl+V raw flood rejection, and guaranteed `Command-V` (macOS bracketed paste) and `Alt-V` (Windows/Linux/macOS async clipboard) paste paths with full matrix tests.

## Why

- **Context Dilution across Many CAs**: Rapid iteration across CA-535..CA-624 left hardcoded `runtime.GOOS == "windows"` checks in production code paths, preventing Windows raw flood rejection from being unit-tested on macOS/Linux and confusing the Single Source of Truth.
- **Accidental Process Exit on `Ctrl+C`**: If a user was composing a draft prompt and pressed `Ctrl+C` (by muscle memory to copy or cancel), `handleKey` immediately triggered `m.cmdShutdownAndQuit()`, killing FlowPilot and dropping the unsent prompt.
- **Paste Ambiguity across Platforms**: On macOS, `Command-V` delivers native bracketed paste (`msg.Paste == true`); on Windows/Linux, `Alt-V` delivers 1-message clipboard paste via timeout-bounded background worker; raw Windows Terminal `Ctrl-V` character floods are rejected and guided to `Alt-V`.

## Fix

1. **`apps/local-runner/internal/tui/app/app.go`**:
   - `handleKey`: Updated `KeyCtrlC` so that if `m.inputValue != ""` and no text is selected/turn is running, pressing `Ctrl+C` clears the prompt (`m.clearInputValue()`, `m.statusMsg = "prompt cleared"`) instead of killing the app. Pressing `Ctrl+C` on an empty prompt continues to quit as expected.
   - `handleKey`: Replaced hardcoded `runtime.GOOS == "windows"` checks with `m.rejectWindowsRawPaste` (which is enabled by default on Windows in `Run()`), allowing all platforms and test suites to verify Windows raw paste rejection deterministically.
   - Fixed missing `m.pasteCtrlVHintShown = true` flag on `KeyCtrlV` reject branch.
2. **`apps/local-runner/internal/changecontract/paths.go` & `frozen_scope.go`**:
   - Fixed path normalization to convert backslashes (`\`) to forward slashes (`/`) across all platforms before checking.
3. **`apps/local-runner/internal/tui/app/tui_input_comprehensive_matrix_test.go`**:
   - Comprehensive 10-axis test suite testing UTF-8 multi-byte, Vietnamese Telex IME (`dd` -> `đ`), tone key holds (`ssss`), caret movements, Backspace/Delete token deletion in 1 press, Windows raw flood rejection, `Command-V` / `Alt-V` / `Ctrl-V` paste shortcuts, send gating on Turn Running vs Workflow Park, child view isolation, and 3-provider parity (`claude`, `codex`, `grok`).

## Tests

- `go test ./internal/tui/... ./internal/cli/... -count=1` passed 100%.
- New comprehensive test suite `tui_input_comprehensive_matrix_test.go` runs all 10 feature axes against Claude, Codex, and Grok.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CA-625
change_type: refactor
summary: clean TUI input context, protect Ctrl+C from accidental quit on draft, support Command-V and Alt-V paste matrix across Windows and macOS
# --->8---
