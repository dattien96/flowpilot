# CA-552 - Drive (synced) badge survives restart + never re-targets synced chats

## Problem

After a successful `/sync` the TUI showed the badge in-session, but a restart
lost it (screenshot: `/open` picker, 56 chats, zero `· synced` rows). Root
cause: the runner writes `SyncStatus="synced"` to its local store asynchronously
(`updateLocalSessionSyncStatus` silently no-ops when `GetProviderSession` misses,
and the write can lag / be lost across a restart), so `/history` returns
`syncStatus=""` for runs that are actually in Drive. The badge had no fallback
beyond the local flag.

## Fix (TUI only — mirrors Desktop navigatorHistory.ts reconcile behavior)

- `internal/tui/app/history.go`:
  - `syncBadgeWithRemote(it, remote)` — `formatSyncBadge` plus Drive-index
    reconciliation: a local marker (synced/failed/unsyncable/syncing) always
    wins; otherwise a row whose `sourceMachineId:sourceRunId` pair (or local
    `runId`, for a first-machine run whose local fields were never stamped)
    appears in `remoteChatList` is shown as `synced`.
  - `formatChatListWithRemote` (dump) — `formatChatList` delegates with `nil`;
    appends `(synced)` for reconciled rows.
  - `mergeChatListSyncStatus(cached, fresh)` — carries known local sync markers
    across a fresh runner fetch so a prefetch/poll racing the async store write
    can't wipe a just-set `synced`/`failed`; a fresh non-empty status wins.
- `internal/tui/app/helpers.go`: `filterHistorySuggestionsWithRemote` — badge is
  placed **before the title** (`#4 · chat · completed · Aug 17 · synced · title`)
  so it can't be pushed off by a long title; old `filterHistorySuggestions`
  delegates with `nil`.
- `internal/tui/app/chat_drive_sync.go`: `filterSyncSuggestionsWithRemote` — the
  `/sync` picker shows reconciled badges so an already-synced chat is never
  re-targeted; old `filterSyncSuggestions` delegates with `nil`.
- `internal/tui/app/app.go`: `ChatListMsg` (silent + loud) merges sync markers
  and the loud dump renders `formatChatListWithRemote(msg.Items,
  m.remoteChatList)`; `collectSuggestions` uses the `WithRemote` variants.
- `internal/tui/app/history.go`: `cmdMaybePrefetchHistory` batches the remote
  index fetch alongside the chat list, so the picker reconciles on first open.

## Files

- `apps/local-runner/internal/tui/app/history.go`
- `apps/local-runner/internal/tui/app/helpers.go`
- `apps/local-runner/internal/tui/app/chat_drive_sync.go`
- `apps/local-runner/internal/tui/app/app.go`
- `apps/local-runner/internal/tui/app/ca552_drive_sync_badge_reconcile_test.go` (new)

## Tests

Additive (12 test funcs): reconcile from remote when local empty; pair match;
local status (failed/unsyncable) wins over remote; no-remote/no-match empty;
dump shows reconciled badge; history-picker badge precedes the title; /sync
picker reconciled badge; merge preserves local synced on fresh-empty, fresh
status wins, unrelated rows never mutated; silent refetch keeps the synced
marker; loud dump reconciles the cached remote index.

## Verification

- Full `go test ./internal/tui/app/ ./internal/tui/client/ -count=1` green except
  the two pre-existing environmental `TestCmdFocusAgent_*` failures.
- gofmt clean (LF-normalized temp copies); new Go files CRLF; `go vet` clean.
- No old test edited; no runner/desktop/OAuth changes; provider-agnostic.
- Runner `updateLocalSessionSyncStatus` silent no-op left untouched (out of
  TUI scope — optional follow-up).

## Change Ledger

```json
{
  "flowpilot:change-ledger": {
    "source_doc_id": "CP-56",
    "change_type": "bugfix",
    "feature_key": "cli-tui",
    "impacted_areas": ["tui-history", "tui-client", "tui-drive-sync"],
    "upstream_docs": ["SS-07", "CP-56", "SD-14", "SD-15", "Task-069", "BUG-309", "BUG-311", "CA-548"],
    "status": "verified"
  }
}
```