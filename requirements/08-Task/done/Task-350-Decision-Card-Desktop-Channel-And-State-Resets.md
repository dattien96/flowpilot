# Task-350: Desktop Card Channel + Cross-Sprint State Resets (fix review P1)

## Metadata

- Document ID: `Task-350`
- Title: `Desktop Decision Card Channel (continueFlow) + Cross-Sprint State Resets`
- Feature Keys: `decision-card-ui, sprint-handoff, context-profile`
- Phase: `task`
- Status: `done`
- Created: `2026-09-14`
- Last Updated: `2026-09-14`
- Parent Documents: [CP-62](../../07-Coding-Plan/done/CP-62-Zcode-Harness-Parity.md), [Task-345](./Task-345-Decision-Card-Desktop-TUI.md), [Task-346](./Task-346-Sprint-Handoff-Enrichment.md), [Task-344](./Task-344-Reviewer-AC-Coverage-Wiring.md)
- Related Documents: [CA-862](../../../change-audit/CA-862-Decision-Card-Desktop-Channel-Fix.md)
- Tags: `decision-card-ui, sprint-handoff, context-profile, cp-62`

## AI Quick View

### Summary
- Review phát hiện **P1**: desktop trả lời card qua `sendPrompt` → `POST /turns` → **409 `flow_awaiting_user`** trên run parked → choice không bao giờ tới runner; kèm claim sai "composer dùng được (Q-1)" ăn vào 5 artifact; và state chéo sprint (verdicts/tampered/card/chosen/AC cache) không được reset.

### Current Ask
- Fix kênh desktop + resets + sửa comment/docs khớp thực tế.

### Key Decisions
- `D-1` Desktop `chooseDecisionOption` → `get().continueFlow(optionId)` (POST `/agent-loop/continue` — cùng kênh TUI + FlowAwaitingUserCard); optimistic answered + rollback khi lỗi (pattern BUG-172).
- `D-2` Cross-sprint reset tại `takeNextVibeSprintLocked` (điểm tăng index duy nhất): `lastFlowVerdicts`, `lastTamperedTestPaths`, `decisionCard`, `decisionCardChosen`, `expectedACsCache/Resolved` — hết misattribution chéo sprint (CP-49) và hết cache AC stale của hub (Task-344 P1).
- `D-3` Card MỚI tới → clear `decisionCardChosen` cũ (không ghép choice cũ với question mới).
- `D-4` Comment/docs sửa theo thực tế: composer bị khóa khi parked; Q-1 prose fallback trên desktop = ô feedback FlowAwaitingUserCard.

## 6. Acceptance Check
- [x] AC-1: desktop chọn option → `POST /agent-loop/continue` mang option id → `captureDecisionChoice` ghi nhận.
- [x] AC-2: sprint advance reset toàn bộ verified state (test pin).
- [x] AC-3: card mới clear choice cũ (code, kèm test tại sprint-advance).
- [x] AC-4: reducer/store/component comments + task doc khớp hành vi.

## 8. Completion Notes
- Desktop typecheck: chỉ còn lỗi pre-existing. Chi tiết CA-862.
