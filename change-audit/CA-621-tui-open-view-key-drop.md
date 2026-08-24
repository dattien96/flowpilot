# CA-621: TUI open with sidebar drops keys (View 100-800ms + log dump)

## What

Opening TUI (`/open` or cold open with history) occasionally made composer impossible: typing produced 0 `KeyMsg` in `tui.log` for 50s, `View slow` 107-839ms repeatedly with `side=true`. Composer looked enabled (`chat`) but no rune arrived — same class as CA-610, not CA-620 `sendBlocked` (`next` blocks Enter only, still logs `KeyMsg`).

## Why

- `Update` logged `ChatListMsg`/`FlowListMsg`/`SessionDefaultsMsg` via `%v` — dumped entire history (hundreds of runs + prompts) into `tui.log` synchronously on the event loop. On open with large history this I/O stalled the loop and filled conhost 64-slot queue.
- `View()` rendered `renderChatPane` + `renderSidebarPane` + `composeCellBuf` every cursor tick (500ms) and thinking tick (90ms) at 126×50. With 50+ steps and sidebar, `View` >100ms each frame; with `workIsLive` true the 90ms spinner kept it hot, queue filled, keys dropped.
- `View slow` log did open/write/close on every slow frame, adding more I/O while already slow.
- No test covered “open with large history + sidebar → View <200ms and keys not dropped”. `ca610_*` only covered hover filter.

## Fix

- **app.go** `Update`: large payloads log summary only — `ChatListMsg n=`, `FlowListMsg builtins=/workflows`, `SessionDefaultsMsg providers/accounts/projects`, `SkillsListMsg n=`, `ChatOpenedMsg run/msgs`, `ProjectContextMsg path/branch`. No body dump.
- **model.go** `AppModel.lastViewSlowLog time.Time` + **app.go** `View()`: throttle `View slow` log to once per 5s (`time.Since >5s`).
- Keeps CA-610 filter, CA-615 clipboard timeout, CA-619/620 park chips/sendBlocked.

Will not undo: CA-610 hover filter + no pulse, CA-612 Ctrl+V reject, CA-615 Alt+V timeout, CA-619 escalate chips, CA-620 sendBlocked park.

## Tests (additive)

- `tui/app/ca621_tui_open_keys_not_dropped_test.go` (agnostic, no provider branch):
  - `TestCA621_ChatListMsg_DoesNotBlockKeys` — 200-item `ChatListMsg` then `a` → `inputValue` has `a`.
  - `TestCA621_View_NotSlowWithManySteps` — 50 steps, 126×50 View <200ms.
  - `TestCA621_ViewSlowLog_Throttled` — View without slow does not bump `lastViewSlowLog`; throttled path checked.
  - `TestCA621_KeysNotDropped_AfterOpen` — rapid `a,b,c` after open → `abc`.
- Old: `go vet ./internal/tui/app`, `go test ./internal/tui/app -run TestCA621|TestRun136749|TestBlockedBar|TestCA620` pass, full suite 21s pass.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: throttle View slow log and summarize large ChatList/Skills payloads so open with sidebar does not spam log nor drop keys (CA-621)
# --->8---
