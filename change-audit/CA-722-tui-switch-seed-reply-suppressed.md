# CA-722 — TUI switch: seed turn reply suppressed + synchronous divider + in-flight Tab message

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: BUG-347
change_type: bugfix
summary: after a provider switch, render the switch divider synchronously on commit and suppress the seed turn's envelope reply everywhere (live stream, replay, timeline backfill) so no orphan assistant bubble appears; Tab during chatSwitchInFlight now says "switch in progress" instead of the question/approval message
# --->8---

## Problem

- Live run `cht_e4975cb1f769` (B5 double-Tab): sau switch opencode→grok, seed
  turn (fire-and-return, `prompt:""` trong transcript) để lại **assistant
  bubble mồ côi** "Chào bạn! Mình là Grok 4.5…" — không có You-box. Đồng thời
  **không có divider** nào render live (live path không bao giờ add user
  message từ event → `addMessage`'s lastSwitchStats path chết).
- Tab trong lúc `chatSwitchInFlight` hiện *"Cannot switch provider/model while
  a question or approval is pending"* — không có question/approval mounted.

## What changed (TUI only, runner seed dispatch không đổi)

- `chat_switch.go` `applyChatSwitched` (success): `seedTurnActive=true` +
  `addMessage("system", switchDividerContent())` synchronous — divider luôn
  xuất hiện (D-7 deterministic), stats consumed ngay.
- `app.go` `handleEvent`: seed guard drop `message_delta`/`message_completed`;
  `turn_completed` clear guard + không re-append FinalMessage (`wasSeed`);
  `turn_failed` clear; `turn_started` real prompt disarm. Helper
  `switchDividerContent()` dùng chung với conversion cũ trong `addMessage`.
- `history.go` `replayHistoryMessages`: `turn_started` prompt rỗng = seed →
  `skipNextAssistant` (trước đây chỉ khớp handoff-prefix — transcript ghi
  prompt rỗng nên không khớp).
- `chat_switch.go` `renderChatTimelineBackfill`: skip-leg — seed
  turn_started (rỗng/handoff-prefix) đánh dấu leg; message_completed trên leg
  đó bị drop cho tới real turn.
- `chat_posture.go`: tách busy message — `chatSwitchInFlight` →
  "Provider switch in progress — wait for it to finish, then Tab again";
  question/approval giữ message cũ.

## R1 — old-suite regression evidence

- Operator-approved updates (contract mới):
  - `TestAdoptKeepsTranscriptAndResetsStreamState`: success giờ +1 divider.
  - `TestRenderChatTimelineBackfill_SkipsSeedPromptKeepsDivider` → renamed
    `…_DropsSeedPromptReplyKeepsDivider`: seed reply bị drop, chỉ E-9 divider.
- Full `go test ./internal/tui/... -count=1` → ok (app 7.2s).
- Runner: `go test ./internal/runner/ -run 'TestChatSwitch|TestSwitch|TestChatTranscript|TestSwitchMatrix'` → ok.

## R2 — provider parity

- Toàn bộ là UI layer (handleEvent/messages), không branch theo provider —
  Claude/Codex/Grok đều có divider + không bubble mồ côi.

## R3 — new coverage (`bug347_seed_turn_suppress_test.go`)

- Live: divider synchronous, seed delta/completed/FinalMessage drop, guard
  clear, real turn render.
- Real turn_started disarm (seed failed/stop race).
- Busy message: in-flight Tab nói "switch in progress", không "question or
  approval".
- Replay: empty-prompt seed reply drop, real turns sống.
- Backfill: seed reply không vào timeline, E-9 divider + real turns có.

## Honest gaps

- Runner transcript vẫn ghi seed `turn_started{prompt:""}` + reply (SD-26 D-6
  muốn payload mang isHandoffSeed + counts — chưa implement); TUI drop ở
  render, không sửa nguồn. Nếu sau này runner ghi prompt đầy đủ, replay/backfill
  vẫn khớp nhờ nhánh handoff-prefix.
- Chưa verify live lại sau fix (cần operator rebuild + switch thật).

## Prior CA not undone

- CA-720/721 (question chip highlight, drag highlight) intact.
- SD-26 D-7 single-source divider: text vẫn từ `lastSwitchStats` (E-9 trên
  replay); chỉ chuyển thời điểm render live sang synchronous.