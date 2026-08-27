# CA-653: TUI attach open without cmd.exe and console re-arm

## What

Fix for TUI stuck after Alt+V image → [open] (log pid 12736: 49s stall,
`motionLive=true`, 0 KeyMsg/MouseMsg after `attach-open:1`).

- `clipboard_paste.go`: remove `openPath` (`cmd /c start`); Windows path now lives in `open_path_windows.go` via `ShellExecuteW("open", path)` — no `cmd.exe` conhost attachment, so Photos opening does not steal console focus or leave the terminal without input.
- `open_path_windows.go` (new, `//go:build windows`): `ShellExecuteW` with `SW_SHOWNORMAL`, error when `<=32`; `open_path_other.go` keeps `open`/`xdg-open` on darwin/linux.
- `app.go:AttachmentOpenMsg`: re-arm console with `disableConsoleQuickEdit()` immediately so QuickEdit/mouse mode survives a viewer activate.
- Tests: `attach_open_console_test.go` — `TestAttachmentOpenMsg_RearmsConsole` (handler adds "Opened image" and does not wedge), `TestOpenPath_WindowsUsesShellExecute` (no `exec.Command`, uses `ShellExecuteW`, has build tag), `TestOpenPath_OtherUsesOpenFallback`.

## Why

`openPath` used `cmd /c start "" path` on Windows. `cmd.exe` attaches to the TUI console; activating Photos steals focus and the conhost 64-slot queue delivers hover (motionLive) but no clicks/keys until the user clicks back into WT. Watchdog logged the stall but TUI was idle so no banner. Replacing with ShellExecuteW removes the console-attached parent.

## Tests

- `go vet ./internal/tui/app` clean
- `go test ./internal/tui/app -run "TestHitAttachPanel|TestAttachmentOpenMsg|TestOpenPath" -count=1` PASS
- `go test ./internal/tui/app -count=1` 12.7s PASS (includes `task308_textarea_test.go` boundary)

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: Task-308
change_type: bugfix
summary: open image via ShellExecuteW not cmd/start and re-arm console QuickEdit after viewer
# --->8---
