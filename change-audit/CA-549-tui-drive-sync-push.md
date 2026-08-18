# CA-549 - TUI /sync pushes chat sessions to Drive (batch, no freeze)

## Problem

The TUI had no way to upload a chat session to the project's Drive folder.
Desktop Navigator has per-row "Sync" and a bulk "Sync all"; the TUI `/history`
only showed rows (and, since CA-548, their sync badges) but the only action was
`/open`. A TUI user wanting the Drive backup (server parity via
`/client/workflow-runs/{id}/sync-chat`) had to open Desktop.

## Fix (TUI only — G2 of the TUI Drive parity G1/G2/G3 plan)

- `internal/tui/app/chat_drive_sync.go` (new): `isSyncableRun`/`isAgentHistoryItem`/
  `hasAgentPromptPrefix` are a faithful port of Desktop
  `navigatorHistory.ts:25-41` — kind `chat|""|workflow`, never an agent child
  (ParentRunID or agent-prompt prefix), never unavailable, never already
  `synced`/`unsyncable`, and never already present in the remote index by
  `sourceMachineId+sourceRunId`. `driveSyncState` + `DriveSyncBatchMsg` run the
  batch: one HTTP per Update, sequential queue, single failure never aborts the
  rest (Desktop `syncRuns` parity), and the chatList row's badge is written back
  immediately (`markChatSyncStatus`).
- `internal/tui/app/app.go`: `case "/sync"` → `runSyncDispatch` (current chat /
  `<n|runId>` via `resolveChatOpenTarget` / `all`); Update handles
  `DriveSyncBatchMsg` by starting the next item or printing the summary
  `Synced n/m … · k failed.`; `collectSuggestions` gains the `/sync ` picker;
  `suggestionAcceptValue` maps kind `"sync"` → `/sync <value>` (the `all` bulk
  row included).
- `internal/tui/app/chat_drive_sync.go`: `filterSyncSuggestions` — bulk `all`
  row stays visible while typing a query (Desktop "Sync all" parity).
- `internal/tui/app/history.go`: `cmdMaybePrefetchHistory` also prefetches for
  `/sync`, `/sync all` and `/sync ` so the picker is never empty.
- `internal/tui/app/model.go`: `remoteChatList` cache field (populated by G3)
  and `driveSync` batch state; `/sync` and `/restore` appended at the END of
  `knownSlashCommands` so `/help` stays suggestion index 0 and `/s` skill
  picking keeps its own prefix match.
- Error mapping: `google_drive_not_connected` → hint `/settings`; `unsyncable` →
  runner wording; everything else surfaces the raw APIError message. Missing
  project → clear message, no batch (never panics).

## Files

- `apps/local-runner/internal/tui/app/chat_drive_sync.go` (new)
- `apps/local-runner/internal/tui/app/app.go`
- `apps/local-runner/internal/tui/app/history.go`
- `apps/local-runner/internal/tui/app/model.go`
- `apps/local-runner/internal/tui/app/ca549_tui_drive_sync_push_test.go` (new)

## Tests

Additive (8 test funcs, ~14 cases): isSyncableRun matrix (kind/agent-child/
synced/unsyncable/unavailable/remote-index); remote index truth for restored
rows; /sync picker all-row + query filtering + non-/sync input returns nil;
suggestionAcceptValue; dispatch for current run / `all` batch queue / no-project
no-op / bad target no batch; batch state machine — one failure continues the
queue, badge writes back, final message, `driveSync` resets to nil; summary
strings; error hints; `cmdMaybePrefetchHistory` triggers on `/sync`.

## Verification

- Full `go test ./internal/tui/app/ ./internal/tui/client/ -count=1` green except
  the two pre-existing environmental `TestCmdFocusAgent_*` failures.
- gofmt clean (LF-normalized temp copies); new Go files CRLF; `go vet` clean.
- Provider-agnostic (cross-provider-parity Case 1): `/sync` POSTs only
  `runId`+`googleDriveProjectId`; no providerKey branch — runner already handles
  Claude/Codex/Grok. No old test edited; `chatOpenSlashCommands`,
  `parseChatOpenArgPrefix` untouched.

## Change Ledger

```json
{
  "flowpilot:change-ledger": {
    "source_doc_id": "CP-56",
    "change_type": "bugfix",
    "feature_key": "cli-tui",
    "impacted_areas": ["tui-history", "tui-client", "tui-gates", "tui-drive-sync"],
    "upstream_docs": ["SS-07", "CP-56", "SD-14", "SD-15", "Task-069", "BUG-311"],
    "status": "verified"
  }
}
```
