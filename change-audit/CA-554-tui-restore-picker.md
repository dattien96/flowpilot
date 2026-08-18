# CA-554 - /restore Tab picker lists Drive chats to restore one-by-one

## Problem

CA-550 shipped `filterRestoreSuggestions` (an "all" row + one row per
Drive-backed chat), but in practice the `/restore` Tab picker never appeared:
`cmdMaybePrefetchHistory` returned `nil` as soon as `chatList` was cached
(which is always true after the TUI has been used), so the `remoteChatList`
index stayed empty and `filterRestoreSuggestions` returned nil. The bare
`/restore` dump still worked, but there was no interactive per-chat picker —
users had to run `/restore <n>` blind or `/restore all`.

## Fix (TUI only)

- `internal/tui/app/history.go` (`cmdMaybePrefetchHistory`): the prefetch gate is
  split. When `chatList` is empty, any history/sync/restore trigger fetches
  **both** lists (unchanged CA-552 behavior). When `chatList` is already cached
  but the input is `/restore`/`/restore all`/`/restore <arg>` **and**
  `remoteChatList` is empty, the remote index is still prefetched — so the
  `/restore` Tab picker always opens with data. `/sync` keeps the old gate
  (its rows come from `syncableChats()`, not the remote index).
- `internal/tui/app/app.go` (`collectSuggestions`): a `/restore ` picker
  placeholder mirrors `/history`: `loading Drive chats…` while the index is in
  flight (`remoteChatList == nil`), and `(no Drive-backed chats to restore)`
  once a confirmed-empty fetch returns. `renderSuggestions` now labels the
  `restore` and `sync` picker kinds (`restore:` / `sync:`) instead of the
  generic `commands:`.
- `internal/tui/app/chat_drive_sync.go` (`filterRestoreSuggestions`): each chat
  row now carries the same `#N · kind · status · date · title` detail as the
  `/history` picker (index `#N` is the 1-based position in the remote list; the
  `all` row stays at the top). The accept value remains `machineId:runId` →
  `/restore m1:r1` (unchanged contract, CA-550 tests untouched).

## Files

- `apps/local-runner/internal/tui/app/history.go`
- `apps/local-runner/internal/tui/app/app.go`
- `apps/local-runner/internal/tui/app/chat_drive_sync.go`
- `apps/local-runner/internal/tui/app/ca554_tui_restore_picker_test.go` (new)

## Tests

Additive (8 test funcs): `/restore ` prefetches the Drive index when chatList is
cached; no re-fetch when the remote index is cached; `collectSuggestions`
loading placeholder (nil index) and empty-state placeholder (confirmed empty);
restore picker lists `all` + every Drive chat with `machine:run` values; restore
detail starts `#1 · ` and carries status + title; picker header labels `restore:`
and `sync:`.

## Verification

- Full `go test ./internal/tui/app/ ./internal/tui/client/ -count=1` green except
  the two pre-existing environmental `TestCmdFocusAgent_*` failures.
- gofmt clean (LF-normalized temp copies); new/changed Go files CRLF.
- No old test edited; no runner/desktop/OAuth changes; provider-agnostic
  (restore still POSTs only projectId + source ids + optional cwd).

## Change Ledger

```json
{
  "flowpilot:change-ledger": {
    "source_doc_id": "CP-56",
    "change_type": "bugfix",
    "feature_key": "cli-tui",
    "impacted_areas": ["tui-history", "tui-client", "tui-drive-sync"],
    "upstream_docs": ["SS-07", "CP-56", "SD-14", "SD-15", "Task-069", "BUG-311", "CA-550", "CA-552"],
    "status": "verified"
  }
}
```