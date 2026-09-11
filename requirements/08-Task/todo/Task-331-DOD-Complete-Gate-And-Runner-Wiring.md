# Task-331: Xây dựng Cổng r-dod-complete và Tích hợp Runner

## Metadata

- Document ID: `Task-331`
- Title: `Xây dựng Cổng r-dod-complete và Tích hợp Runner`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-09-11`
- Last Updated: `2026-09-11`
- Parent Documents: [CP-47: Cổng kiểm duyệt Definition-of-Done (r-dod)](../../07-Coding-Plan/todo/CP-47-DOD-Gate.md)
- Child Documents: `None`
- Related Documents: [Task-330: DOD Parser và Cổng r-dod-present](../done/Task-330-DOD-Parser-And-Present-Gate.md), [SS-13: Hợp đồng tài liệu cho AI](../../05-System-Specs/SS-13-AI-Followable-Document-Contract.md)
- Replaces: `None`
- Tags: `flowgate, r-dod, r-dod-complete, runner-wiring, block-or-explained`

## AI Quick View

### Summary

- Triển khai quy tắc **`r-dod-complete`** trong `flowgate`: Khi tài liệu `Task-*` hoặc `BUG-*` chuyển trạng thái sang hoàn thành (`Status: done` hoặc di chuyển vào thư mục `done/`), bắt buộc **toàn bộ checkbox trong DOD phải được tích `[x]`**.
- Cơ chế linh hoạt có kiểm soát (**Block-or-Explained**):
  - Nếu tất cả checkbox đều là `[x]` $\rightarrow$ Cho phép hoàn thành (`pass`).
  - Nếu còn checkbox `[ ]` chưa tick nhưng lượt này có đoạn giải trình hợp lệ (ghi chú lý do hoãn / loại bỏ trong doc hoặc trong `FinalMessage`) $\rightarrow$ Cho qua dạng cảnh báo (`warn`).
  - Nếu còn checkbox `[ ]` mà **không có giải trình** $\rightarrow$ Chặn đứng (`block`) không cho phép chuyển trạng thái `done`.
- Tích hợp phát hiện tín hiệu chuyển trạng thái hoàn thành vào `runner/gate_hook.go` cho cả Root flow và Child flow.

### Current Ask

- Thêm quy tắc `r-dod-complete` vào `flowgate.DefaultRules()`.
- Bổ sung logic trích xuất trạng thái Done và đánh giá `r-dod-complete` trong `evaluate.go`.
- Tích hợp luồng tín hiệu trong `apps/local-runner/internal/runner/gate_hook.go`.

### Key Decisions

- `T-1` **Tín hiệu chuyển trạng thái Done từ file thực tế**: Đọc trạng thái từ metadata của file `Task-*` / `BUG-*` được ghi trong lượt (`Status: done`) hoặc nhận diện file được di chuyển/ghi mới vào thư mục con `done/`.
- `T-2` **Nhận diện giải trình hợp lệ (Explanation Detection)**: Tương tự như cơ chế của `r-tests` (`tests_green_or_explained`), một lượt được coi là có giải trình nếu:
  - Có dòng giải thích ngay cạnh checkbox chưa tick (ví dụ: `- [ ] Mục X (Hoãn sang sprint sau do phụ thuộc Y)`).
  - Hoặc trong tài liệu có section `## Deferred` / `## Open Items`.
  - Hoặc `FinalMessage` của AI giải trình rõ lý do các mục còn mở.
- `T-3` **Tự động bỏ qua khi không chạm vào doc (Bypass Safety)**: Nếu lượt chạy là một hotfix nhanh không ghi hay sửa file `Task-*`/`BUG-*` nào, gate tự động bypass.

### Constraints

- Không làm ảnh hưởng đến các gate khác như `r-tests` hay `r-reg`.
- Khi bị `block`, thông báo lỗi phải liệt kê đích danh các checkbox còn đang mở (`OpenItems`) để AI hoặc người dùng biết chính xác cần hoàn thành điều gì.

### Source Refs

- `requirements/07-Coding-Plan/todo/CP-47-DOD-Gate.md` (P-3, P-4, P-5).
- `apps/local-runner/internal/runner/gate_hook.go` (Hạ tầng gate hook của runner).
- `apps/local-runner/internal/flowgate/evaluate.go` (Bộ đánh giá rule).

### Open Questions

- Đã giải quyết: Tín hiệu Done ưu tiên metadata `Status: done` trước, đường dẫn `done/` làm backup.
- Đã giải quyết: Giải trình có thể nằm trong doc hoặc trong `FinalMessage`.

---

## 1. Goal

Khóa chặt tính kỷ luật khi nghiệm thu công việc: Ngăn chặn triệt để tình trạng task/bug được đánh dấu `done` khi các tiêu chuẩn kỹ thuật hoặc kiểm thử cam kết trong Definition of Done vẫn còn dang dở mà không có lý do chính đáng.

---

## 2. Parent Links

- Coding Plan: [CP-47 P-3, P-4, P-5](../../07-Coding-Plan/todo/CP-47-DOD-Gate.md).
- Sibling Task: [Task-330](../done/Task-330-DOD-Parser-And-Present-Gate.md).

---

## 3. Trigger

Hiện tại, AI có thể tự ý di chuyển file Task vào thư mục `done/` hoặc sửa metadata thành `done` dù thực tế vẫn chưa viết xong test hay chưa hoàn thiện checklist. Cần một cổng chặn tự động mức runtime.

---

## 4. Exact Change

- `T-1` Thêm trường `DodTransitionedToDone bool` và `DodStatus DodStatus` vào struct `TurnResult` trong `rules.go`.
- `T-2` Thêm rule `r-dod-complete` vào `DefaultRules()` với `Action = "block"` và `RequiredOutput = "dod_all_checked_or_explained"`.
- `T-3` Thêm logic kiểm tra trong `evaluate.go` cho trigger `marked_done_with_open_dod`.
- `T-4` Trong `gate_hook.go`: Kiểm tra các file `Task-*` / `BUG-*` trong `tr.WrittenPaths`, đọc nội dung để tính toán `DodTransitionedToDone` và nạp vào `TurnResult`.

---

## 5. Touched Areas

- `apps/local-runner/internal/flowgate/rules.go`
- `apps/local-runner/internal/flowgate/evaluate.go`
- `apps/local-runner/internal/flowgate/r_dod_complete_test.go` (Mới)
- `apps/local-runner/internal/runner/gate_hook.go`
- `apps/local-runner/internal/runner/task331_gate_hook_dod_test.go` (Mới)

---

## 6. Acceptance Check

- Chạy `go test ./internal/flowgate/ -run TestRDodComplete` pass 100%.
- Chạy `go test ./internal/runner/ -run TestGateHook_DodComplete` pass 100%.
- Khi file Task chuyển sang `done` nhưng còn checkbox trống không giải trình $\rightarrow$ Bị chặn (`block`), trả về danh sách các checkbox mở.
- Khi file Task chuyển sang `done` và tất cả checkbox đã tick `[x]` $\rightarrow$ Cho qua mượt mà (`pass`).

---

## 7. Out of Scope

- Không can thiệp vào quy trình merge Git hoặc tạo Pull Request.
- Không tự động thay đổi nội dung file trên ổ đĩa.

---

## 8. Completion Notes

- Trạng thái: `draft` (chờ triển khai).

---

## 9. Definition of Done

- [ ] Trường `DodTransitionedToDone` và `DodStatus` được bổ sung vào `TurnResult`.
- [ ] Khai báo `r-dod-complete` trong `flowgate.DefaultRules()` với `Action = "block"` và `Trigger = "marked_done_with_open_dod"`.
- [ ] `checkRule` trong `evaluate.go` chặn (`block`) khi có checkbox chưa tick và không có giải trình.
- [ ] `checkRule` hạ cấp thành cảnh báo (`warn`) khi có checkbox chưa tick nhưng có đoạn giải trình lý do trong tài liệu hoặc trong `FinalMessage`.
- [ ] Runner (`gate_hook.go`) trích xuất đúng tín hiệu khi file Task/Bug chuyển sang `done` và kích hoạt kiểm tra cổng.
- [ ] Các flow không đụng đến file Task/Bug tự động bypass cổng `r-dod-complete`.
- [ ] Toàn bộ unit tests mới pass và không làm gãy các unit tests hiện có.
- [ ] `MergeDefaultRules` tự động bổ sung `r-dod-complete` vào `flow-rules.json` của workspace cũ.
- [ ] Thông báo vi phạm hiển thị đầy đủ trên UI Decision Card của runner.

---

## 10. Test Signature Guide (TDD)

Tệp kiểm thử: `apps/local-runner/internal/flowgate/r_dod_complete_test.go`

```go
package flowgate

import "testing"

// Scenario: Đánh dấu done khi tất cả checkbox đã tick [x]
// Input: DodTransitionedToDone=true, DodStatus{Total: 3, Checked: 3, OpenItems: []}
// Expect: Không phát sinh vi phạm (Pass)
func TestRDodComplete_AllChecked_Pass(t *testing.T) {}

// Scenario: Đánh dấu done nhưng còn checkbox [ ] mở và không giải trình
// Input: DodTransitionedToDone=true, DodStatus{Total: 3, Checked: 2, OpenItems: ["Item 3"]}, FinalMessage="Xong task"
// Expect: Phát sinh vi phạm Rule="r-dod-complete", Action="block", Detail chứa "Item 3"
func TestRDodComplete_OpenItemsNoExplanation_Block(t *testing.T) {}

// Scenario: Đánh dấu done, còn checkbox [ ] mở nhưng có giải trình hợp lệ
// Input: DodTransitionedToDone=true, DodStatus{Total: 3, Checked: 2, OpenItems: ["Item 3"]}, FinalMessage="Hoãn Item 3 sang phase sau vì lý do phụ thuộc API"
// Expect: Phát sinh vi phạm Action="warn" (hạ cấp từ block), cho phép qua
func TestRDodComplete_OpenItemsWithExplanation_Warn(t *testing.T) {}

// Scenario: Lượt chạy bình thường, doc chưa chuyển sang done
// Input: DodTransitionedToDone=false, DodStatus{Total: 3, Checked: 1}
// Expect: Không kích hoạt rule r-dod-complete
func TestRDodComplete_NotTransitionedToDone_Ignore(t *testing.T) {}

// [Edge] Scenario: Section DOD tồn tại nhưng Total == 0 (heading mà không có checkbox)
// Input: DodTransitionedToDone=true, DodStatus{Present: false, Total: 0}
// Expect: Trả về vi phạm r-dod-present (thiếu DOD), không phải r-dod-complete
func TestRDodComplete_ZeroDodItems_DelegatesToPresent(t *testing.T) {}

// [Edge] Scenario: File nằm trong thư mục done/ nhưng metadata Status vẫn là draft
// Input: WrittenPaths=["requirements/08-Task/done/Task-001.md"], metadata Status=draft
// Expect: Vẫn kích hoạt r-dod-complete vì đường dẫn chứa done/ là tín hiệu đủ
func TestRDodComplete_PathBasedDoneDetection(t *testing.T) {}

// [Error] Scenario: File không thể đọc nội dung khi kiểm tra DOD
// Input: WrittenPaths chứa file hợp lệ nhưng I/O error khi đọc
// Expect: Gate degrade graceful - log cảnh báo, không panic, không block
func TestRDodComplete_FileReadError_GracefulDegradation(t *testing.T) {}
```

Tệp kiểm thử: `apps/local-runner/internal/runner/task331_gate_hook_dod_test.go`

```go
package runner

import (
	"context"
	"testing"
)

// Scenario: Runner phát hiện file Task chuyển vào thư mục done/ và kích hoạt r-dod-complete
// Input: WrittenPaths chứa "requirements/08-Task/done/Task-001.md" có checkbox chưa tick
// Expect: runFlowGate trả về block=true
func TestGateHook_DodComplete_BlocksUnfinishedTask(t *testing.T) {}

// Scenario: Runner bỏ qua r-dod-complete khi chỉ sửa code và không chạm file Task/Bug
// Input: WrittenPaths chứa "src/service.go"
// Expect: runFlowGate cho phép pass bình thường
func TestGateHook_DodComplete_BypassesWhenNoDocTouched(t *testing.T) {}

// [Edge] Scenario: File Task vừa được tạo mới (chưa từng tồn tại) trong thư mục done/
// Input: WrittenPaths=["requirements/08-Task/done/Task-NEW.md"], file mới hoàn toàn
// Expect: Gate vẫn kích hoạt kiểm tra r-dod-complete
func TestGateHook_DodComplete_NewFileInDoneDir(t *testing.T) {}
```

---

## 11. Code Guide

Cấu trúc mở rộng trong `apps/local-runner/internal/flowgate/rules.go`:

```go
type TurnResult struct {
    // ... các trường hiện có ...
    DodTransitionedToDone bool      `json:"dod_transitioned_to_done,omitempty"`
    DodStatus             DodStatus `json:"dod_status,omitempty"`
}

// Khai báo trong DefaultRules():
Rule{
    ID:             "r-dod-complete",
    Scope:          "step",
    Trigger:        "marked_done_with_open_dod",
    RequiredOutput: "dod_all_checked_or_explained",
    Action:         "block",
    Enabled:        true,
}
```

Đánh giá trong `apps/local-runner/internal/flowgate/evaluate.go`:

```go
case "marked_done_with_open_dod":
    if tr.DodTransitionedToDone && tr.DodStatus.Checked < tr.DodStatus.Total {
        if hasValidDodExplanation(tr) {
            // Có giải trình -> Cho phép qua nhưng gắn cảnh báo
            warnRule := rule
            warnRule.Action = "warn"
            return &Violation{
                Rule:   warnRule,
                Detail: fmt.Sprintf("tài liệu hoàn thành nhưng còn %d mục DOD chưa tích (đã có giải trình): %s", 
                    len(tr.DodStatus.OpenItems), strings.Join(tr.DodStatus.OpenItems, ", ")),
            }
        }
        // Không có giải trình -> Chặn cứng
        return &Violation{
            Rule:   rule,
            Detail: fmt.Sprintf("chặn hoàn thành: còn %d mục DOD chưa hoàn thành và không có giải trình: %s", 
                len(tr.DodStatus.OpenItems), strings.Join(tr.DodStatus.OpenItems, ", ")),
        }
    }
```

Hàm hỗ trợ nhận diện giải trình trong `apps/local-runner/internal/flowgate/evaluate.go`:

```go
// hasValidDodExplanation kiểm tra xem lượt chạy có chứa đoạn giải trình hợp lệ
// cho các mục DOD còn mở hay không.
func hasValidDodExplanation(tr TurnResult) bool {
	// 1. Kiểm tra trong nội dung tài liệu: Tìm section ## Deferred hoặc ## Open Items
	// 2. Kiểm tra các dòng cạnh checkbox chưa tick có chứa cụm giải trình
	//    (ví dụ: "Hoãn", "Deferred", "Loại bỏ", "Bỏ qua vì")
	// 3. Kiểm tra tr.FinalMessage có chứa đoạn giải trình cho các mục còn mở
	// Trả về true nếu tìm thấy bất kỳ giải trình nào
	return false
}
```
