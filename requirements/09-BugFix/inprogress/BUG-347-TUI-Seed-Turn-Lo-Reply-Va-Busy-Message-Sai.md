# BUG-347: TUI sau switch — seed turn lộ reply thành chat + busy message sai khi Tab in-flight

## Metadata

- Document ID: `BUG-347`
- Title: `TUI sau switch — seed turn lộ reply thành chat + busy message sai khi Tab in-flight`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-02`
- Last Updated: `2026-09-02`
- Feature Keys: `cli-tui`
- Parent Documents: [CP-59: Chat SSOT](../../07-Coding-Plan/inprogress/CP-59-Test-Steps.md)
- Child Documents: `none`
- Related Documents: [SD-26](../../06-System-Tech-Design/SD-26-Chat-Continuity-Ssot.md), [CA-721](../../../change-audit/CA-721-tui-live-drag-highlight-1002h.md)
- Replaces: `none`
- Tags: `cli-tui, chat-switch, seed-turn, divider, busy-message`

## AI Quick View

### Summary

- Sau switch provider, seed turn chạy fire-and-return trên leg mới: envelope không bao giờ render thành divider trên live path (không ai gọi `addMessage("user", envelope)`), còn model reply của seed ("Chào bạn! Mình là Grok 4.5…") lộ thành **assistant bubble mồ côi, không có You-box** (`cht_e4975cb1f769` seq 5-8).
- Tab trong lúc `chatSwitchInFlight` hiện message sai: *"Cannot switch provider/model while a question or approval is pending"* — không có question nào được mount.
- Fix: divider render **synchronous** tại switch commit (D-7 deterministic), seed guard `seedTurnActive` drop mọi assistant output của seed (live + replay + backfill), busy message tách riêng cho in-flight switch.

### Current Ask

- Live: switch xong có đúng 1 divider `⇄ switched to … — carried N turns (mode)`, không có bubble mồ côi.
- Reopen/backfill: seed prompt + reply của nó bị drop, chỉ E-9 divider đại diện.
- Tab lúc switch đang bay: nói "switch in progress", không phải "question or approval".

### Key Decisions

- `D-1` Divider render tại `applyChatSwitched` (synchronous) — thay contract cũ "no client divider on success" (contract cũ chưa bao giờ render live vì live path không add user message từ event; SD-26 D-7 single-source giữ ở phía divider text từ `lastSwitchStats`).
- `D-2` `seedTurnActive`: drop `message_delta`/`message_completed`/`FinalMessage` của seed; clear ở `turn_completed`/`turn_failed`/real `turn_started`.
- `D-3` Replay + backfill: `turn_started` với prompt rỗng hoặc handoff-prefix = seed → drop reply đi kèm (skip-leg trong backfill).
- `D-4` Busy message: `chatSwitchInFlight` → "Provider switch in progress — wait for it to finish, then Tab again"; question/approval giữ message cũ.

### Constraints

- Đã được operator approve sửa 2 test cũ assert hành vi bug (`TestAdoptKeepsTranscriptAndResetsStreamState`, `TestRenderChatTimelineBackfill_SkipsSeedPromptKeepsDivider`).
- Không đổi runner seed dispatch (transcript vẫn ghi seed prompt rỗng + reply — TUI drop ở render, không phải nguồn).

## 1. Issue Summary

Seed turn của switch: runner dispatch `startTurn(prompt=envelope)` fire-and-return → model (Grok) trả lời envelope → transcript ghi `turn_started{prompt:""}` + `message_completed{text:"Chào bạn! Mình là Grok 4.5…"}`. Live TUI: `handleEvent` turn_started không add user message (envelope không thành divider), `message_completed` appendAssistantDelta → bubble mồ côi.

## 2. Root Cause

- Live path thiếu wiring divider: `addMessage("user", HandoffPromptPrefix…)` chỉ được gọi từ send path; seed envelope không bao giờ đi qua → `lastSwitchStats` path chết.
- Seed reply không bị chặn ở cả live (`handleEvent`), replay (`replayHistoryMessages` chỉ skip khi prompt có handoff-prefix — transcript ghi prompt rỗng nên không khớp) và backfill (`renderChatTimelineBackfill` append mọi message_completed).
- Busy message gộp chung `chatSwitchInFlight` với question/approval.

## 3. Fix

- `chat_switch.go` `applyChatSwitched` (success): `m.seedTurnActive = true` + `addMessage("system", m.switchDividerContent(), "")` + nil `lastSwitchStats`.
- `app.go`:
  - `switchDividerContent()` helper (shared với addMessage conversion).
  - `handleEvent`: `message_delta`/`message_completed` drop khi `seedTurnActive`; `turn_completed` clear + skip FinalMessage re-append (`wasSeed`); `turn_failed` clear; `turn_started` real prompt (non-empty, non-envelope) disarm.
- `history.go` `replayHistoryMessages`: `turn_started` prompt rỗng → seed → `skipNextAssistant = true`.
- `chat_switch.go` `renderChatTimelineBackfill`: skip-leg — `turn_started` seed (rỗng/handoff-prefix) → skip message_completed trên leg đó cho tới real turn.
- `chat_posture.go` busy message tách nhánh `chatSwitchInFlight`.

## 4. Tests

New `bug347_seed_turn_suppress_test.go` (additive):

- `TestSeedTurn_SuppressesEnvelopeReplyLive` — divider synchronous + drop seed delta/completed/FinalMessage + guard clear + real turn render.
- `TestSeedTurn_RealTurnDisarmsGuard` — real turn_started disarm khi turn_completed miss.
- `TestTabDuringSwitchInFlight_BusyMessageIsSwitchNotQuestion` — message "switch in progress", không chứa "question or approval".
- `TestReplayHistory_DropsEmptyPromptSeedReply` — seed reply drop, real turns sống.
- `TestTimelineBackfill_DropsSeedReplyOnPriorLeg` — seed reply không backfill, E-9 divider + real turns có.

Updated (operator-approved): `TestAdoptKeepsTranscriptAndResetsStreamState` (divider +1), `TestRenderChatTimelineBackfill_SkipsSeedPromptKeepsDivider` → `…_DropsSeedPromptReplyKeepsDivider`.

## 5. Verification

- `go test ./internal/tui/... -count=1` → full green (app 7.2s, client 36.7s).
- `go test ./internal/runner/ -run 'TestChatSwitch|TestSwitch|TestChatTranscript|TestSwitchMatrix'` → ok.
- Manual chờ verify live: switch sẽ hiện divider ngay + không còn "Chào bạn! Mình là Grok…" mồ côi; Tab in-flight báo "switch in progress".