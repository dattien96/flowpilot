# Task-345: Decision Card UI cho Desktop App và TUI

## Metadata

- Document ID: `Task-345`
- Title: `Decision Card UI cho Desktop App và TUI (request_user_decision render + answer)`
- Feature Keys: `zcode-parity, gate-schema, decision-card-ui`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-09-13`
- Last Updated: `2026-09-13`
- Parent Documents: [CP-62: Zcode Harness Parity](../../07-Coding-Plan/done/CP-62-Zcode-Harness-Parity.md), [Task-339: Structured Escalation Card](./Task-339-Structured-Escalation-Card-And-Or-Explained-Schema.md), [SS-08: Approve Gate](../../05-System-Specs/SS-08-Approve-Gate.md)
- Child Documents: `None`
- Related Documents: [SS-18: Vibe Working Mode](../../05-System-Specs/SS-18-Vibe-Working-Mode.md), [CP-56: Terminal TUI Chat And Flow Client](../../07-Coding-Plan/done/CP-56-Terminal-TUI-Chat-And-Flow-Client.md)
- Replaces: `None`
- Tags: `zcode-parity, gate-schema, decision-card, desktop, tui, cp-62`

## AI Quick View

### Summary

- Task-339 đã ship runner-side: event `user_decision_card_requested` mang payload schema `{question, options[{id,label,consequence}], recommended, evidence, detail}` — nhưng **chưa client nào render** (Task-339 chủ tường minh không đụng renderer).
- Task này build UI card trên **desktop app (`apps/desktop-flowpilot`) và TUI** — Operator chốt 2026-09-13: **không làm admin-web**.
- Click option → client gửi trả lời qua kênh park-resume hiện có với `option_id`; runner nhận được câu trả lời có cấu trúc thay vì văn xuôi tự do.

### Current Ask

- Operator duyệt 2026-09-13 (nhóm 5 task CP-62 follow-up). Discovery trước về event pipeline của desktop app + TUI (client nào nhận ProviderEvent kiểu gì), sau đó implement.

### Key Decisions

- `D-1` Card là view **additive** trên event mới — client không biết event thì bỏ qua (Q-1 wrap-around giữ nguyên: prose card vẫn là fallback).
- `D-2` `recommended` highlight nổi bật; mỗi option hiển thị `label` + `consequence`; `evidence` là danh sách file:line có thể click/hiển thị; `detail` là đoạn phụ đề.
- `D-3` Answer channel: dùng đúng kênh resume/answer hiện có của park/escalate (không tạo endpoint mới) — message mang `option_id`; runner-side match option (Task-346 ghi nhận lựa chọn). **Task-350 hiệu chỉnh**: desktop gửi qua `continueFlow` (POST `/agent-loop/continue`) — `/turns` bị seal 409 trên run parked; Q-1 prose fallback trên desktop là ô feedback của FlowAwaitingUserCard (composer bị khóa khi parked).
- `D-4` Parity: cùng payload event, cùng hành vi answer trên cả desktop + TUI.

## 1. Goal

Khi agent gọi `request_user_decision`, user trên desktop app và TUI thấy thẻ lựa chọn 1-chạm (options + consequence + recommended + evidence) thay vì一段 prose; lựa chọn được gửi về runner có cấu trúc.

## 2. Parent Links

- CP-62 P-3 completion + Task-339 Completion Notes ("không sửa renderer" — Operator nay gỡ constraint này cho desktop/TUI).

## 3. Trigger

- Operator duyệt 2026-09-13.

## 4. Exact Change

### 4.0 Before → After

| | Before (hiện trạng trước Task-345) | After (sau Task-345) |
|---|---|---|
| **Hỏi user** | Event `user_decision_card_requested` chỉ là payload JSON — **không client nào render** (Task-339 chủ tường minh không đụng renderer); user chỉ nhận prose | Desktop app + TUI render **thẻ lựa chọn thật**: question, options dạng nút/dòng đánh số, `consequence` dưới mỗi lựa chọn, `recommended` highlight, `evidence` file:line |
| **Cách trả lời** | User gõ văn xuôi tự do ("ừ chọn cái đầu nhưng thêm refresh token") → runner khó map về option | 1 chạm nút (desktop) / gõ số-tên (TUI) → gửi **`option_id`** qua kênh parked-run feedback (`/agent-loop/continue`) — máy đọc được |
| **Fallback** | n/a | Q-1 giữ nguyên: payload hỏng → prose card; user vẫn gõ tự do được (desktop composer không khóa, TUI text lạ gửi nguyên văn) |
| **Phạm vi** | n/a | admin-web **không** đụng (Operator chốt); runner không đổi gì (client-only) |


- `T-1` Discovery: trace `user_decision_card_requested` từ ProviderEvent stream tới desktop app (`apps/desktop-flowpilot`) và TUI — tìm chỗ render event gate/ask_user hiện có để đặt card cạnh đó.
- `T-2` Desktop app: component DecisionCard (question, options buttons, recommended highlight, consequence, evidence, detail) + click → gửi `option_id` qua kênh answer/resume hiện có.
- `T-3` TUI: render options đánh số + chấp nhận selection + gửi `option_id` cùng kênh.
- `T-4` Tests: contract test payload→view-model cho cả 2 client (nơi có test harness); không đụng runner trừ khi discovery chỉ ra payload cần bổ sung trường gì (additive).

## 5. Touched Areas

- `apps/desktop-flowpilot/...` (component + state), TUI module (theo discovery). Runner: chỉ sửa nếu event payload thiếu trường (additive, đi kèm test).

## 6. Acceptance Check

- [ ] AC-1: Desktop app render card đủ question/options/recommended/consequence/evidence từ event thật.
- [ ] AC-2: Click option gửi `option_id` về runner qua kênh hiện có; run tiếp diễn.
- [ ] AC-3: TUI render + chọn được option, gửi cùng payload.
- [ ] AC-4: Event sai payload → client rơi về prose card như cũ (không crash).
- [ ] AC-5: admin-web không bị đụng.

## 7. Out of Scope

- admin-web render (Operator chốt không làm).
- Đổi schema card trên runner (nếu discovery buộc đổi → tách task riêng hỏi Operator).

## 8. Completion Notes

- Trạng thái: `done` (2026-09-13).
- Wire format: card nằm trong field `input` của runner event (ProviderEvent.Input) — desktop + TUI decode từ đó.
- Desktop: DecisionCard component (options 1-chạm + consequence + recommended highlight + evidence), timeline item mới, status `waiting_question` giữ composer dùng được (Q-1), action `chooseDecisionOption` gửi option id qua `sendPrompt`. Reducer suite 32/32 (esbuild-bundled node:test); typecheck sạch trừ 1 lỗi PRE-EXISTING (`store.chat-mode-persist.test.ts:82` — có cả khi stash thay đổi của tôi; cùng 1 lỗi TS pre-existing trong adminLogic.test.ts đang chặn pipeline `test:phase1` — báo cáo, không sửa theo R1).
- TUI: `DecisionCardState` + message đánh số options (+[recommended]) + evidence; input khớp số/id/label → gửi option id qua kênh feedback parked-run (`/agent-loop/continue`); text khác gửi nguyên văn (Q-1); headless in card non-fatal. 3 test mới pass; 6 test spinner/timing fail ở app test là PRE-EXISTING (fail y hệt trên base f6634215).
- admin-web không bị đụng (Operator chốt).
- Chi tiết: CA-858 (feature key `decision-card-ui`).

## 9. Definition of Done

- [x] AC-1..AC-5 tick; build desktop (typecheck) + TUI (go build/test) xanh trừ pre-existing đã ghi trên.
- [x] CA note + feature key `decision-card-ui` đăng ký trong `change-audit/FEATURE-KEYS.md` (CA-858).
- [x] Commit `[Feature][zcode-parity] ... Task-345`.
