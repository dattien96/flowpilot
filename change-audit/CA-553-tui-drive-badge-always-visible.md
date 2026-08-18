# CA-553 - Drive sync badge always visible: picker start + session-panel line

## Problem

Even after CA-552 reconciled badges, the user still missed them: the picker
placed the badge **after** the `#N · kind · status · date` prefix, which on a
narrow terminal (`paintRow` truncates each picker row to `chatWidth()` — the
right sidebar eats ~42 columns) could land at/beyond the truncation boundary.
The `/history` dump appended `(synced)` at the very end of the line too, so a
wrapped row buried it. And when a chat was opened there was no badge anywhere
in the right sidebar — only the transient CA-551 progress spinner.

## Fix (TUI only)

- `internal/tui/app/helpers.go` (`filterHistorySuggestionsWithRemote`): the
  badge is now the **second token** of the detail line, right after the `#N`
  index: `#1 · synced · workflow · completed · Aug 18, 06:49 · title`. It is the
  first thing rendered after the 28-col run-id value column, so `paintRow`
  truncation can never cut it for `/history`, `/open`, and `/resume` (all three
  share this picker).
- `internal/tui/app/chat_drive_sync.go` (`filterSyncSuggestionsWithRemote`):
  `/sync` rows put the badge **before** the title (`synced · title`) for the
  same reason.
- `internal/tui/app/history.go` (`formatChatListWithRemote`): the dump moves the
  `(synced)` marker to immediately after `[kind] status`, before the timestamp
  and title, so it is never at the wrapped tail.
- `internal/tui/app/chat_drive_sync.go`: new `openChatDriveBadge()` resolves the
  currently open chat (`runHandle.RunID`) against the cached `/history` rows via
  the same `syncBadgeWithRemote` reconciliation the picker uses (local marker
  wins; else Drive-index match = synced).
- `internal/tui/app/session_panel.go`: `sessionInfoPanel` gains `DriveBadge`,
  rendered as its own `Drive: <badge>` line **directly below** the `Run: …`
  line. Set in `refreshSessionPanel` and in every render path that refreshes
  `DriveStatus` (`renderRightSidebar`, `renderSessionPanelOverlay`, `/status`),
  so an open chat always shows its badge in the right sidebar.

## Files

- `apps/local-runner/internal/tui/app/helpers.go`
- `apps/local-runner/internal/tui/app/chat_drive_sync.go`
- `apps/local-runner/internal/tui/app/history.go`
- `apps/local-runner/internal/tui/app/session_panel.go`
- `apps/local-runner/internal/tui/app/app.go`
- `apps/local-runner/internal/tui/app/ca553_drive_badge_always_visible_test.go` (new)

## Tests

Additive (10 test funcs): picker detail starts with `#N · synced`; `/history`,
`/open`, `/resume` all carry the badge and never leak it to unsynced rows; dump
badge precedes the title and empty badges never render; `openChatDriveBadge`
from cached history, from remote reconcile, and empty for no-run / unsynced;
session-panel lines render `Drive: synced` exactly one row below `Run:` and
skip it when empty; right sidebar shows the open chat's badge after the Run
line.

## Verification

- Full `go test ./internal/tui/app/ ./internal/tui/client/ -count=1` green except
  the two pre-existing environmental `TestCmdFocusAgent_*` failures.
- gofmt clean (LF-normalized temp copies); new/changed Go files CRLF.
- No old test edited; no runner/desktop/OAuth changes; provider-agnostic.

## Change Ledger

```json
{
  "flowpilot:change-ledger": {
    "source_doc_id": "CP-56",
    "change_type": "bugfix",
    "feature_key": "cli-tui",
    "impacted_areas": ["tui-history", "tui-client", "tui-drive-sync", "tui-session-panel"],
    "upstream_docs": ["SS-07", "CP-56", "SD-14", "SD-15", "Task-069", "BUG-309", "BUG-311", "CA-548", "CA-552"],
    "status": "verified"
  }
}
```