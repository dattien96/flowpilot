# CA-550 - TUI /restore pulls Drive-backed chats (batch, cwd remap)

## Problem

The TUI could push chats to Drive (CA-549) but had no way to pull a chat that
was synced on another machine. Desktop Navigator shows a "Restore" action per
Drive-backed session and a bulk "Restore all"; the runner already exposes
`GET /client/projects/{id}/chat-sessions/remote` and
`POST /client/chat-sessions/restore`, but the TUI never called them. A user on a
new machine was stuck unless they opened Desktop.

## Fix (TUI only — G3, closes the G1/G2/G3 TUI Drive parity plan)

- `internal/tui/app/chat_drive_sync.go`:
  - `RemoteChatListMsg` + `cmdFetchRemoteChats(silent)` populate the
    `remoteChatList` cache from the runner index (silent refresh after a batch,
    loud dump for a bare `/restore`).
  - `restoreState` + `RestoreBatchMsg` reuse the G2 no-freeze sequential batch
    pattern: one HTTP per Update, single failure never aborts the rest.
  - `cmdRestoreOne` posts `{projectId, sourceMachineId, sourceRunId, cwd}`; on
    `cwd_remap_required` it retries exactly once with the bound project path
    (Desktop remap parity) — only the first attempt omits cwd.
  - `formatRemoteChatList` dumps the index (`n. title [machine:run] status`);
    `resolveRestoreTarget` accepts 1-based index, `machine:run` key, or local
    runId; `filterRestoreSuggestions` picker with a persistent bulk `all` row.
  - Error mapping: `google_drive_not_connected` → `/settings` hint;
    `session_unavailable` / `sync_remote_not_found` / `sync_integrity_failed`
    surface runner wording (BUG-311 parity).
- `internal/tui/app/app.go`: `case "/restore"` → `runRestoreDispatch`; Update
  handles `RemoteChatListMsg` and `RestoreBatchMsg` — a single restore opens the
  restored chat via `cmdOpenChat` on success (Desktop opens restored session),
  `/restore all` stays silent and on completion refreshes both the local and
  remote lists; `collectSuggestions` + `suggestionAcceptValue` wire the `restore`
  picker kind.
- `internal/tui/app/history.go`: `cmdMaybePrefetchHistory` also prefetches for
  `/restore`, `/restore all` and `/restore `.

## Files

- `apps/local-runner/internal/tui/app/chat_drive_sync.go`
- `apps/local-runner/internal/tui/app/app.go`
- `apps/local-runner/internal/tui/app/history.go`
- `apps/local-runner/internal/tui/app/model.go`
- `apps/local-runner/internal/tui/app/ca550_tui_drive_restore_test.go` (new)

## Tests

Additive (12 test funcs): source key; /restore picker all-row + query filtering +
non-/restore nil + empty nil; suggestionAcceptValue; resolveRestoreTarget matrix
(index/runId/key/out-of-range/no-match/empty); dump empty + populated; dispatch —
bare uses cache when loaded, fetches when empty, single opens after (cwd pre-bound
from project path), all batch queue, no-project no-op; batch state machine —
single failure continues, single success opens chat, all-done refreshes lists;
error hints; `cmdMaybePrefetchHistory` triggers on `/restore`.

## Verification

- Full `go test ./internal/tui/app/ ./internal/tui/client/ -count=1` green except
  the two pre-existing environmental `TestCmdFocusAgent_*` failures.
- gofmt clean (LF-normalized temp copies); new Go files CRLF; `go vet` clean.
- Provider-agnostic (cross-provider-parity Case 1): restore POSTs only
  `projectId`+source ids+optional `cwd`; no providerKey branch. No old test
  edited; `chatOpenSlashCommands`, `parseChatOpenArgPrefix`, G1/G2 surface
  untouched.

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
