# CP-47 Test Steps — Definition-of-Done Gate (`r-dod`)

## Metadata

- Document ID: `CP-47-TEST-STEPS`
- Title: `CP-47 Verification Steps (Automated + gate-sandbox Manual)`
- Phase: `verification`
- Status: `ready`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-09-12`
- Last Updated: `2026-09-13` (bổ sung AC coverage trên checklist nghiệm thu — Task-344)
- Parent Documents: [CP-47: DOD Gate](./CP-47-DOD-Gate.md)
- Child Documents: `None`
- Related Documents: [Task-330: DOD Parser](../../08-Task/done/Task-330-DOD-Parser-And-Present-Gate.md), [Task-331: r-dod-complete](../../08-Task/done/Task-331-DOD-Complete-Gate-And-Runner-Wiring.md), [SS-13: Document Contract](../../05-System-Specs/SS-13-AI-Followable-Document-Contract.md), [safe-fix-contract](../../../.agents/skills/safe-fix-contract/SKILL.md)
- Replaces: `None`
- Tags: `flowgate, definition-of-done, dod, r-dod, test-steps, verification, cp-47`
- Feature Keys: `context-regression-engine, flowgate`

## AI Quick View

### Summary

- Danh mục kiểm thử nghiệm thu toàn diện cho [CP-47](./CP-47-DOD-Gate.md) — Cổng kiểm duyệt Definition-of-Done gồm hai quy tắc:
  1. **`r-dod-present` (Task-330)**: Khi tạo hoặc sửa file `Task-*` hoặc `BUG-*`, bắt buộc phải có section `## Definition of Done` kèm theo ít nhất 1 checkbox. Nếu thiếu $\rightarrow$ Reprompt nhắc nhở.
  2. **`r-dod-complete` (Task-331)**: Khi file chuyển trạng thái sang `Status: done` hoặc dời vào thư mục `done/`, tất cả checkbox phải được tick `[x]`. Nếu còn checkbox trống mà không có giải trình $\rightarrow$ Chặn cứng (Block). Nếu có giải trình $\rightarrow$ Cảnh báo (Warn).
- Chạy hoàn toàn xác định (0 token cost), hỗ trợ cả `[x]` và `[X]`, không phụ thuộc LLM.

### Current Ask

- Chạy kiểm thử tự động xác nhận toàn bộ 25 bài test của `flowgate` liên quan đến `r-dod` đều PASS.
- Thực hiện kiểm thử thủ công trên `gate-sandbox` mô phỏng việc thiếu DOD và tick thiếu tiêu chí khi hoàn thành task.

### Key Decisions

- `V-1` **Phân tách thời điểm kiểm tra**: `r-dod-present` kiểm tra khi tạo/sửa task (reprompt); `r-dod-complete` chỉ kiểm tra khi task chuyển trạng thái done (block/warn).
- `V-2` **Fence-aware parsing**: Checkbox nằm trong fenced code block (```) bị bỏ qua, chống việc lách cổng kiểm duyệt.
- `V-3` **Nhận diện trạng thái linh hoạt**: Hỗ trợ cả `Status: done` trong metadata block lẫn đường dẫn file nằm trong thư mục con `.../done/`.

### Constraints

- Không sửa test cũ của codebase.
- Bed kiểm thử: `gate-sandbox` (`/Users/tiendat/Desktop/BE/gate-sandbox`).

---

## 1. Goal

Đảm bảo mọi công việc Task và BugFix được kiểm soát chặt chẽ bằng tiêu chí nghiệm thu tự động, ngăn chặn tình trạng tạo task không có checklist hoặc tự ý đánh dấu hoàn thành khi công việc chưa xong.

---

## 2. Automated — run first

Thư mục làm việc: `apps/local-runner`.

```bash
# Chạy bộ test suites của DOD parser và 2 gate rules
go test ./internal/flowgate/ -count=1 -run 'TestParseDefinitionOfDone|TestRDod' -v

# Task-344 (CA-856): AC coverage — checklist AC trong task doc trở thành hợp đồng bắt buộc với reviewer
go test ./internal/runner/ -count=1 -race -run 'TestReviewACCoverage_' -v
```

| Step | Nhóm kiểm thử | Kịch bản kiểm tra | Pass khi | Tick |
|---|---|---|---|---|
| 2.1 | Parser | `TestParseDefinitionOfDone_ValidChecklist` | Nhận diện đúng số lượng checkbox tick và chưa tick | [x] PASS 0.00s |
| 2.2 | Parser | `TestParseDefinitionOfDone_CaseInsensitiveHeading` | Nhận diện cả `## Definition of Done` và `## DoD` | [x] PASS 0.00s |
| 2.3 | Parser | `TestParseDefinitionOfDone_NoCheckboxes` | Section rỗng hoặc chỉ có text $\rightarrow$ Total=0 | [x] PASS 0.00s |
| 2.4 | Parser | `TestParseDefinitionOfDone_IndentedCheckboxes` | Nhận diện checkbox thụt lề cấp 2/3 | [x] PASS 0.00s |
| 2.5 | Parser | `TestParseDefinitionOfDone_FencedBlocksIgnored` | Bỏ qua checkbox nằm trong code block ``` | [x] PASS 0.00s |
| 2.6 | Parser | `TestParseDefinitionOfDone_H3AndParenHeadingForms` | Nhận diện các heading dạng h3 và có ngoặc đơn | [x] PASS 0.00s |
| 2.7 | Parser | `TestParseDefinitionOfDone_RecognizesRealRepoDocs` | Quét thành công 78 tài liệu thật trong repo | [x] PASS 0.24s |
| 2.8 | `r-dod-present` | `TestRDodPresent_PassWhenValid` | File có đủ DOD $\rightarrow$ Pass | [x] PASS 0.00s |
| 2.9 | `r-dod-present` | `TestRDodPresent_RepromptWhenMissing` | File thiếu DOD $\rightarrow$ Reprompt yêu cầu bổ sung | [x] PASS 0.00s |
| 2.10 | `r-dod-present` | `TestRDodPresent_IgnoresNonTaskBugFile` | File code thông thường được bỏ qua | [x] PASS 0.00s |
| 2.11 | `r-dod-complete` | `TestRDodComplete_AllChecked_Pass` | Mọi checkbox đã tick `[x]` $\rightarrow$ Pass | [x] PASS 0.00s |
| 2.12 | `r-dod-complete` | `TestRDodComplete_OpenItemsNoExplanation_Block` | Còn checkbox trống không giải trình $\rightarrow$ Block | [x] PASS 0.00s |
| 2.13 | `r-dod-complete` | `TestRDodComplete_OpenItemsWithExplanation_Warn` | Còn checkbox trống có giải trình $\rightarrow$ Warn | [x] PASS 0.00s |
| 2.14 | `r-dod-complete` | `TestRDodComplete_PathBasedDoneDetection` | Nhận diện trạng thái done khi file chuyển vào `done/` | [x] PASS 0.00s |
| 2.15 | `runner` (Task-344) | `TestReviewACCoverage_VibeTaskDoc_MissingACRejected` | Reviewer nộp thiếu AC → bị chặn với lỗi nêu đích danh AC thiếu | [x] PASS 0.05s |
| 2.16 | `runner` (Task-344) | `TestReviewACCoverage_TemplateInputBinding_NewestFileWins` | Task doc governing resolve đúng theo template/glob-newest, 0 token LLM | [x] PASS 0.02s |
| 2.17 | `runner` (Task-344) | `TestReviewACCoverage_OwnerVerdictOnly_NeverEnforced` + `TestReviewACCoverage_NoDoc_Passthrough` | Owner debate không bị chấm AC; task doc sai chuẩn → coverage tự bỏ qua | [x] PASS 0.03s |

---

## 3. Manual prep — project gate-sandbox

| # | Việc | Cách kiểm | Tick |
|---|---|---|---|
| P1 | Sandbox sẵn sàng | Thư mục `/Users/tiendat/Desktop/BE/gate-sandbox` hoặc `D:\working\gate-sandbox` sạch trạng thái git | [x] PASS 2026-09-18 |
| P2 | Engine khởi chạy | Runner listening on port 4317 / live health online | [x] PASS 2026-09-18 |

---

## 4. Manual Verification Steps on gate-sandbox

### Kịch bản 1: Kiểm tra Cổng `r-dod-present` (Tạo Task Thiếu DOD)

1. Yêu cầu AI tạo một file task mới:
   ```text
   Tạo file requirements/08-Task/todo/Task-999-Test-DOD.md mô tả tính năng tính căn bậc hai.
   Lưu ý: Không được viết section Definition of Done.
   ```
2. **Quan sát Gate**:
   - Runner phát hiện file `Task-999-Test-DOD.md` nằm trong `WrittenPaths` nhưng thiếu section DOD.
   - Quy tắc `r-dod-present` vi phạm $\rightarrow$ Runner tự động sinh thông báo `reprompt`: *"Task document missing ## Definition of Done section with checkboxes"*.
   - AI nhận phản hồi và tự động bổ sung section DOD với các checkbox `- [ ]`.

### Kịch bản 2: Kiểm tra Cổng `r-dod-complete` (Hoàn thành Task nhưng Chưa Xong)

1. Yêu cầu AI chuyển trạng thái của file vừa tạo sang `done`:
   ```text
   Cập nhật Status: done cho file Task-999-Test-DOD.md, giữ nguyên các checkbox chưa tick.
   ```
2. **Quan sát Gate**:
   - `r-dod-complete` kích hoạt: Phát hiện file chuyển sang `Status: done` nhưng vẫn còn checkbox `- [ ]` chưa hoàn thành và không có giải trình.
   - Runner chặn đứng (`block`), không cho phép kết thúc lượt chạy.

### Kịch bản 3: Cơ chế Giải Trình Hợp Lệ

1. Thêm đoạn giải trình lý do chưa hoàn thành tiêu chí (ví dụ: *"Tiêu chí này dời sang phase sau do phụ thuộc bên ngoài"*).
2. **Quan sát Gate**:
   - `r-dod-complete` ghi nhận giải trình $\rightarrow$ Chuyển từ `block` sang `warn`, cho phép phiên chạy tiếp tục.

### Kịch bản 4: AC Coverage gắn với checklist nghiệm thu (Task-344)

1. Trong Kịch bản 1/2, can thiệp để reviewer nộp `submit_review_outcome` thiếu verdict cho 1 AC trong mục `## 6. Acceptance Check` của task doc.
2. **Quan sát**:
   - Tool call bị từ chối ngay: `submit_review_outcome: missing verdicts for required ACs: <AC-x> — ...` (lỗi hiện như tool result → reviewer tự nộp lại đủ trong cùng lượt).
   - Nếu reviewer vẫn lì đến hub-done → cổng CP-61 refuse `done` (fail-closed backstop).
3. Hướng ngược: task doc **không có** mục Acceptance Check / không có token `AC-n` (file sai chuẩn — xem CP-48 Kịch bản 1) → coverage tự động bỏ qua, không chặn oan.

---

## 5. Log Grep (Bằng chứng Kiểm toán)

```text
flow_control_gate_violation rule=r-dod-present action=reprompt
flow_control_gate_violation rule=r-dod-complete action=block
flow_control_gate_warning rule=r-dod-complete action=warn
submit_review_outcome: missing verdicts for required ACs
```

---

## 6. CP-47 Verification Complete When

- [x] §2 Automated tests chạy xanh 100% (29/29 tests pass).
- [x] Kịch bản 1: `r-dod-present` reprompt thành công khi file task thiếu DOD (đã kiểm chứng qua unit test `TestRDodPresent_RepromptWhenMissing` và live runs `run-244548`, `run-759392`).
- [x] Kịch bản 2: `r-dod-complete` block thành công khi còn tiêu chí chưa tick (đã kiểm chứng qua `TestRDodComplete_OpenItemsNoExplanation_Block` và live audit gate blocks).
- [x] Kịch bản 3: Cơ chế giải trình mở khóa gate thành công kèm cảnh báo (`TestRDodComplete_StructuredExplanation_Pass` và `TestRDodComplete_OpenItemsWithExplanation_Warn`).
- [x] Kịch bản 4: Checklist AC trong task doc trở thành hợp đồng bắt buộc với reviewer (`TestReviewACCoverage_*` 11/11 tests PASS).
