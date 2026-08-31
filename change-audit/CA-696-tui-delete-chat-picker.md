# CA-696 — TUI /delete picker with multi-select and delete-all (Task-318)

## Problem

TUI had no way to prune persisted chats (`/clear` only clears the on-screen transcript, no `DELETE`). The first Task-318 implementation wired `DELETE /client/workflow-runs/{runId}` via a single-select `/delete` picker but `Enter` never fired: the list was never prefetched for `/delete` (stuck on `loading chats…`) and `Tab` filled the input instead of ticking, unlike `/skill`'s skill-picker UX. Users also needed batch delete and a `delete all` affordance.

## Change

- **Client** (`tui/client/client.go`): `DeleteRun(ctx, runID) error` — `DELETE /client/workflow-runs/{runId}` via `methodJSON` with `PathEscape` and `*APIError` propagation.
- **Model** (`tui/app/model.go`): `knownSlashCommands` `/delete` → `Delete chats — type /delete  then Tab tick · Enter del · all`; `deleteSelected map[string]bool`, `deletePendingIDs/RunID/Label`, `deleteBatchQueue/Total`.
- **Helpers** (`tui/app/helpers.go`): `parseDeleteArgPrefix`, `filterDeleteSuggestions*`/`WithSelected` (`kind="delete"`, `slash="/delete"`) — `[ ]`/`[*]` ticks, top `all` row (`[ ] all · delete all N` → `[*]` when all ticked), `q=="all"` bypasses title filter.
- **History** (`tui/app/history.go`): `ChatDeletedMsg`, `cmdDeleteChat` (15s), `handleChatDeleted` (optimistic `chatList` filter + tick clear, BUG-258 current-chat reset, auto-dispatches `deleteBatchQueue` sequentially, continues on per-item error, final `Deleted N chats.`), `toggleDeleteSelection` (single + `all` toggle), `retargetDeleteSuggestion`.
- **App** (`tui/app/app.go`): `collectSuggestions` delete branch after history with ticks; `cmdMaybePrefetchHistory` now prefetches for `/delete`; `suggestionVisibleLimit`/`renderSuggestions` treat `delete` like `history`; `Tab`/`Shift+Tab` toggle `delete` ticks (skill-like, stays open); `Enter` on `delete` arms batch (`tick` > `all` > highlighted); slash `/delete <n|runId|all>` and bare `/delete` (open chat) arm `deletePendingIDs`; pending `y`/`Enter` starts sequential batch (`deleteBatchQueue`), `n`/`Esc` cancels; `Update` `ChatDeletedMsg` + logging; `Esc` on `/delete` input clears ticks.
- **Docs** (`tui/README.md`): `/delete` row → `Tab tick · Enter del · all`.
- **Tests** (additive, no pre-existing edits): `tui/client/delete_run_test.go` (escaped path + error), `tui/app/delete_chat_test.go` (14: registered, all+tick picker, Tab single/all, Enter highlighted/batch/all, slash 2/all/bare, Enter arm, pending y/batch-y/Enter/n/Esc/swallow, chatDeleted single/other/batch/error).

## Tests

- `go vet ./internal/tui/...` clean
- `go test ./internal/tui/... -count=1` PASS (`app` ~11s, `client` ~38s, boundary `TestPackageBoundary_TuiDoesNotImportRunnerInternals` PASS, 14 new delete cases PASS)

## Out of scope

Drive-synced copy deletion remains deferred (Task-077 parity).

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: Task-318
change_type: feature
summary: TUI /delete multi-select picker with tick, batch and delete-all (fix Enter-no-op prefetch)
# --->8---
