# BUG-349: Desktop posture Tab cross-provider không switch — không mint leg, không divider (lệch TUI)

## Metadata

- Document ID: `BUG-349`
- Title: `Desktop posture Tab cross-provider không switch — không mint leg, không divider (lệch TUI)`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-04`
- Last Updated: `2026-09-04`
- Feature Keys: `chat-history`
- Parent Documents: [CP-59: Chat SSOT](../../07-Coding-Plan/done/CP-59-Test-Steps.md)
- Child Documents: `none`
- Related Documents: [SD-26](../../06-System-Tech-Design/SD-26-Chat-Continuity-Ssot.md), [CA-725](../../../change-audit/CA-725-desktop-posture-tab-switch-route.md), [BUG-348](./BUG-348-Desktop-Posture-Modal-Nhap-Text-Tay-Lech-TUI.md)
- Replaces: `none`
- Tags: `chat-history, desktop, posture-tab, provider-switch, tui-parity`

## AI Quick View

### Summary

- Operator E7: bấm Tab qua lại giữa các posture (Plan=grok, Code=opencode) nhưng không thấy dòng divider `⇄ switched to …`. Verify runner: chat mới nhất `cht_8efbaa593d69` chỉ có 1 leg opencode — Tab chưa hề mint leg.
- Root cause: Desktop `setChatPosture` (`apps/desktop-flowpilot/src/state/store.ts`) chỉ apply pins vào session state (in-place), không bao giờ gọi `switchChatProvider`. TUI `routePostureSwitch` (`apps/local-runner/internal/tui/app/chat_switch.go`) route cross-provider Tab trên live chat tới switch endpoint (mint leg + divider); chỉ in-place khi: chưa có run, cùng provider, detached, hoặc switch đang in-flight.
- Fix: `setChatPosture` capture `prevProvider` trước apply; sau persist-active, nếu pin resolve khác provider + live chat (`normal_chat` + `chatId` + `runId`, không detached, không switch in-flight, không question/approval pending) → set `pendingProviderSwitch` (targetModel = pinned model, không phải provider default) + `await confirmProviderSwitch()` (dùng lại divider `⇄ switched to …` + timeline kept của E1).
- Kèm E7 derive-once: pin có model nhưng không provider → derive 1 lần, persist vào runner config, notice `Posture <p> pin had no provider — derived "<prov>" from model "<m>" (persisting)`; Tab sau provider đã set nên notice đúng 1 lần.

### Current Ask

- Live (Desktop, E7 retest): Tab Code(opencode) → Plan(grok) trên chat đã có turn → đúng 1 divider, leg mới grok; Tab ngược lại tương tự; Tab lần 2 không derive lại.

## Verification

- New `src/state/store.chatPostureSwitch.test.ts`: 6/6 pass (cross-provider route + pinned model, same-provider in-place, no-live-chat local, detached local, in-flight skip, bare-pin derive-once + persist + no-double).
- Neighbors green: `store.chatSwitch` + `store.chat-detached-parity` + `store.chatOpenTimeline` 16/16.
- Project `tsc`: không lỗi mới (2 lỗi pre-existing ở `adminLogic.test.ts`, `store.chat-mode-persist.test.ts`, verify qua grep).
