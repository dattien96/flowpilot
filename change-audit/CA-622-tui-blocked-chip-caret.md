# CA-622: TUI blocked [Continue]/[Stop] click swallowed by caret

## What

Blocked park (escalate/cap/delegate_failed) showed `[Continue]/[Stop]` chips (CA-619) and `sendBlocked` was correct (CA-620), but clicking the chips did nothing via the normal Update path: `tui.log` showed occasional `target=""` while `dispatchMouseClick` alone worked. `tryPlaceInputCursor` was swallowing the press.

## Why

- `renderInputLine` appends `renderBlockedBar()` (2 lines) and `renderAttentionBar()` inside the input block (`inner` before body). Chips are *inside* the input chrome.
- `handlePlainLeftMouse` (press path) calls `tryPlaceInputCursor` **before** `dispatchMouseClick`. If it returns true, the click is treated as caret placement and the chip never sees `dispatchMouseClick`.
- `tryPlaceInputCursor` excluded `hitAttachChrome`, `hitApprovalChrome`, `hitQuestionChrome`, but **not** `hitBlockedChrome` / `hitAttentionChip`.
- `innerLead` only counted approval/question (1+1). The 2 blocked lines (+ N attention lines) were counted as body, so a click on the blocked bar hit `bodyIdx` 0 and returned true.
- Direct `dispatchMouseClick` tests (e.g. `TestBlockedBar_ClickContinueUnparks`) missed the press path, so the bug was not covered.

The loop was `blocked` and the banner was correct; only the mouse press path was dead.

## Fix

- **tui/app/chat_input_cursor.go** `tryPlaceInputCursor`:
  - After approval/question, also `if m.hitBlockedChrome(c,x,y)!="" return false` and `if m.hitAttentionChip(x,y)!="" return false`.
  - `innerLead` now mirrors `renderInputLine` order: `approval` (+1), `question` (+1), `renderAttentionBar()` (+ lines), `renderBlockedBar()` (+ lines). Requires `strings.Count`.
- No change to `handlePlainLeftMouse`, `hitBlockedChrome`, `dispatchMouseClick`; no change to `flowLoopBlocked`.

Keeps `TestBlockedBar_*` contract (running child still hides chips) and YOLO/ask_user chrome.

## Tests (additive, no old edit)

- `tui/app/ca622_blocked_caret_test.go` (provider-agnostic: grep `ProviderKey` on `tryPlaceInputCursor`/`hitBlockedChrome` 0 hits — one representative would pass, but table-tested for guard):
  - `TestCA622_BlockedContinueViaUpdate_NotCaret` — table `claude/codex/grok` via `Update(clickLeft)` on `[Continue]` → `tea.Batch(pulse, cmdContinueFlow)` POSTs `/agent-loop/continue`; `tryPlaceInputCursor` false on chip.
  - `TestCA622_BlockedStopViaUpdate_NotCaret` — same for `[Stop]` → `StoppedMsg`.
  - `TestCA622_BodyClickStillPlacesCaret` — body click remains caret-placeable; chip click not.
  - `TestCA622_AttentionChipNotCaret` — attention chip also excluded from caret.

Old: `go vet ./internal/tui/app` pass; `go test ./internal/tui/app -run TestCA622|TestBlockedBar|TestRun136749` pass; full `go test ./internal/tui/app` 22s pass.

Provider parity: **Case 1 agnostic** — `tryPlaceInputCursor`, `hitBlockedChrome`, `hitAttentionChip` take no `ProviderKey` and never branch on one (`grep -n ProviderKey chat_input_cursor.go mouse.go attention.go` 0 hits on changed symbols). Verified with table over `claude/codex/grok`.

Will not undo: CA-619 chip rendering & Thinking off, CA-620 `sendBlocked` park, CA-610/615 clipboard, `TestBlockedBar_NoChipsWhenRunningChild`.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: blocked [Continue]/[Stop] click not swallowed by caret — tryPlaceInputCursor excludes blocked/attention chrome and counts their innerLead lines (CA-622)
# --->8---
