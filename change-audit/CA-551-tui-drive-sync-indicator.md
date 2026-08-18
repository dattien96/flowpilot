# CA-551 - Visible Drive sync/restore progress (right-side spinner + start message)

## Problem

G2/G3 added `/sync` and `/restore` but the batch ran silently: nothing on screen
until the final summary, so a long push looked like a no-op. A user typing
`/sync` with an empty syncable list (history never fetched) saw only a one-line
message, and there was no in-flight progress anywhere on screen.

## Fix (TUI only)

- `internal/tui/app/chat_drive_sync.go`:
  - `driveIndicatorLine()` — a `Drive: ⠋ Syncing n/m` / `Drive: ⠋ Restoring n/m`
    line with the animated spinner, driven by `driveSyncFrame`; empty when idle
    so the session panel stays unchanged.
  - `driveSyncTickMsg` + `cmdDriveSyncTick()` — 90ms tick mirroring the thinking
    spinner cadence; reschedules while `driveSync`/`restoreBatch` is non-nil and
    self-cancels on idle.
  - `startSyncBatch` / `startRestoreBatch` now print an immediate start message
    ("Syncing run-xxx to Drive…" / "Syncing 3 chats to Drive…" /
    "Restoring m1:r1 from Drive…") so `/sync` always produces visible feedback.
- `internal/tui/app/app.go`: the always-on cursor tick starts the drive ticker
  while a batch is active (same pattern as the CA-537 thinking ticker);
  `case driveSyncTickMsg` advances the frame and reschedules/self-cancels.
- `internal/tui/app/session_panel.go`: `sessionInfoPanel.DriveStatus` rendered as
  a line in the panel; set from `driveIndicatorLine()` before the right sidebar
  (`renderRightSidebar`), the legacy top-right overlay
  (`renderSessionPanelOverlay`), and the `/status` dump.
- `internal/tui/app/model.go`: `driveSyncFrame` + `driveSyncTickerActive`.

## Files

- `apps/local-runner/internal/tui/app/chat_drive_sync.go`
- `apps/local-runner/internal/tui/app/app.go`
- `apps/local-runner/internal/tui/app/session_panel.go`
- `apps/local-runner/internal/tui/app/model.go`
- `apps/local-runner/internal/tui/app/ca551_drive_sync_indicator_test.go` (new)

## Tests

Additive (11 test funcs): idle indicator empty; sync/restore indicator shows
spinner + n/m progress; session panel lines render the Drive line; right sidebar
shows progress while syncing; start messages for single + bulk sync and single
restore; cursor tick starts the drive ticker while a batch is active; drive tick
self-cancels when idle and reschedules (frame advances) while active.

## Verification

- Full `go test ./internal/tui/app/ ./internal/tui/client/ -count=1` green except
  the two pre-existing environmental `TestCmdFocusAgent_*` failures.
- gofmt clean (LF-normalized temp copies); new Go files CRLF; `go vet` clean.
- No old test edited; no runner/desktop/OAuth changes; provider-agnostic.

## Change Ledger

```json
{
  "flowpilot:change-ledger": {
    "source_doc_id": "CP-56",
    "change_type": "bugfix",
    "feature_key": "cli-tui",
    "impacted_areas": ["tui-history", "tui-client", "tui-drive-sync"],
    "upstream_docs": ["SS-07", "CP-56", "SD-14", "SD-15", "Task-069", "BUG-311", "CA-549", "CA-550"],
    "status": "verified"
  }
}
```
