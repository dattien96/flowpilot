# BUG-350: Desktop backfill hiện seed reply thành bubble mồ côi (lệch TUI skipLeg)

## Metadata

- Document ID: `BUG-350`
- Title: `Desktop backfill hiện seed reply thành bubble mồ côi (lệch TUI skipLeg)`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-04`
- Last Updated: `2026-09-04`
- Feature Keys: `chat-history`
- Parent Documents: [CP-59: Chat SSOT](../../07-Coding-Plan/inprogress/CP-59-Test-Steps.md)
- Child Documents: `none`
- Related Documents: [SD-26](../../06-System-Tech-Design/SD-26-Chat-Continuity-Ssot.md), [CA-726](../../../change-audit/CA-726-desktop-backfill-seed-skip.md), [BUG-347](./BUG-347-TUI-Seed-Turn-Lo-Reply-Va-Busy-Message-Sai.md)
- Replaces: `none`
- Tags: `chat-history, desktop, backfill, seed-turn, tui-parity`

## AI Quick View

### Summary

- I1 headless cross-render trên live `cht_1e5b706a8201` (16 records): code thật TUI `renderChatTimelineBackfill` render **5 items** (skip seed reply seq 8 "Tôi là Grok 4.5…"), code Desktop `buildPriorChatTimeline` render **6 items** — seed reply hiện thành assistant bubble mồ côi.
- Root cause: Desktop thiếu luật `skipLeg` của TUI (seed turn empty/envelope prompt → skip leg đó tới khi real turn tới). BUG-347 mới fix phía TUI.
- Fix: mirror `skipLeg` vào `buildPriorChatTimeline` + mirror copy trong `store.chatOpenTimeline.test.ts`; expectation test cũ "skips handoff seed prompt" sửa theo (operator-approved: message trên seed leg trước real turn = seed reply → skip, chỉ giữ divider); thêm test shape live (empty-prompt seed).
- Sau fix: Desktop render đúng 5 items identical TUI trên cùng live data.

### Current Ask

- Live (Desktop): mở lại `cht_1e5b706a8201` từ history → không còn bubble "Tôi là Grok 4.5…" mồ côi, đúng 1 divider sau turn đầu.

## Verification

- Headless cross-render (Go test throwaway với live JSON + node harness extract nguyên văn function): TUI 5 items vs Desktop 5 items identical (kinds + texts + divider).
- Suites: `chatOpenTimeline` (4, gồm test BUG-350 mới) + `chatPostureSwitch` + `chatSwitch` + `chat-detached-parity` = 23/23 green.
- Project `tsc`: không lỗi mới.
