# CA-548 - TUI history rows carry no Drive chat-session sync status

## Problem

The TUI client had no surface for the runner's Drive chat-session sync/restore
endpoints, and `/history` rows never showed whether a chat was already in Drive
(Desktop Navigator shows `synced`/`failed`/`unsyncable` per row). RunHistoryItem
was stripped down to no sync metadata, so any Drive-aware TUI flow had nothing to
render or reconcile against.

## Fix (TUI only — G1 of the TUI Drive parity G1/G2/G3 plan)

- `internal/tui/client/client.go`: `RunHistoryItem` gains the Desktop-parity sync
  fields (`sourceMachineId`, `sourceRunId`, `syncStatus`, `unavailableReason`,
  all `omitempty` — old composite literals still compile). New DTOs mirror the
  runner contract (`chat_session_sync.go:134-174`): `ChatSessionSyncRequest/Result`,
  `ChatSessionRestoreRequest/Result`, `RemoteChatSessionSummary`. New client
  methods `SyncChatRun`, `ListRemoteChatSessions`, `RestoreChatRun` hit the same
  three endpoints Desktop uses.
- `internal/tui/app/history.go`: `formatSyncBadge` maps `syncStatus` to a badge;
  `formatChatList` appends `(synced|failed|unsyncable|syncing)` after the title
  only when non-empty, so local-first rows render unchanged.
- `internal/tui/app/helpers.go`: `filterHistorySuggestions` includes the badge in
  the picker `detail` and adds `syncStatus` to the filter haystack (typing
  `failed` narrows to failed rows). `value`/`slash` contract untouched.

## Files

- `apps/local-runner/internal/tui/client/client.go`
- `apps/local-runner/internal/tui/app/history.go`
- `apps/local-runner/internal/tui/app/helpers.go`
- `apps/local-runner/internal/tui/client/client_drive_sync_test.go` (new)
- `apps/local-runner/internal/tui/app/ca548_tui_drive_sync_status_test.go` (new)

## Tests

Additive. Client: RunHistoryItem decodes drive metadata; SyncChatRun posts
`googleDriveProjectId` to `/client/workflow-runs/{id}/sync-chat`; list remote
path + decode; restore posts the full request; APIError surfaces the code with
the real runner error shape `{"error":{"code","message"}}`. App: badge mapping
(empty/local_only stays blank); dump keeps titles and shows `(synced)`/`(failed)`
without leaking `()`; picker detail carries the badge and the haystack filters on
it while preserving value+slash.

## Verification

- `go test ./internal/tui/app/ ./internal/tui/client/ -count=1` green except the
  two pre-existing environmental `TestCmdFocusAgent_*` failures in `tui/app`.
- gofmt clean (LF-normalized temp copies); new Go files CRLF; `go vet` clean.
- Provider-agnostic (cross-provider-parity Case 1): new client methods take only
  `runID`/`projectID`, no `providerKey` branch anywhere — the runner already
  handles Claude/Codex/Grok Drive sync.

## Change Ledger

```json
{
  "flowpilot:change-ledger": {
    "source_doc_id": "CP-56",
    "change_type": "bugfix",
    "feature_key": "cli-tui",
    "impacted_areas": ["tui-history", "tui-client", "tui-gates"],
    "upstream_docs": ["SS-07", "CP-56", "SD-14", "SD-15", "Task-069"],
    "status": "verified"
  }
}
```
