# Task-346: Sprint Handoff Enrichment (Card Choice + Weakened Tests)

## Metadata

- Document ID: `Task-346`
- Title: `Sprint Handoff Enrichment (Card Choice + Weakened Tests vào handoff-sprint)`
- Feature Keys: `zcode-parity, sprint-handoff`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-09-13`
- Last Updated: `2026-09-13`
- Parent Documents: [CP-62: Zcode Harness Parity](../../07-Coding-Plan/done/CP-62-Zcode-Harness-Parity.md), [Task-342: Sprint Handoff Artifact](../done/Task-342-Sprint-Handoff-Artifact-Schema-And-Chain.md), [Task-345: Decision Card Desktop TUI](./Task-345-Decision-Card-Desktop-TUI.md)
- Child Documents: `None`
- Related Documents: [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md), [CP-49: Reverse-Documentation](../../07-Coding-Plan/done/CP-49-Reverse-Documentation-And-Doc-Ingestion.md)
- Replaces: `None`
- Tags: `zcode-parity, sprint-handoff, decision-card, oracle, cp-62`

## AI Quick View

### Summary

- Task-342 ship schema `sprint_handoff.v1` đủ field nhưng mới điền từ verdict rows: `open`, `weakened_tests`, `alternatives` chưa có nguồn; `chosen_option` của card P-3 chưa được consume.
- Task này nối 2 nguồn dữ liệu verified có sẵn: (1) **decision card** (`rs.decisionCard` — Task-339) + lựa chọn của user (Task-345 gửi `option_id` về) → `decisions` (kèm `alternatives` = các option còn lại); (2) **`TamperedTestPaths`** (oracle guard đã chạy ở `gate_hook.go:304/:1144`, TurnResult) → `weakened_tests` kèm justification từ violation detail.

### Current Ask

- Operator duyệt 2026-09-13 (nhóm 5 task). Làm SAU Task-345 (cần kênh `option_id` để capture lựa chọn).

### Key Decisions

- `D-1` Vẫn nguyên tắc CP-49 hard-ceiling: chỉ ghi dữ liệu verified (card schema + lựa chọn khớp option id/label; tamper path từ oracle guard) — không bịa.
- `D-2` Lựa chọn user: khi run có `decisionCard` active và resume message khớp (case-insensitive) một `option_id`/`label` → ghi nhận `rs.decisionCardChosen`; không khớp → chỉ ghi question + options (không đoán).
- `D-3` Schema KHÔNG đổi (field names giữ nguyên như CP-62 P-6) — chỉ điền thêm dữ liệu vào field hiện có.
- `D-4` Best-effort như Task-342: I/O/parse fail không bao giờ chặn sprint chain.

## 1. Goal

Sprint sau đọc handoff thấy đủ: quyết định nào user/owner đã chốt (kèm phương án bị loại), test nào bị đụng trong sprint (kèm lý do) — hết cảnh "sprint sau sửa ngược code/test của sprint trước vì không biết why".

## 2. Parent Links

- CP-62 P-6 completion; Task-342 Completion Notes (open/weakened_tests/alternatives chưa điền).

## 3. Trigger

- Operator duyệt 2026-09-13; dependency Task-345.

## 4. Exact Change

- `T-1` `gate_hook.go`: nơi build TurnResult (2 điểm đã có `oracle.Tampered`) → lưu `rs.lastTamperedTestPaths` (field additive trên interactiveRun) kèm detail.
- `T-2` `interactive_service.go`: khi run có `decisionCard` và nhận resume/answer khớp option → `rs.decisionCardChosen = optionID`.
- `T-3` `sprint_handoff.go` `emitSprintHandoff`: (a) `rs.decisionCard != nil` → 1 entry `decisions` (what = question, why = recommended + chosen nếu có, alternatives = labels còn lại); (b) `rs.lastTamperedTestPaths` → `WeakenedTest{Path, Justification}`.
- `T-4` Tests mới: card + chosen → decisions/alternatives đúng; card không chosen → vẫn ghi question; tampered → weakened_tests; rỗng → field omitted (schema byte-identical khi trống).

## 5. Touched Areas

- `internal/runner/sprint_handoff.go`, `interactive_service.go` (2 field + capture), `gate_hook.go` (2 điểm capture đã có data), test mới `sprint_handoff_enrichment_test.go`.

## 6. Acceptance Check

- [ ] AC-1: Handoff sprint có entry decisions từ card (kèm chosen + alternatives khi có lựa chọn).
- [ ] AC-2: Tampered test trong sprint xuất hiện ở `weakened_tests` với justification.
- [ ] AC-3: Không có dữ liệu → file giống hệt hiện tại (field omitted).
- [ ] AC-4: Sprint chain không bao giờ fail vì enrichment (best-effort).
- [ ] AC-5: Suite runner xanh (trừ pre-existing), test cũ untouched.

## 7. Out of Scope

- Đổi schema handoff; ghi handoff từ cohort verdict rows (harness) — nếu cần thì task riêng.
- UI (thuộc Task-345).

## 8. Completion Notes

- Trạng thái: `done` (2026-09-13).
- Triển khai khớp §4: choice capture qua `captureDecisionChoice` trong `handleContinueFlow` (match id/label case-insensitive; prose → không đoán); tamper capture ở cả 2 điểm oracle (`gate_hook.go` root + child) qua `setTamperedTestPaths`; `emitSprintHandoff` thêm entry decisions từ card (why = chosen label+consequence / detail / "recommended: <id>"; alternatives = các label còn lại) + weakened_tests với justification từ oracle guard.
- Schema không đổi; không có nguồn → field omitted (byte-identical với output Task-342).
- `open` giữ nguyên là chỗ trống chủ đích trong schema (đúng plan).
- Chi tiết: CA-860.

## 9. Definition of Done

- [x] AC-1..AC-5 tick.
- [x] CA note ghi `sprint-handoff` (CA-860).
- [x] Commit `[Feature][zcode-parity] ... Task-346`.
