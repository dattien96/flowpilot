# Task-318: TUI Delete Chat History — Picker + Confirm (Desktop Parity)

## Metadata

- Document ID: `Task-318`
- Title: `TUI Delete Chat History — /delete picker + multi-select + delete-all`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-31`
- Last Updated: `2026-08-31`
- Parent Documents: [Task-077: Delete Chat History](../../08-Task/done/Task-077-Delete-Chat-History.md), [CA-100: delete-chat-history](../../../change-audit/CA-100-delete-chat-history.md)
- Child Documents: `None`
- Related Documents: [CA-102: codex rollout cleanup](../../../change-audit/CA-102-fix-delete-chat-codex-rollout-cleanup.md), [Navigator delete UI](../../08-Task/done/Task-077-Delete-Chat-History.md)
- Replaces: `None`
- Tags: `tui, cli-tui, chat-history, delete, picker`

## AI Quick View

### Summary

- TUI currently cannot delete persisted chats: `/clear` only clears the on-screen transcript, `/history|/open|/resume` only opens. Backend `DELETE /client/workflow-runs/{runId}` already exists (Task-077/086, cascade child deletes, otel/NDJSON atomic rewrite) and Desktop deletes via `client.deleteRun`. This task wires the TUI to that endpoint. **v2** fixes the Enter-no-op bug and upgrades UX to skill-like multi-select.
- Adds `Client.DeleteRun(runID)` and slash `/delete` with picker `m.chatList` (`slash="/delete"`, `kind="delete"`). `/delete ` shows searchable list with `[ ]`/`[*]` ticks and a top row `[ ] all · delete all N chats`. **Tab** toggles tick (picker stays open, like `/skill`); **Enter** deletes ticked set, or highlighted row, or `all`. Typed `/delete <n|runId|all>` also arms. `y`/`Enter` confirms batch (sequential `DELETE`s, open-chat reset via BUG-258 parity, remaining queue continues even on per-item error), `n`/`Esc` cancels. No Drive-copy delete.

### Current Ask

- Implement the additive TUI delete path (client + model + helpers + app) and verify with new `delete_chat_test.go` only — do not edit pre-existing tests.

### Key Decisions

- `T-1` **Dedicated `kind="delete"` picker (not `history`)** — fixes Enter-no-op: `filterDeleteSuggestionsWithSelected` returns `kind="delete"` + `slash="/delete"` with an `all` row and `[ ]`/`[*]` ticks driven by `m.deleteSelected`. `collectSuggestions` checks delete after history, `suggestionVisibleLimit` and placeholder rows treat `delete` like `history`/`skill` (12 rows). `cmdMaybePrefetchHistory` now prefetches for `/delete` so the list is populated on first `/delete ` (root cause of the loading-stuck bug).
- `T-2` **Skill-like multi-select** — `Tab` on a `delete` row toggles `m.deleteSelected[runID]` (or all ids when `value=="all"`), stays in picker and retargets highlight (`retargetDeleteSuggestion`), like `toggleSkillByNameQuiet`. `Enter` on the picker collects ticked ids in `chatList` order, or highlighted `all`/single when nothing ticked, arms `deletePendingIDs` + `deletePendingRunID`, clears input and asks `Delete N chats? [y]/[n]` (single shows `Delete "label" (run-xxx)?`). While pending, `handleKey` swallows normal input until `y`/`Enter` → batch start, `n`/`Esc` → cancel.
- `T-3` **Batch sequential delete** — `y` sets `deleteBatchQueue = remaining`, `deleteBatchTotal = len(ids)`, clears `deleteSelected`, dispatches first `DELETE`. `handleChatDeleted` drops the row, clears its tick, resets current chat when `runID==openID` (BUG-258 parity), then auto-dispatches next queued id; on error it still continues the queue. Final `Deleted N chats.` when total>1 and queue empty. Esc while pending clears pending/batch but keeps ticks (so user can retick).
- `T-4` **Delete-all as first-class** — `all` row (`value="all"`, `kind="delete"`) always present, `[ ]`/`[*]` reflects all-ticked, and typed `/delete all` arms all ids. Unlike Task-077 deferral, TUI now offers batch delete-all because the picker makes it safe behind the `y` gate.

### Constraints

- Additive tests only — do not edit `cp56_*`, `chat_ux_*`, `history_*`, `app_*` pre-existing tests.
- Do not touch `internal/runner/deleteChatSession` or Desktop files.
- `tui/client` must not import `internal/runner` (A9.1 package boundary).

### Open Questions

- None.

### Source Refs

- `apps/local-runner/internal/runner/interactive_resume.go:26` (`deleteChatSession`)
- `apps/local-runner/internal/runner/interactive_handlers.go:262` (`handleDeleteRun`)
- `apps/local-runner/internal/tui/client/client.go:835` (`ListRunHistory`), `client.go:1365` (`methodJSON`)
- `apps/local-runner/internal/tui/app/model.go:650` (`knownSlashCommands`), `app.go:3659` (slash switch), `helpers.go:845` (`chatOpenSlashCommands`), `history.go:16` (`ChatListMsg`)

---

## 1. Goal

Let the TUI delete persisted chats via `/delete` with a skill-like picker: tick 1..N rows with Tab (including an `all` row), Enter deletes ticked/highlighted, `y` confirms batch, `n` cancels. The runner removes NDJSON + provider files + in-memory state; deleting the open chat resets like `/new` (BUG-258 parity).

## 2. Parent Links

- coding plan: none (bug/Ux gap, not a CP)
- tech design: `requirements/10-Refactor/New-System/09-Cross-Account-Chat-Resume-Implementation-Guide.md` (session lifecycle)
- system spec: none (delete lifecycle spec implied)
- specific upstream ids: Task-077 `T-3`/`T-4`, `DELETE /client/workflow-runs/{runId}` contract

## 3. Trigger

TUI has no mechanism to prune chat history. `/clear` only clears the transcript on screen, not the saved chats. The history list grows unbounded and the only way to delete is Desktop's Navigator `×`.

## 4. Exact Change

- `T-1` `client/client.go` — `DeleteRun(ctx, runID) error` → `DELETE /client/workflow-runs/{runId}` via `methodJSON` (escaping, `*APIError` propagation).
- `T-2` `app/model.go` — `knownSlashCommands` `/delete` → `Delete chats — type /delete  then Tab tick · Enter del · all`; `AppModel` fields `deleteSelected map[string]bool`, `deletePendingRunID/IDs/Label`, `deleteBatchQueue/Total`.
- `T-3` `app/helpers.go` — `parseDeleteArgPrefix`, `filterDeleteSuggestions`/`WithRemote`/`WithSelected` (`kind="delete"`, `slash="/delete"`) — `[ ]`/`[*]` ticks, top `all` row (`[ ] all · delete all N`), `q=="all"` bypasses title filter so the bulk row stays visible while typing `all`.
- `T-4` `app/history.go` — `ChatDeletedMsg`, `cmdDeleteChat`, `handleChatDeleted` (success drops from `chatList` + tick, resets current chat when `runID==openID` (BUG-258), auto-dispatches `deleteBatchQueue`, final `Deleted N chats.` when total>1, continues on per-item error), `toggleDeleteSelection` (single + `all` toggle), `retargetDeleteSuggestion`, `chatLabelForRunID`.
- `T-5` `app/app.go` — `collectSuggestions` delete branch after history (`filterDeleteSuggestionsWithSelected` with ticks), placeholder rows `kind="delete"`; `cmdMaybePrefetchHistory` now prefetches for `/delete`; `suggestionVisibleLimit`/`renderSuggestions` treat `delete` like `history` (12 rows, header `delete`); `handleKey` Tab toggles `delete` ticks (skill-like, `Shift+Tab` also), `handleKey` Enter on `delete` collects ticked/highlighted/`all` ids, arms `deletePendingIDs` and asks `Delete N?`; typed `/delete <n|runId|all>` and bare `/delete` (open-chat) also arm pending; pending `y`/`Enter` starts sequential batch (`deleteBatchQueue`/`Total`, clears `deleteSelected`), `n`/`Esc` cancels.
- `T-6` `app/app.go` — `Update` `ChatDeletedMsg` → `handleChatDeleted`; logging `ChatDeletedMsg`; `KeyEscape` clears input and `deleteSelected` when input was `/delete`.
- `T-7` `app/README.md` — `/delete` row → `Delete chats — type /delete  then Tab tick · Enter del · all`.
- `T-8` Tests (new files only): `client/delete_run_test.go` (escaped path + error), `app/delete_chat_test.go` (14 tests: command registered, all+tick picker, Tab single/all toggle, Enter highlighted/batch/all, slash `2`/`all`/bare, Enter suggestion arm, pending y/Enter/n/Esc/swallow, batch-y, chatDeleted single/other/batch/error).

## 5. Touched Areas

- files: `tui/client/client.go`, `tui/client/delete_run_test.go` (new), `tui/app/model.go`, `tui/app/helpers.go`, `tui/app/history.go`, `tui/app/app.go`, `tui/app/delete_chat_test.go` (new), `tui/README.md`
- modules: TUI chat client, bubbletea app model, slash command dispatch, suggestion picker
- routes consumed: `DELETE /client/workflow-runs/{runId}` (existing)
- tables: none (runner owns `sessions.ndjson` rewrite)

## 6. Acceptance Check

- [x] `/delete` appears in `/help` and `knownSlashCommands`.
- [x] `/delete ` shows ticks `[ ]`/`[*]` + top `all` row; Tab toggles tick (all toggles all), picker stays open (skill-like); ↑↓ navigates; `/delete` prefetches `chatList` (Enter no longer stuck on `loading chats…`).
- [x] `Enter` on picker with no tick → deletes highlighted single/`all`; with ticks → deletes ticked batch in `chatList` order; all cases arm pending `Delete "…"?` / `Delete N chats?` and wait for `y`/`Enter`.
- [x] Typed `/delete <n|runId>` / `/delete all` / bare `/delete` (open chat) arm pending without picker, same `y` gate.
- [x] `y`/`Enter` while pending starts sequential `DELETE`s; per-item row dropped + tick cleared; current-chat delete resets transcript/panel (BUG-258); batch auto-continues and finishes with `Deleted N chats.`; error per-item continues queue (no row drop).
- [x] `n`/`Esc` while pending shows `Delete cancelled.` and keeps `chatList`; ticks survive cancel, `Esc` on bare input clears `deleteSelected`.
- [x] Existing `go test ./internal/tui/...` still green; no pre-existing test edited; new tests are additive only (14 delete cases).

### 6.1 Definition of Done (DOD)

- [x] `DOD-1` Client `DeleteRun` round-trips via httptest (200 + error path).
- [x] `DOD-2` Picker shows `all` row + tick parity (`[ ]`/`[*]`, all-ticked reflects `deleteSelected`).
- [x] `DOD-3` Tab toggles single/all; Enter arms pending batch (no immediate `DELETE`).
- [x] `DOD-4` Slash ` /delete 2|runId|all` arms pendingIDs correctly.
- [x] `DOD-5` `y`/`Enter` starts sequential batch; `ChatDeletedMsg` queues next; current-chat reset parity.
- [x] `DOD-6` `all` bulk delete available via picker and typed `all`, behind `y` gate.

### 6.2 Test Signatures

```go
// tui/client/delete_run_test.go
func TestDeleteRun_SendsDeleteToWorkflowRuns(t *testing.T)
func TestDeleteRun_PropagatesAPIError(t *testing.T)

// tui/app/delete_chat_test.go (14)
func TestKnownSlashCommands_IncludesDelete(t *testing.T)
func TestFilterDeleteSuggestions_IncludesAllAndTicks(t *testing.T)
func TestDelete_TabTogglesSingleAndAll(t *testing.T)
func TestDelete_EnterWithNoSelectionDeletesHighlighted(t *testing.T)
func TestDelete_EnterWithSelectionDeletesBatch(t *testing.T)
func TestDelete_EnterOnAllRowDeletesAll(t *testing.T)
func TestHandleSlash_DeleteArmsPendingNotDirectDelete(t *testing.T)
func TestHandleSlash_DeleteAllArmsPending(t *testing.T)
func TestHandleSlash_DeleteNoArgWithOpenArmsCurrent(t *testing.T)
func TestHandleSlash_DeleteNoArgNoOpenShowsUsage(t *testing.T)
func TestEnter_AcceptsDeleteSuggestionArmsPending(t *testing.T)
func TestHandleKey_PendingDeleteYDispatchesDelete(t *testing.T)
func TestHandleKey_PendingBatchYDispatchesFirst(t *testing.T)
func TestHandleKey_PendingDeleteNCancels(t *testing.T)
func TestHandleChatDeleted_BatchQueuesNext(t *testing.T)
```

## 7. Out of Scope

- `DELETE /client/workflow-runs/{runId}` server changes (already ships, cascade handled).
- Drive-synced copy deletion (`chat-sessions/runs/<machine>/<runId>/` on Google Drive) — same deferral as Task-077.
- Undo/soft-delete, Supabase `DeleteProviderSession` parity, Desktop Navigator changes.

## 8. Completion Notes

- result: Implemented and verified — `go vet ./internal/tui/...` clean, `go test ./internal/tui/...` PASS (app ~11s, client ~38s). Additive tests only: `tui/client/delete_run_test.go` (2), `tui/app/delete_chat_test.go` (14). Pre-existing suite still green. Enter-no-op fixed by prefetch + `kind="delete"`.
- follow-ups: None — batch delete-all now ships via picker + typed `all` behind `y` gate; Drive-copy delete still deferred.
- upstream docs updated: `apps/local-runner/internal/tui/README.md` Slash Commands table (`/delete` → `Tab tick · Enter del · all`). No SD/CP change required — additive delta, same `DELETE` contract as Task-077/086.
