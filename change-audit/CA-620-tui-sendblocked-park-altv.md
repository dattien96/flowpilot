# CA-620: TUI composer blocked on park (ConnWaiting + sendBlocked) — Alt+V appeared broken

## What

After CA-619 the TUI showed `[Continue]/[Stop]` for `blocked: escalate` `WAITING_USER_APPROVAL` (run-136749), but `sendBlocked()` was still `true` due to `ConnWaiting` set by `showBlockedBanner`. Composer appeared with prefix `next` and Enter toasted “A turn is already in progress — wait…”. Typing worked if keys arrived (handleKey inserts before sendBlocked), but Enter was rejected, so `/continue` via typing needed the chip. In the same session Alt+V produced 0 `KeyMsg` in `tui.log` (pid 3112 14:20) — not a CA-615 clipboard hang (`alt+v` never logged), likely conhost key drop or stale binary, not CA-619 diff.

## Why

- `showBlockedBanner` (BUG-231) has always set `connStatus = ConnWaiting` (“awaiting your decision”).
- `sendBlocked()` returned `true` for `ConnRunning` **or** `ConnWaiting`, regardless of park. A park is not a turn in flight — it is waiting on the user.
- `processInput` checks slash `/continue` **before** `sendBlocked`, so `/continue` via slash still unblocked, but normal Enter was blocked with the wrong toast. The blocked bar was correct after CA-619, the send gate was not.
- `handleKey` Alt+V (`alt+v` → `cmdClipboardPaste`) is dispatched in `handleKey` before any paste-burst or send logic, so `sendBlocked` does not affect Alt+V itself. A 0-KeyMsg session is CA-610 key drop (64-slot conhost + 107ms View) or stale binary, not a clipboard timeout.
- No existing test asserted `blocked + ConnWaiting` → `!sendBlocked` together with Alt+V.

## Fix

- **app.go** `sendBlocked()`: return `false` immediately when `flowLoopBlocked()` (escalate/cap/delegate_failed park with `WAITING_USER_APPROVAL` stamp, no YOLO/ask_user/unresolved attention). Park input stays enabled for `/continue` / chip; a real `ConnRunning` turn without park remains blocked. Added comment linking to run-136749.
- Keeps `hasLiveWorkingChild`/`flowLoopBlocked` (CA-619) for Thinking/[stop] and bar. No change to `rejectWindowsRawPaste`, `tuiMsgFilter`, or clipboard timeout (CA-615).
- YOLO (`approval`/`question`/`gate`) and unresolved attention still block as before — `flowLoopBlocked()` is false when they are present, so `sendBlocked` stays true.

Will not undo: CA-619 chip/Thinking, CA-610 filter, CA-612 Ctrl+V reject, CA-615 native clipboard timeout, BUG-231 banner.

## Tests (additive)

- `tui/app/run136749_composer_not_sendblocked_test.go` (agnostic, 1 representative, no old edit):
  - `TestCA620_BlockedPark_DoesNotBlockSend` — `blocked: escalate` + `WAITING` + `ConnWaiting` → `!sendBlocked`, `/continue` slash returns cmd.
  - `TestCA620_BlockedPark_StillShowsChipsAndNoThinking` — chips present, `!workIsLive`, `!turnIsActive`.
  - `TestCA620_NormalTurnStillBlocksSend` — `ConnRunning` without park → `sendBlocked` true (no regression).
  - `TestCA620_AltV_StillDispatchesWhenParkBlocked` — Alt+V → `tea.Cmd` non-nil, no literal `v` inserted, even when park.
  - `TestCA620_AltV_DoesNotNeedSendBlocked` / `TestCA620_YOLOApprovalStillBlocksSend` — YOLO still blocks.
- Old: `go vet ./internal/tui/app`, `go test ./internal/tui/app -run TestCA620|TestRun136749|TestBlockedBar|TestRun127174|TestCA618|TestStatusSpinner` pass, full suite 21s pass.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: composer not blocked on park (ConnWaiting) — sendBlocked respects flowLoopBlocked so /continue and Alt+V stay enabled (CA-620)
# --->8---
