# CA-653: TUI attach open + clipboard without console attach and re-arm

## What

Fix for TUI stuck after Alt+V (log pid 12736 after image open, pid 9288 after
text paste `192` chars: 47-49s stall `motionLive=true`, 0 KeyMsg/MouseMsg,
Enter never reaches `handleKey`).

- `clipboard_paste.go`: remove `openPath` (`cmd /c start`); Windows path now lives in `open_path_windows.go` via `ShellExecuteW("open", path)` — no `cmd.exe` conhost attachment, so Photos opening does not steal console focus.
- `open_path_windows.go` (new, `//go:build windows`): `ShellExecuteW` with `SW_SHOWNORMAL`, error when `<=32`; `open_path_other.go` keeps `open`/`xdg-open` on darwin/linux.
- `clipboard_paste.go:runClipboardPS`: keep PowerShell from attaching to the TUI console on Windows via `applyClipboardSysProcAttr` → `CREATE_NO_WINDOW (0x08000000)` (same pattern as `runner/probe_cmd_windows.go` CA-535). `clipboard_ps_windows.go` / `clipboard_ps_other.go`.
- `app.go:AttachmentOpenMsg` + `ClipboardPasteMsg`: re-arm console with `disableConsoleQuickEdit()` immediately so QuickEdit/mouse mode survives a viewer activate or a PS clipboard child (both previously left conhost without keys).
- Tests: `attach_open_console_test.go` — `TestAttachmentOpenMsg_RearmsConsole`, `TestOpenPath_WindowsUsesShellExecute`/`Other`, `TestRunClipboardPS_UsesCreateNoWindow` (assert `CREATE_NO_WINDOW` + helper call), `TestClipboardPasteMsg_TextThenEnterSubmits` (short paste inline and long collapsed paste both Enter-submit).

## Why

Both `openPath` (`cmd /c start`) and `runClipboardPS` (`powershell` without `CREATE_NO_WINDOW`) inherit the TUI conhost. Activating Photos or running the PS `Forms.Clipboard` STA leaves hover but no clicks/keys until click-back. Probe already fixed the same attach via `CREATE_NO_WINDOW` (CA-535); clipboard was not. Bubble `textarea` mirrors value only — keys must first reach `Update`, so the fix is below bubbling.

## Tests

- `go vet ./internal/tui/app` clean
- `go test ./internal/tui/app -run "TestHitAttachPanel|TestAttachmentOpenMsg|TestOpenPath|TestRunClipboardPS|TestClipboardPasteMsg" -count=1` PASS
- `go test ./internal/tui/app -count=1` 13.6s PASS (includes `task308_textarea_test.go` boundary)

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: Task-308
change_type: bugfix
summary: open image via ShellExecuteW and clipboard PS CREATE_NO_WINDOW, re-arm console after paste/open
# --->8---
